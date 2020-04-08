package hci

import (
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/rigado/ble"
	"github.com/rigado/ble/linux/hci/cmd"
	"github.com/rigado/ble/linux/hci/evt"
)

// Command ...
type Command interface {
	OpCode() int
	Len() int
	Marshal([]byte) error
}

// CommandRP ...
type CommandRP interface {
	Unmarshal(b []byte) error
}

type handlerFn func(b []byte) error

type pkt struct {
	cmd  Command
	done chan []byte
}

// NewHCI returns a hci device.
func NewHCI(smp SmpManagerFactory, opts ...ble.Option) (*HCI, error) {
	h := &HCI{
		smp:       smp,
		chCmdPkt:  make(chan *pkt),
		chCmdBufs: make(chan []byte, chCmdBufChanSize),
		sent:      make(map[int]*pkt),
		muSent:    sync.Mutex{},

		evth: map[int]handlerFn{},
		subh: map[int]handlerFn{},

		muConns:      sync.Mutex{},
		conns:        make(map[uint16]*Conn),
		chMasterConn: make(chan *Conn),
		chSlaveConn:  make(chan *Conn),

		muClose:   sync.Mutex{},
		done:      make(chan bool),
		sktRxChan: make(chan []byte, 16), //todo pick a real number
	}
	h.params.init()
	if err := h.Option(opts...); err != nil {
		return nil, errors.Wrap(err, "can't set options")
	}

	return h, nil
}

// HCI ...
type HCI struct {
	sync.Mutex

	params params

	smp        SmpManagerFactory
	smpEnabled bool

	transport transport
	skt       io.ReadWriteCloser

	// Host to Controller command flow control [Vol 2, Part E, 4.4]
	chCmdPkt  chan *pkt
	chCmdBufs chan []byte
	muSent    sync.Mutex
	sent      map[int]*pkt

	// evtHub
	evth map[int]handlerFn
	subh map[int]handlerFn

	// aclHandler
	bufSize int
	bufCnt  int

	// Device information or status.
	addr    net.HardwareAddr
	txPwrLv int

	// adHist and adLast track the history of past scannable advertising packets.
	// Controller delivers AD(Advertising Data) and SR(Scan Response) separately
	// through HCI. Upon receiving an AD, no matter it's scannable or not, we
	// pass a Advertisement (AD only) to advHandler immediately.
	// Upon receiving a SR, we search the AD history for the AD from the same
	// device, and pass the Advertisiement (AD+SR) to advHandler.
	// The adHist and adLast are allocated in the Scan().
	advHandlerSync bool
	advHandler     ble.AdvHandler
	adHist         []*Advertisement
	adLast         int

	// Host to Controller Data Flow Control Packet-based Data flow control for LE-U [Vol 2, Part E, 4.1.1]
	// Minimum 27 bytes. 4 bytes of L2CAP Header, and 23 bytes Payload from upper layer (ATT)
	pool *Pool

	// L2CAP connections
	muConns      sync.Mutex
	conns        map[uint16]*Conn
	chMasterConn chan *Conn // Dial returns master connections.
	chSlaveConn  chan *Conn // Peripheral accept slave connections.

	dialerTmo   time.Duration
	listenerTmo time.Duration

	//error handler
	errorHandler func(error)
	err          error

	muClose sync.Mutex
	done    chan bool

	sktRxChan chan []byte

	cache ble.GattCache
}

// Init ...
func (h *HCI) Init() error {
	h.evth[0x3E] = h.handleLEMeta
	h.evth[evt.CommandCompleteCode] = h.handleCommandComplete
	h.evth[evt.CommandStatusCode] = h.handleCommandStatus
	h.evth[evt.DisconnectionCompleteCode] = h.handleDisconnectionComplete
	h.evth[evt.NumberOfCompletedPacketsCode] = h.handleNumberOfCompletedPackets
	h.evth[evt.EncryptionChangeCode] = h.handleEncryptionChange

	h.subh[evt.LEAdvertisingReportSubCode] = h.handleLEAdvertisingReport
	h.subh[evt.LEConnectionCompleteSubCode] = h.handleLEConnectionComplete
	h.subh[evt.LEConnectionUpdateCompleteSubCode] = h.handleLEConnectionUpdateComplete
	h.subh[evt.LELongTermKeyRequestSubCode] = h.handleLELongTermKeyRequest
	h.subh[evt.EncryptionChangeCode] = h.handleEncryptionChange
	// evt.ReadRemoteVersionInformationCompleteCode: todo),
	// evt.HardwareErrorCode:                        todo),
	// evt.DataBufferOverflowCode:                   todo),
	// evt.EncryptionKeyRefreshCompleteCode:         todo),
	// evt.AuthenticatedPayloadTimeoutExpiredCode:   todo),
	// evt.LEReadRemoteUsedFeaturesCompleteSubCode:   todo),
	// evt.LERemoteConnectionParameterRequestSubCode: todo),

	var err error
	h.skt, err = getTransport(h.transport)
	if err != nil {
		return err
	}

	// check params
	p := &h.params
	if err = p.validate(); err != nil {
		return err
	}
	h.setAllowedCommands(1)

	go h.sktReadLoop()
	go h.sktProcessLoop()
	if err := h.init(); err != nil {
		return err
	}

	// Pre-allocate buffers with additional head room for lower layer headers.
	// HCI header (1 Byte) + ACL Data Header (4 bytes) + L2CAP PDU (or fragment)
	h.pool = NewPool(1+4+h.bufSize, h.bufCnt-1)
	h.Send(&p.advParams, nil)
	h.Send(&p.scanParams, nil)
	return nil
}

func (h *HCI) cleanup() {
	//close the socket
	h.close(nil)

	//this effectively kills any dials in flight
	close(h.chMasterConn)
	h.chMasterConn = nil

	// kill all open connections w/o disconnect
	for ch := range h.conns {
		h.cleanupConnectionHandle(ch)
	}

	// clean out all sent commands (prob unneeded)
	h.muSent.Lock()
	for k := range h.sent {
		delete(h.sent, k)
	}
	h.muSent.Unlock()
}

// Close ...
func (h *HCI) Close() error {
	h.muClose.Lock()
	defer h.muClose.Unlock()

	select {
	case <-h.done:
		//already closed, nothing to do
	default:
		close(h.done)
	}

	return nil
}

// Error ...
func (h *HCI) Error() error {
	return h.err
}

// Option sets the options specified.
func (h *HCI) Option(opts ...ble.Option) error {
	var err error
	for _, opt := range opts {
		err = opt(h)
	}
	return err
}

func (h *HCI) isOpen() bool {
	select {
	case <-h.done:
		return false
	default:
		return true
	}
}

func (h *HCI) init() error {
	logger.Info("hci reset")
	h.Send(&cmd.Reset{}, nil)

	ReadBDADDRRP := cmd.ReadBDADDRRP{}
	h.Send(&cmd.ReadBDADDR{}, &ReadBDADDRRP)

	a := ReadBDADDRRP.BDADDR
	h.addr = net.HardwareAddr([]byte{a[5], a[4], a[3], a[2], a[1], a[0]})

	//ES note: Per Core Spec 5.0, Part E, 7.4.5
	//This command is _not_ to be supported by LE only controllers
	ReadBufferSizeRP := cmd.ReadBufferSizeRP{}
	h.Send(&cmd.ReadBufferSize{}, &ReadBufferSizeRP)

	// Assume the buffers are shared between ACL-U and LE-U.
	h.bufCnt = int(ReadBufferSizeRP.HCTotalNumACLDataPackets)
	h.bufSize = int(ReadBufferSizeRP.HCACLDataPacketLength)

	LEReadBufferSizeRP := cmd.LEReadBufferSizeRP{}
	h.Send(&cmd.LEReadBufferSize{}, &LEReadBufferSizeRP)

	if LEReadBufferSizeRP.HCTotalNumLEDataPackets != 0 {
		// Okay, LE-U do have their own buffers.
		h.bufCnt = int(LEReadBufferSizeRP.HCTotalNumLEDataPackets)
		h.bufSize = int(LEReadBufferSizeRP.HCLEDataPacketLength)
	}

	LEReadAdvertisingChannelTxPowerRP := cmd.LEReadAdvertisingChannelTxPowerRP{}
	h.Send(&cmd.LEReadAdvertisingChannelTxPower{}, &LEReadAdvertisingChannelTxPowerRP)

	h.txPwrLv = int(LEReadAdvertisingChannelTxPowerRP.TransmitPowerLevel)

	LESetEventMaskRP := cmd.LESetEventMaskRP{}
	h.Send(&cmd.LESetEventMask{LEEventMask: 0x000000000000001F}, &LESetEventMaskRP)

	SetEventMaskRP := cmd.SetEventMaskRP{}
	h.Send(&cmd.SetEventMask{EventMask: 0x3dbff807fffbffff}, &SetEventMaskRP)

	WriteLEHostSupportRP := cmd.WriteLEHostSupportRP{}
	h.Send(&cmd.WriteLEHostSupport{LESupportedHost: 1, SimultaneousLEHost: 0}, &WriteLEHostSupportRP)

	return h.err
}

// Send ...
func (h *HCI) Send(c Command, r CommandRP) error {
	b, err := h.send(c)
	if err != nil {
		return err
	}
	if len(b) > 0 && b[0] != 0x00 {
		return ErrCommand(b[0])
	}
	if r != nil {
		return r.Unmarshal(b)
	}
	return nil
}

func (h *HCI) send(c Command) ([]byte, error) {
	if h.err != nil {
		return nil, h.err
	}

	p := &pkt{c, make(chan []byte)}

	// get buffer w/timeout
	var b []byte
	select {
	case <-h.done:
		return nil, fmt.Errorf("hci closed")
	case b = <-h.chCmdBufs:
		//ok
	case <-time.After(chCmdBufTimeout):
		err := fmt.Errorf("chCmdBufs get timeout")
		h.dispatchError(err)
		return nil, err
	}

	b[0] = byte(pktTypeCommand) // HCI header
	b[1] = byte(c.OpCode())
	b[2] = byte(c.OpCode() >> 8)
	b[3] = byte(c.Len())
	if err := c.Marshal(b[4:]); err != nil {
		h.close(fmt.Errorf("hci: failed to marshal cmd"))
	}

	h.muSent.Lock()
	_, ok := h.sent[c.OpCode()]
	if ok {
		h.muSent.Unlock()
		return nil, fmt.Errorf("command with opcode %v pending", c.OpCode())
	}

	h.sent[c.OpCode()] = p
	h.muSent.Unlock()

	if !h.isOpen() {
		return nil, fmt.Errorf("hci closed")
	} else if n, err := h.skt.Write(b[:4+c.Len()]); err != nil {
		h.close(fmt.Errorf("hci: failed to send cmd"))
	} else if n != 4+c.Len() {
		h.close(fmt.Errorf("hci: failed to send whole cmd pkt to hci socket"))
	}

	var ret []byte
	var err error

	// emergency timeout to prevent calls from locking up if the HCI
	// interface doesn't respond.  Responsed here should normally be fast
	// a timeout indicates a major problem with HCI.
	select {
	case <-time.After(3 * time.Second):
		err = fmt.Errorf("hci: no response to command, hci connection failed")
		fmt.Println("no response to command")
		fmt.Println("pending commands:")
		fmt.Printf("cmd: %x pkt: %s\n", c.OpCode(), hex.EncodeToString(b[:4+c.Len()]))
		h.dispatchError(err)
		ret = nil
	case <-h.done:
		err = h.err
		ret = nil
	case b := <-p.done:
		err = nil
		ret = b
	}

	// clear sent table when done, we sometimes get command complete or
	// command status messages with no matching send, which can attempt to
	// access stale packets in sent and fail or lock up.
	h.muSent.Lock()
	delete(h.sent, c.OpCode())
	h.muSent.Unlock()

	return ret, err
}

func (h *HCI) sktProcessLoop() {

	defer h.cleanup()
	defer h.dispatchError(h.err)

	for {
		var p []byte
		var ok bool

		select {
		case <-h.done:
			fmt.Println("close requested")
			h.err = io.EOF
			return

		case p, ok = <-h.sktRxChan:
			if !ok {
				fmt.Println("socket rx closed")
				h.err = io.EOF
				return
			}
			// will process the bytes below
		}

		if err := h.handlePkt(p); err != nil {
			// Some bluetooth devices may append vendor specific packets at the last,
			// in this case, simply ignore them.
			if strings.HasPrefix(err.Error(), "unsupported vendor packet:") {
				_ = logger.Error("hci", "skt: ", err)
			} else {
				h.err = fmt.Errorf("skt handle error: %v", err)
				return
			}
		}
	}
}

func (h *HCI) sktReadLoop() {
	defer func() {
		fmt.Println("sktRxLoop done")
		close(h.sktRxChan)
	}()

	b := make([]byte, 4096)

	for {
		n, err := h.skt.Read(b)

		switch {
		case n == 0 && err == nil:
			// read timeout
			select {
			case <-h.done:
				//exit!
				return
			default:
				continue
			}

		//callers depend on detecting io.EOF, don't wrap it.
		case err == io.EOF:
			h.err = err
			return

		case err != nil:
			h.err = fmt.Errorf("skt read error: %v", err)
			return

		default:
			// ok
			p := make([]byte, n)
			copy(p, b)
			h.sktRxChan <- p
		}
	}
}

func (h *HCI) close(err error) error {
	h.err = err
	return h.skt.Close()
}

func (h *HCI) handlePkt(b []byte) error {
	// Strip the 1-byte HCI header and pass down the rest of the packet.
	t, b := b[0], b[1:]
	switch t {
	case pktTypeACLData:
		return h.handleACL(b)
	case pktTypeEvent:
		return h.handleEvt(b)

		//unhandled stuff
	case pktTypeCommand:
		return fmt.Errorf("unmanaged cmd: % X", b)
	case pktTypeSCOData:
		return fmt.Errorf("unsupported sco packet: % X", b)
	case pktTypeVendor:
		return fmt.Errorf("unsupported vendor packet: % X", b)
	default:
		return fmt.Errorf("invalid packet: 0x%02X % X", t, b)
	}
}

func (h *HCI) handleACL(b []byte) error {
	handle := packet(b).handle()

	h.muConns.Lock()
	defer h.muConns.Unlock()

	if c, ok := h.conns[handle]; ok {
		c.chInPkt <- b
	} else {
		_ = logger.Warn("invalid connection handle on ACL packet", "handle:", handle)
	}

	return nil
}

func (h *HCI) handleEvt(b []byte) error {
	code, plen := int(b[0]), int(b[1])
	if plen != len(b[2:]) {
		return fmt.Errorf("invalid event packet: % X", b)
	}

	if code == evt.CommandCompleteCode || code == evt.CommandStatusCode {
		if f := h.evth[code]; f != nil {
			return f(b[2:])
		}
	}

	if f := h.evth[code]; f != nil {
		h.err = f(b[2:])
		return nil
	}
	if code == 0xff { // Ignore vendor events
		return nil
	}
	return fmt.Errorf("unsupported event packet: % X", b)
}

func (h *HCI) handleLEMeta(b []byte) error {
	subcode := int(b[0])
	if f := h.subh[subcode]; f != nil {
		return f(b)
	}
	return fmt.Errorf("unsupported LE event: % X", b)
}

func (h *HCI) makeAdvError(e error, b []byte, dispatch bool) error {
	err := fmt.Errorf("%v, bytes %v", e, b)
	if dispatch {
		h.dispatchError(err)
	}
	return err
}

func (h *HCI) handleLEAdvertisingReport(b []byte) error {
	if h.advHandler == nil {
		return nil
	}

	var a *Advertisement
	var err error

	e := evt.LEAdvertisingReport(b)

	nr, err := e.NumReportsWErr()
	if err != nil {
		ee := h.makeAdvError(errors.Wrap(err, "advRep numReports"), e, true)
		return ee
	}

	//DSC: zephyr currently returns 1 report per report wrapper
	if nr != 1 {
		ee := h.makeAdvError(fmt.Errorf("invalid rep count %v", nr), e, true)
		return ee
	}

	for i := 0; i < int(nr); i++ {
		var et byte
		et, err = e.EventTypeWErr(i)
		if err != nil {
			h.makeAdvError(errors.Wrap(err, "advRep eventType"), e, true)
			continue
		}

		switch et {
		case evtTypAdvInd: //0x00
			fallthrough
		case evtTypAdvScanInd: //0x02
			a, err = newAdvertisement(e, i)
			if err != nil {
				h.makeAdvError(errors.Wrap(err, fmt.Sprintf("newAdv (typ %v)", et)), e, true)
				continue
			}
			h.adHist[h.adLast] = a
			h.adLast++
			if h.adLast == len(h.adHist) {
				h.adLast = 0
			}

			//advInd, advScanInd

		case evtTypScanRsp: //0x04
			sr, err := newAdvertisement(e, i)
			if err != nil {
				h.makeAdvError(errors.Wrap(err, fmt.Sprintf("newAdv (typ %v)", et)), e, true)
				continue
			}

			for idx := h.adLast - 1; idx != h.adLast; idx-- {
				if idx == -1 {
					idx = len(h.adHist) - 1
					if idx == h.adLast {
						break
					}
				}
				if h.adHist[idx] == nil {
					break
				}

				//bad addr?
				addrh, err := h.adHist[idx].addrWErr()
				if err != nil {
					h.makeAdvError(errors.Wrap(err, fmt.Sprintf("adHist addr (typ %v)", et)), e, true)
					break
				}

				//bad addr?
				addrsr, err := sr.addrWErr()
				if err != nil {
					h.makeAdvError(errors.Wrap(err, fmt.Sprintf("srAddr (typ %v)", et)), e, true)
					break
				}

				//set the scan response here
				if addrh.String() == addrsr.String() {
					//this will leave everything alone if there is an error when we attach the scanresp
					err = h.adHist[idx].setScanResponse(sr)
					if err != nil {
						h.makeAdvError(errors.Wrap(err, fmt.Sprintf("setScanResp (typ %v)", et)), e, true)
						break
					}
					a = h.adHist[idx]
					break
				}
			} //for

			// Got a SR without having received an associated AD before?
			if a == nil {
				ee := h.makeAdvError(errors.Wrap(err, fmt.Sprintf("scanRsp (typ %v) w/o associated advData, srAddr %v", et, sr.Addr())), e, true)
				return ee
			}
			// sr

		case evtTypAdvDirectInd: //0x01
			fallthrough
		case evtTypAdvNonconnInd: //0x03
			a, err = newAdvertisement(e, i)
			if err != nil {
				h.makeAdvError(errors.Wrap(err, fmt.Sprintf("newAdv (typ %v)", et)), e, true)
				continue
			}

		default:
			h.makeAdvError(fmt.Errorf("invalid eventType %v", et), e, true)
			continue
		} // switch

		if a == nil {
			h.makeAdvError(fmt.Errorf("nil advertisement (i %v, typ %v)", i, et), e, true)
			continue
		}

		//dispatch
		if h.advHandlerSync {
			h.advHandler(a)
		} else {
			go h.advHandler(a)
		}

	} //for

	return nil
}

func (h *HCI) handleCommandComplete(b []byte) error {
	e := evt.CommandComplete(b)
	h.setAllowedCommands(int(e.NumHCICommandPackets()))

	// NOP command, used for flow control purpose [Vol 2, Part E, 4.4]
	// no handling other than setAllowedCommands needed
	if e.CommandOpcode() == 0x0000 {
		return nil
	}
	h.muSent.Lock()
	p, found := h.sent[int(e.CommandOpcode())]
	h.muSent.Unlock()

	if !found {
		return fmt.Errorf("can't find the cmd for CommandCompleteEP: % X", e)
	}

	select {
	case <-h.done:
		return fmt.Errorf("hci closed")
	case p.done <- e.ReturnParameters():
		return nil
	}
}

func (h *HCI) handleCommandStatus(b []byte) error {
	e := evt.CommandStatus(b)

	if !e.Valid() {
		err := fmt.Errorf("invalid command status: %v", e)
		h.dispatchError(err)
		return err
	}

	h.setAllowedCommands(int(e.NumHCICommandPackets()))

	h.muSent.Lock()
	p, found := h.sent[int(e.CommandOpcode())]
	h.muSent.Unlock()
	if !found {
		return fmt.Errorf("can't find the cmd for CommandStatusEP: % X", e)
	}

	select {
	case <-h.done:
		return fmt.Errorf("hci closed")
	case p.done <- []byte{e.Status()}:
		return nil
	}
}

func (h *HCI) handleLEConnectionComplete(b []byte) error {
	e := evt.LEConnectionComplete(b)

	status := e.Status()
	if status != 0 {
		fmt.Printf("[BLE] connection failed: %s\n", hex.EncodeToString(b))
		return nil
	}
	c := newConn(h, e)
	h.muConns.Lock()
	pa := e.PeerAddress()
	addr := pa[:]
	logger.Debug("hci", fmt.Sprintf("[BLE] connection complete for %04X: addr: %s, lecc evt: %s\n", e.ConnectionHandle(), hex.EncodeToString(addr), hex.EncodeToString(b)))
	h.conns[e.ConnectionHandle()] = c
	h.muConns.Unlock()

	if e.Role() == roleMaster {
		if e.Status() == 0x00 {
			select {
			case h.chMasterConn <- c:
			default:
				go c.Close()
			}
			return nil
		}
		if ErrCommand(e.Status()) == ErrConnID {
			// The connection was canceled successfully.
			return nil
		}
		return nil
	}
	if e.Status() == 0x00 {
		h.chSlaveConn <- c
		// When a controller accepts a connection, it moves from advertising
		// state to idle/ready state. Host needs to explicitly ask the
		// controller to re-enable advertising. Note that the host was most
		// likely in advertising state. Otherwise it couldn't accept the
		// connection in the first place. The only exception is that user
		// asked the host to stop advertising during this tiny window.
		// The re-enabling might failed or ignored by the controller, if
		// it had reached the maximum number of concurrent connections.
		// So we also re-enable the advertising when a connection disconnected
		h.params.RLock()
		if h.params.advEnable.AdvertisingEnable == 1 {
			go h.Send(&h.params.advEnable, nil)
		}
		h.params.RUnlock()
	}
	return nil
}

func (h *HCI) handleLEConnectionUpdateComplete(b []byte) error {
	return nil
}

func (h *HCI) cleanupConnectionHandle(ch uint16) error {

	h.muConns.Lock()
	defer h.muConns.Unlock()
	logger.Debug("hci", "cleanupConnHan: looking for", fmt.Sprintf("%04X", ch))
	c, found := h.conns[ch]
	if !found {
		return nil
		//return fmt.Errorf("disconnecting an invalid handle %04X", ch)
	}

	logger.Debug("hci", "", fmt.Sprintf("clenupConnHan %04X: found device with address %s\n", ch, c.RemoteAddr().String()))

	delete(h.conns, ch)
	logger.Debug("hci", "[BLE] cleanupConnHan close c.chInPkt", fmt.Sprintf("%04X", ch))
	close(c.chInPkt)

	if !h.isOpen() && c.param.Role() == roleSlave {
		// Re-enable advertising, if it was advertising. Refer to the
		// handleLEConnectionComplete() for details.
		// This may failed with ErrCommandDisallowed, if the controller
		// was actually in advertising state. It does no harm though.
		h.params.RLock()
		if h.params.advEnable.AdvertisingEnable == 1 {
			go h.Send(&h.params.advEnable, nil)
		}
		h.params.RUnlock()
	} else {
		// remote peripheral disconnected
		logger.Debug("hci", "cleanupConnHan close c.chDone", fmt.Sprint("%04X", ch))
		close(c.chDone)
	}
	// When a connection disconnects, all the sent packets and weren't acked yet
	// will be recycled. [Vol2, Part E 4.3]
	//
	// must be done with the pool locked to avoid race conditions where
	// writePDU is in progress and does a Get from the pool after this completes,
	// leaking a buffer from the main pool.
	c.txBuffer.LockPool()
	c.txBuffer.PutAll()
	c.txBuffer.UnlockPool()
	return nil
}

func (h *HCI) handleDisconnectionComplete(b []byte) error {
	e := evt.DisconnectionComplete(b)
	ch := e.ConnectionHandle()
	logger.Debug("hci", "[BLE] disconnect complete for handle", fmt.Sprintf("%04x", ch))
	return h.cleanupConnectionHandle(ch)
}

func (h *HCI) handleEncryptionChange(b []byte) error {
	return nil
}

func (h *HCI) handleNumberOfCompletedPackets(b []byte) error {
	e := evt.NumberOfCompletedPackets(b)
	h.muConns.Lock()
	defer h.muConns.Unlock()
	for i := 0; i < int(e.NumberOfHandles()); i++ {
		c, found := h.conns[e.ConnectionHandle(i)]
		if !found {
			continue
		}

		// Put the delivered buffers back to the pool.
		for j := 0; j < int(e.HCNumOfCompletedPackets(i)); j++ {
			c.txBuffer.Put()
		}
	}
	return nil
}

func (h *HCI) handleLELongTermKeyRequest(b []byte) error {
	//todo: probably need to support this
	e := evt.LELongTermKeyRequest(b)
	panic(nil)
	return h.Send(&cmd.LELongTermKeyRequestNegativeReply{
		ConnectionHandle: e.ConnectionHandle(),
	}, nil)
}

func (h *HCI) setAllowedCommands(n int) {
	if n > chCmdBufChanSize {
		fmt.Printf("hci.setAllowedCommands: warning, defaulting %d -> %d\n", n, chCmdBufChanSize)
		n = chCmdBufChanSize
	}

	//put with timeout
	for len(h.chCmdBufs) < n {
		select {
		case <-h.done:
			//closed
			return
		case h.chCmdBufs <- make([]byte, chCmdBufElementSize):
			//ok
		case <-time.After(chCmdBufTimeout):
			h.dispatchError(fmt.Errorf("chCmdBufs put timeout"))
			//timeout
			break
		}
	}
}

func (h *HCI) dispatchError(e error) {
	switch {
	case h.errorHandler == nil:
		fmt.Println(e)
	case !h.isOpen():
		//don't dispatch
		fmt.Println("hci closing:", e)
	default:
		h.errorHandler(e)
	}
}

// workaround, remove
func (h *HCI) NOP() error {
	return nil

	// ReadBDADDRRP := cmd.ReadBDADDRRP{}
	// err := h.Send(&cmd.ReadBDADDR{}, &ReadBDADDRRP)
	// fmt.Println("NOP: err ", err)
	// return err
}

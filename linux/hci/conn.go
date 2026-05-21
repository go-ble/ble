package hci

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/go-ble/ble"
	"github.com/go-ble/ble/linux/hci/cmd"
	"github.com/go-ble/ble/linux/hci/evt"
	"github.com/pkg/errors"
)

// Conn ...
type Conn struct {
	hci *HCI
	ctx context.Context

	param evt.LEConnectionComplete

	// While MTU is the maximum size of payload data that the upper layer (ATT)
	// can accept, the MPS is the maximum PDU payload size this L2CAP implementation
	// supports. When segmantation is not used, the MPS should be made to the same
	// values of MTUs [Vol 3, Part A, 1.4].
	//
	// For LE-U logical transport, the L2CAP implementations should support
	// a minimum of 23 bytes, which are also the default values before the
	// upper layer (ATT) optionally reconfigures them [Vol 3, Part A, 3.2.8].
	rxMTU int
	txMTU int
	rxMPS int

	// Signaling MTUs are The maximum size of command information that the
	// L2CAP layer entity is capable of accepting.
	// A L2CAP implementations supporting LE-U should support at least 23 bytes.
	// Currently, we support 512 bytes, which should be more than sufficient.
	// The sigTxMTU is discovered via when we sent a signaling pkt that is
	// larger thean the remote device can handle, and get a response of "Command
	// Reject" indicating "Signaling MTU exceeded" along with the actual
	// signaling MTU [Vol 3, Part A, 4.1].
	sigRxMTU int
	sigTxMTU int

	sigSent chan []byte
	// smpSent chan []byte

	chInPkt chan packet
	chInPDU chan pdu

	chDone chan struct{}
	// Host to Controller Data Flow Control pkt-based Data flow control for LE-U [Vol 2, Part E, 4.1.1]
	// chSentBufs tracks the HCI buffer occupied by this connection.
	txBuffer *Client

	// sigID is used to match responses with signaling requests.
	// The requesting device sets this field and the responding device uses the
	// same value in its response. Within each signalling channel a different
	// Identifier shall be used for each successive command. [Vol 3, Part A, 4]
	sigID uint8

	// leFrame is set to be true when the LE Credit based flow control is used.
	leFrame bool
}

func newConn(h *HCI, param evt.LEConnectionComplete) *Conn {
	c := &Conn{
		hci:   h,
		ctx:   context.Background(),
		param: param,

		rxMTU: ble.DefaultMTU,
		txMTU: ble.DefaultMTU,

		rxMPS: ble.DefaultMTU,

		sigRxMTU: ble.MaxMTU,
		sigTxMTU: ble.DefaultMTU,

		chInPkt: make(chan packet, 16),
		chInPDU: make(chan pdu, 16),

		txBuffer: NewClient(h.pool),

		chDone: make(chan struct{}),
	}

	go func() {
		for {
			if err := c.recombine(); err != nil {
				if err != io.EOF {
					// TODO: wrap and pass the error up.
					// err := errors.Wrap(err, "recombine failed")
					_ = logger.Error("recombine failed: ", "err", err)
				}
				close(c.chInPDU)
				return
			}
		}
	}()
	return c
}

// Context returns the context that is used by this Conn.
func (c *Conn) Context() context.Context {
	return c.ctx
}

// SetContext sets the context that is used by this Conn.
func (c *Conn) SetContext(ctx context.Context) {
	c.ctx = ctx
}

// Read copies re-assembled L2CAP PDUs into sdu.
func (c *Conn) Read(sdu []byte) (n int, err error) {
	p, ok := <-c.chInPDU
	if !ok {
		return 0, errors.Wrap(io.ErrClosedPipe, "input channel closed")
	}
	if len(p) == 0 {
		return 0, errors.Wrap(io.ErrUnexpectedEOF, "received empty packet")
	}

	// Assume it's a B-Frame.
	slen := p.dlen()
	data := p.payload()
	if c.leFrame {
		// LE-Frame.
		slen = leFrameHdr(p).slen()
		data = leFrameHdr(p).payload()
	}
	if cap(sdu) < slen {
		return 0, errors.Wrapf(io.ErrShortBuffer, "payload received exceeds sdu buffer")
	}
	buf := bytes.NewBuffer(sdu)
	buf.Reset()
	buf.Write(data)
	for buf.Len() < slen {
		p := <-c.chInPDU
		buf.Write(p.payload())
	}
	return slen, nil
}

// Write breaks down a L2CAP SDU into segmants [Vol 3, Part A, 7.3.1]
func (c *Conn) Write(sdu []byte) (int, error) {
	if len(sdu) > c.txMTU {
		return 0, errors.Wrap(io.ErrShortWrite, "payload exceeds mtu")
	}

	plen := len(sdu)
	if plen > c.txMTU {
		plen = c.txMTU
	}
	b := make([]byte, 4+plen)
	binary.LittleEndian.PutUint16(b[0:2], uint16(len(sdu)))
	binary.LittleEndian.PutUint16(b[2:4], cidLEAtt)
	if c.leFrame {
		binary.LittleEndian.PutUint16(b[4:6], uint16(len(sdu)))
		copy(b[6:], sdu)
	} else {
		copy(b[4:], sdu)
	}
	sent, err := c.writePDU(b)
	if err != nil {
		return sent, err
	}
	sdu = sdu[plen:]

	for len(sdu) > 0 {
		plen := len(sdu)
		if plen > c.txMTU {
			plen = c.txMTU
		}
		n, err := c.writePDU(sdu[:plen])
		sent += n
		if err != nil {
			return sent, err
		}
		sdu = sdu[plen:]
	}
	return sent, nil
}

// writePDU breaks down a L2CAP PDU into fragments if it's larger than the HCI buffer size. [Vol 3, Part A, 7.2.1]
func (c *Conn) writePDU(pdu []byte) (int, error) {
	sent := 0
	flags := uint16(pbfHostToControllerStart << 4) // ACL boundary flags

	// All L2CAP fragments associated with an L2CAP PDU shall be processed for
	// transmission by the Controller before any other L2CAP PDU for the same
	// logical transport shall be processed.
	c.txBuffer.LockPool()
	defer c.txBuffer.UnlockPool()

	// Fail immediately if the connection is already closed
	// Check this with the pool locked to avoid race conditions
	// with handleDisconnectionComplete
	select {
	case <-c.chDone:
		return 0, io.ErrClosedPipe
	default:
	}

	for len(pdu) > 0 {
		// Get a buffer from our pre-allocated and flow-controlled pool.
		pkt := c.txBuffer.Get() // ACL pkt
		flen := len(pdu)        // fragment length
		if flen > pkt.Cap()-1-4 {
			flen = pkt.Cap() - 1 - 4
		}

		// Prepare the Headers

		// HCI Header: pkt Type
		if err := binary.Write(pkt, binary.LittleEndian, pktTypeACLData); err != nil {
			return 0, err
		}
		// ACL Header: handle and flags
		if err := binary.Write(pkt, binary.LittleEndian, c.param.ConnectionHandle()|(flags<<8)); err != nil {
			return 0, err
		}
		// ACL Header: data len
		if err := binary.Write(pkt, binary.LittleEndian, uint16(flen)); err != nil {
			return 0, err
		}
		// Append payload
		if err := binary.Write(pkt, binary.LittleEndian, pdu[:flen]); err != nil {
			return 0, err
		}

		// Flush the pkt to HCI
		select {
		case <-c.chDone:
			return 0, io.ErrClosedPipe
		default:
		}

		if _, err := c.hci.skt.Write(pkt.Bytes()); err != nil {
			return sent, err
		}
		sent += flen

		flags = (pbfContinuing << 4) // Set "continuing" in the boundary flags for the rest of fragments, if any.
		pdu = pdu[flen:]             // Advence the point
	}
	return sent, nil
}

// Recombines fragments into a L2CAP PDU. [Vol 3, Part A, 7.2.2]
func (c *Conn) recombine() error {
	pkt, ok := <-c.chInPkt
	if !ok {
		return io.EOF
	}

	p := pdu(pkt.data())

	// Currently, check for LE-U only. For channels that we don't recognizes,
	// re-combine them anyway, and discard them later when we dispatch the PDU
	// according to CID.
	if p.cid() == cidLEAtt && p.dlen() > c.rxMPS {
		return fmt.Errorf("fragment size (%d) larger than rxMPS (%d)", p.dlen(), c.rxMPS)
	}

	// If this pkt is not a complete PDU, and we'll be receiving more
	// fragments, re-allocate the whole PDU (including Header).
	if len(p.payload()) < p.dlen() {
		p = make([]byte, 0, 4+p.dlen())
		p = append(p, pdu(pkt.data())...)
	}
	for len(p) < 4+p.dlen() {
		if pkt, ok = <-c.chInPkt; !ok || (pkt.pbf()&pbfContinuing) == 0 {
			return io.ErrUnexpectedEOF
		}
		p = append(p, pdu(pkt.data())...)
	}

	// TODO: support dynamic or assigned channels for LE-Frames.
	switch p.cid() {
	case cidLEAtt:
		c.chInPDU <- p
	case cidLESignal:
		c.handleSignal(p)
	case cidSMP:
		c.handleSMP(p)
	default:
		logger.Info("recombine()", "unrecognized CID", fmt.Sprintf("%04X, [%X]", p.cid(), p))
	}
	return nil
}

// Disconnected returns a receiving channel, which is closed when the connection disconnects.
func (c *Conn) Disconnected() <-chan struct{} {
	return c.chDone
}

// ReadRSSI retrieves the current RSSI value of remote peripheral. [Vol 2, Part E, 7.5.4]
func (c *Conn) ReadRSSI() int {
	rp := new(cmd.ReadRSSIRP)
	c.hci.Send(&cmd.ReadRSSI{
		Handle: c.param.ConnectionHandle(),
	}, rp)
	return int(rp.RSSI)
}

// ReadPHY retrieves the current PHY settings of the connection. [Vol 2, Part E, 7.8.47]
func (c *Conn) ReadPHY() (int, int, error) {
	rp := new(cmd.LEReadPHYRP)
	err := c.hci.Send(&cmd.LEReadPHY{
		ConnectionHandle: c.param.ConnectionHandle(),
	}, rp)
	return int(rp.TXPHY), int(rp.RXPHY), err
}

// SetPHY sets the PHY for the connection. [Vol 2, Part E, 7.8.49]
func (c *Conn) SetPHY(txPhy int, rxPhy int) error {
	return c.hci.Send(&cmd.LESetPHY{
		ConnectionHandle: c.param.ConnectionHandle(),
		ALLPHYS:          0,
		TXPHYS:           uint8(txPhy),
		RXPHYS:           uint8(rxPhy),
		PHYOptions:       0,
	}, nil)
}

// UpdateConnection updates the connection parameters. [Vol 2, Part E, 7.8.18]
func (c *Conn) UpdateConnection(intervalMin time.Duration, intervalMax time.Duration, latency int, supervisionTimeout time.Duration) error {
	min := uint16(intervalMin / (1250 * time.Microsecond))
	max := uint16(intervalMax / (1250 * time.Microsecond))
	timeout := uint16(supervisionTimeout / (10 * time.Millisecond))

	if c.param.Role() != roleMaster {
		// As a slave, we should use the L2CAP Connection Parameter Update Request
		req := &ConnectionParameterUpdateRequest{
			IntervalMin:       min,
			IntervalMax:       max,
			SlaveLatency:      uint16(latency),
			TimeoutMultiplier: timeout,
		}
		rsp := &ConnectionParameterUpdateResponse{}
		if err := c.Signal(req, rsp); err != nil {
			return err
		}
		if rsp.Result != 0 {
			return fmt.Errorf("connection parameter update rejected: 0x%04x", rsp.Result)
		}
		return nil
	}

	return c.hci.Send(&cmd.LEConnectionUpdate{
		ConnectionHandle:   c.param.ConnectionHandle(),
		ConnIntervalMin:    min,
		ConnIntervalMax:    max,
		ConnLatency:        uint16(latency),
		SupervisionTimeout: timeout,
		MinimumCELength:    0x0000,
		MaximumCELength:    0x0000,
	}, nil)
}

// Close disconnects the connection by sending hci disconnect command to the device.
func (c *Conn) Close() error {
	select {
	case <-c.chDone:
		// Return if it's already closed.
		return nil
	default:
		c.hci.Send(&cmd.Disconnect{
			ConnectionHandle: c.param.ConnectionHandle(),
			Reason:           0x13,
		}, nil)
		return nil
	}
}

// LocalAddr returns local device's MAC address.
func (c *Conn) LocalAddr() ble.Addr { return c.hci.Addr() }

// RemoteAddr returns remote device's MAC address.
func (c *Conn) RemoteAddr() ble.Addr {
	a := c.param.PeerAddress()
	return net.HardwareAddr([]byte{a[5], a[4], a[3], a[2], a[1], a[0]})
}

// RxMTU returns the MTU which the upper layer is capable of accepting.
func (c *Conn) RxMTU() int { return c.rxMTU }

// SetRxMTU sets the MTU which the upper layer is capable of accepting.
func (c *Conn) SetRxMTU(mtu int) { c.rxMTU, c.rxMPS = mtu, mtu }

// TxMTU returns the MTU which the remote device is capable of accepting.
func (c *Conn) TxMTU() int { return c.txMTU }

// SetTxMTU sets the MTU which the remote device is capable of accepting.
func (c *Conn) SetTxMTU(mtu int) { c.txMTU = mtu }

// pkt implements HCI ACL Data Packet [Vol 2, Part E, 5.4.2]
// Packet boundary flags , bit[5:6] of handle field's MSB
// Broadcast flags. bit[7:8] of handle field's MSB
// Not used in LE-U. Leave it as 0x00 (Point-to-Point).
// Broadcasting in LE uses ADVB logical transport.
type packet []byte

func (a packet) handle() uint16 { return uint16(a[0]) | (uint16(a[1]&0x0f) << 8) }
func (a packet) pbf() int       { return (int(a[1]) >> 4) & 0x3 }
func (a packet) bcf() int       { return (int(a[1]) >> 6) & 0x3 }
func (a packet) dlen() int      { return int(a[2]) | (int(a[3]) << 8) }
func (a packet) data() []byte   { return a[4:] }

type pdu []byte

func (p pdu) dlen() int       { return int(binary.LittleEndian.Uint16(p[0:2])) }
func (p pdu) cid() uint16     { return binary.LittleEndian.Uint16(p[2:4]) }
func (p pdu) payload() []byte { return p[4:] }

type leFrameHdr pdu

func (f leFrameHdr) slen() int       { return int(binary.LittleEndian.Uint16(f[4:6])) }
func (f leFrameHdr) payload() []byte { return f[6:] }

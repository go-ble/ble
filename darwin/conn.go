package darwin

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/JuulLabs-OSS/cbgo"
	"github.com/go-ble/ble"
)

// newGenConn creates a new generic (role-less) connection.  This should not be
// called directly; use newCentralConn or newPeripheralConn instead.
func newGenConn(d *Device, a ble.Addr) (*conn, error) {
	c := &conn{
		dev:   d,
		rxMTU: 23,
		txMTU: 23,
		addr:  a,
		done:  make(chan struct{}),

		notifiers: make(map[cbgo.Characteristic]ble.Notifier),

		subs:     make(map[string]*sub),
		chrReads: make(map[string]chan error),
	}

	err := d.addConn(c)
	if err != nil {
		return nil, err
	}

	go func() {
		<-c.done
		d.delConn(c.addr)
	}()

	return c, nil
}

// newCentralConn creates a new connection with us acting as central
// (peer=peripheral).
func newCentralConn(d *Device, prph cbgo.Peripheral) (*conn, error) {
	c, err := newGenConn(d, ble.NewAddr(prph.Identifier().String()))
	if err != nil {
		return nil, err
	}

	// -3 to account for WriteCommand base.
	c.txMTU = prph.MaximumWriteValueLength(false) - 3
	c.prph = prph

	return c, nil
}

// newCentralConn creates a new connection with us acting as peripheral
// (peer=central).
func newPeripheralConn(d *Device, cent cbgo.Central) (*conn, error) {
	c, err := newGenConn(d, ble.NewAddr(cent.Identifier().String()))
	if err != nil {
		return nil, err
	}

	// -3 to account for ATT_HANDLE_VALUE_NTF base.
	c.txMTU = cent.MaximumUpdateValueLength() - 3
	c.cent = cent

	return c, nil
}

type conn struct {
	sync.RWMutex

	dev   *Device
	ctx   context.Context
	rxMTU int
	txMTU int
	addr  ble.Addr
	done  chan struct{}

	evl clientEventListener

	prph cbgo.Peripheral
	cent cbgo.Central

	notifiers map[cbgo.Characteristic]ble.Notifier // central connection only

	subs     map[string]*sub
	chrReads map[string](chan error)
}

func (c *conn) Context() context.Context {
	return c.ctx
}

func (c *conn) SetContext(ctx context.Context) {
	c.ctx = ctx
}

func (c *conn) LocalAddr() ble.Addr {
	// return c.dev.Address()
	return c.addr // FIXME
}

func (c *conn) RemoteAddr() ble.Addr {
	return c.addr
}

func (c *conn) RxMTU() int {
	return c.rxMTU
}

func (c *conn) SetRxMTU(mtu int) {
	c.rxMTU = mtu
}

func (c *conn) TxMTU() int {
	return c.txMTU
}

func (c *conn) SetTxMTU(mtu int) {
	c.Lock()
	c.txMTU = mtu
	c.Unlock()
}

func (c *conn) Read(b []byte) (int, error) {
	return 0, nil
}
func (c *conn) Write(b []byte) (int, error) {
	return 0, nil
}

func (c *conn) Close() error {
	c.evl.Close()
	return nil
}

// Disconnected returns a receiving channel, which is closed when the connection disconnects.
func (c *conn) Disconnected() <-chan struct{} {
	return c.done
}

// ReadRSSI retrieves the current RSSI value of remote peripheral. [Vol 2, Part E, 7.5.4]
func (c *conn) ReadRSSI() int {
	ch := c.evl.rssiRead.Listen()
	defer c.evl.rssiRead.Close()

	c.prph.ReadRSSI()

	select {
	case itf := <-ch:
		ev := itf.(*eventRSSIRead)
		if ev.err != nil {
			return 0
		}
		return ev.rssi
	}
}

// processChrRead handles an incoming read response.  CoreBluetooth does not
// distinguish explicit reads from unsolicited notifications.  This function
// identifies which type the incoming message is.
func (c *conn) processChrRead(err error, cbchr cbgo.Characteristic) {
	c.RLock()
	defer c.RUnlock()

	uuidStr := uuidStrWithDashes(cbchr.UUID().String())
	found := false

	ch := c.chrReads[uuidStr]
	if ch != nil {
		ch <- err
		found = true
	}

	// A failed read carries no value for the subscriber.
	s := c.subs[uuidStr]
	if s != nil && err == nil {
		s.fn(cbchr.Value())
		found = true
	}

	if !found {
		log.Printf("received characteristic read response without corresponding request: uuid=%s", uuidStr)
	}
}

// addChrReader starts listening for a solicited read response.
func (c *conn) addChrReader(char *ble.Characteristic) (chan error, error) {
	uuidStr := uuidStrWithDashes(char.UUID.String())

	c.Lock()
	defer c.Unlock()

	if c.chrReads[uuidStr] != nil {
		return nil, fmt.Errorf("cannot read from the same attribute twice: uuid=%s", uuidStr)
	}

	ch := make(chan error)
	c.chrReads[uuidStr] = ch

	return ch, nil
}

// delChrReader stops listening for a solicited read response.
func (c *conn) delChrReader(char *ble.Characteristic) {
	c.Lock()
	defer c.Unlock()

	uuidStr := uuidStrWithDashes(char.UUID.String())
	delete(c.chrReads, uuidStr)
}

// addSub starts listening for unsolicited notifications and indications for a
// particular characteristic.
func (c *conn) addSub(char *ble.Characteristic, fn ble.NotificationHandler) {
	uuidStr := uuidStrWithDashes(char.UUID.String())

	c.Lock()
	defer c.Unlock()

	// It feels like we should return an error if we are already subscribed to
	// this characteristic.  Just quietly overwrite the existing handler to
	// preserve backwards compatibility.

	c.subs[uuidStr] = &sub{
		fn:   fn,
		char: char,
	}
}

// delSub stops listening for unsolicited notifications and indications for a
// particular characteristic.
func (c *conn) delSub(char *ble.Characteristic) {
	uuidStr := uuidStrWithDashes(char.UUID.String())

	c.Lock()
	defer c.Unlock()

	delete(c.subs, uuidStr)
}

package darwin

import (
	"fmt"

	"github.com/go-ble/ble"
	"github.com/JuulLabs-OSS/cbgo"
)

// A Client is a GATT client.
type Client struct {
	cbgo.PeripheralDelegateBase

	profile *ble.Profile
	pc      profCache
	name    string
	cm      cbgo.CentralManager

	id   ble.UUID
	conn *conn
}

// NewClient ...
func NewClient(cm cbgo.CentralManager, c ble.Conn) (*Client, error) {
	as := c.RemoteAddr().String()
	id, err := ble.Parse(as)
	if err != nil {
		return nil, fmt.Errorf("connection has invalid address: addr=%s", as)
	}

	cln := &Client{
		conn: c.(*conn),
		pc:   newProfCache(),
		cm:   cm,
		id:   id,
	}

	cln.conn.prph.SetDelegate(cln)

	return cln, nil
}

// Addr returns UUID of the remote peripheral.
func (cln *Client) Addr() ble.Addr {
	return cln.conn.RemoteAddr()
}

// Name returns the name of the remote peripheral.
// This can be the advertised name, if exists, or the GAP device name, which takes priority.
func (cln *Client) Name() string {
	return cln.name
}

// Profile returns the discovered profile.
func (cln *Client) Profile() *ble.Profile {
	return cln.profile
}

// DiscoverProfile discovers the whole hierarchy of a server.
func (cln *Client) DiscoverProfile(force bool) (*ble.Profile, error) {
	if cln.profile != nil && !force {
		return cln.profile, nil
	}
	ss, err := cln.DiscoverServices(nil)
	if err != nil {
		return nil, fmt.Errorf("can't discover services: %s", err)
	}
	for _, s := range ss {
		cs, err := cln.DiscoverCharacteristics(nil, s)
		if err != nil {
			return nil, fmt.Errorf("can't discover characteristics: %s", err)
		}
		for _, c := range cs {
			_, err := cln.DiscoverDescriptors(nil, c)
			if err != nil {
				return nil, fmt.Errorf("can't discover descriptors: %s", err)
			}
		}
	}
	cln.profile = &ble.Profile{Services: ss}
	return cln.profile, nil
}

// DiscoverServices finds all the primary services on a server. [Vol 3, Part G, 4.4.1]
// If filter is specified, only filtered services are returned.
func (cln *Client) DiscoverServices(ss []ble.UUID) ([]*ble.Service, error) {
	ch := cln.conn.evl.svcsDiscovered.Listen()
	defer cln.conn.evl.svcsDiscovered.Close()

	cbuuids := uuidsToCbgoUUIDs(ss)
	cln.conn.prph.DiscoverServices(cbuuids)

	select {
	case itf := <-ch:
		if itf != nil {
			return nil, itf.(error)
		}

	case <-cln.Disconnected():
		return nil, fmt.Errorf("disconnected")
	}

	svcs := []*ble.Service{}
	for _, dsvc := range cln.conn.prph.Services() {
		svc := &ble.Service{
			UUID: ble.UUID(dsvc.UUID()),
		}
		cln.pc.addSvc(svc, dsvc)
		svcs = append(svcs, svc)
	}
	if cln.profile == nil {
		cln.profile = &ble.Profile{Services: svcs}
	}
	return svcs, nil
}

// DiscoverIncludedServices finds the included services of a service. [Vol 3, Part G, 4.5.1]
// If filter is specified, only filtered services are returned.
func (cln *Client) DiscoverIncludedServices(ss []ble.UUID, s *ble.Service) ([]*ble.Service, error) {
	return nil, ble.ErrNotImplemented
}

// DiscoverCharacteristics finds all the characteristics within a service. [Vol 3, Part G, 4.6.1]
// If filter is specified, only filtered characteristics are returned.
func (cln *Client) DiscoverCharacteristics(cs []ble.UUID, s *ble.Service) ([]*ble.Characteristic, error) {
	cbsvc, err := cln.pc.findCbSvc(s)
	if err != nil {
		return nil, err
	}

	ch := cln.conn.evl.chrsDiscovered.Listen()
	defer cln.conn.evl.chrsDiscovered.Close()

	cbuuids := uuidsToCbgoUUIDs(cs)
	cln.conn.prph.DiscoverCharacteristics(cbuuids, cbsvc)

	select {
	case itf := <-ch:
		if itf != nil {
			return nil, itf.(error)
		}

	case <-cln.Disconnected():
		return nil, fmt.Errorf("disconnected")
	}

	for _, dchr := range cbsvc.Characteristics() {
		chr := &ble.Characteristic{
			UUID:     ble.UUID(dchr.UUID()),
			Property: ble.Property(dchr.Properties()),
		}
		cln.pc.addChr(chr, dchr)
		s.Characteristics = append(s.Characteristics, chr)
	}
	return s.Characteristics, nil
}

// DiscoverDescriptors finds all the descriptors within a characteristic. [Vol 3, Part G, 4.7.1]
// If filter is specified, only filtered descriptors are returned.
func (cln *Client) DiscoverDescriptors(ds []ble.UUID, c *ble.Characteristic) ([]*ble.Descriptor, error) {
	cbchr, err := cln.pc.findCbChr(c)
	if err != nil {
		return nil, err
	}

	ch := cln.conn.evl.dscsDiscovered.Listen()
	defer cln.conn.evl.dscsDiscovered.Close()

	cln.conn.prph.DiscoverDescriptors(cbchr)
	if err != nil {
		return nil, err
	}

	select {
	case itf := <-ch:
		if itf != nil {
			return nil, itf.(error)
		}

	case <-cln.Disconnected():
		return nil, fmt.Errorf("disconnected")
	}

	for _, ddsc := range cbchr.Descriptors() {
		dsc := &ble.Descriptor{
			UUID: ble.UUID(ddsc.UUID()),
		}
		c.Descriptors = append(c.Descriptors, dsc)
		cln.pc.addDsc(dsc, ddsc)
	}
	return c.Descriptors, nil
}

// ReadCharacteristic reads a characteristic value from a server. [Vol 3, Part G, 4.8.1]
func (cln *Client) ReadCharacteristic(c *ble.Characteristic) ([]byte, error) {
	cbchr, err := cln.pc.findCbChr(c)
	if err != nil {
		return nil, err
	}

	ch, err := cln.conn.addChrReader(c)
	if err != nil {
		return nil, fmt.Errorf("failed to read characteristic: %v", err)
	}
	defer cln.conn.delChrReader(c)

	cln.conn.prph.ReadCharacteristic(cbchr)

	select {
	case itf := <-ch:
		if itf != nil {
			return nil, itf.(error)
		}

	case <-cln.Disconnected():
		return nil, fmt.Errorf("disconnected")
	}

	c.Value = cbchr.Value()

	return c.Value, nil
}

// ReadLongCharacteristic reads a characteristic value which is longer than the MTU. [Vol 3, Part G, 4.8.3]
func (cln *Client) ReadLongCharacteristic(c *ble.Characteristic) ([]byte, error) {
	return cln.ReadCharacteristic(c)
}

// WriteCharacteristic writes a characteristic value to a server. [Vol 3, Part G, 4.9.3]
func (cln *Client) WriteCharacteristic(c *ble.Characteristic, b []byte, noRsp bool) error {
	cbchr, err := cln.pc.findCbChr(c)
	if err != nil {
		return err
	}

	if noRsp {
		cln.conn.prph.WriteCharacteristic(b, cbchr, false)
		return nil
	}

	ch := cln.conn.evl.chrWritten.Listen()
	defer cln.conn.evl.chrWritten.Close()

	cln.conn.prph.WriteCharacteristic(b, cbchr, true)

	select {
	case itf := <-ch:
		if itf != nil {
			return itf.(error)
		}

	case <-cln.Disconnected():
		return fmt.Errorf("disconnected")
	}

	return nil
}

// WriteLongCharacteristic writes a characteristic value that is longer than the MTU to a server. [Vol 3, Part F, 3.4.6]
func (cln *Client) WriteLongCharacteristic(c *ble.Characteristic, b []byte) error {
	panic("not implemented")
}

// ReadDescriptor reads a characteristic descriptor from a server. [Vol 3, Part G, 4.12.1]
func (cln *Client) ReadDescriptor(d *ble.Descriptor) ([]byte, error) {
	cbdsc, err := cln.pc.findCbDsc(d)
	if err != nil {
		return nil, err
	}

	ch := cln.conn.evl.dscRead.Listen()
	defer cln.conn.evl.dscRead.Close()

	cln.conn.prph.ReadDescriptor(cbdsc)

	select {
	case itf := <-ch:
		if itf != nil {
			return nil, itf.(error)
		}

	case <-cln.Disconnected():
		return nil, fmt.Errorf("disconnected")
	}

	d.Value = cbdsc.Value()

	return d.Value, nil
}

// WriteDescriptor writes a characteristic descriptor to a server. [Vol 3, Part G, 4.12.3]
func (cln *Client) WriteDescriptor(d *ble.Descriptor, b []byte) error {
	cbdsc, err := cln.pc.findCbDsc(d)
	if err != nil {
		return err
	}

	ch := cln.conn.evl.dscWritten.Listen()
	defer cln.conn.evl.dscWritten.Close()

	cln.conn.prph.WriteDescriptor(b, cbdsc)
	if err != nil {
		return err
	}

	select {
	case itf := <-ch:
		if itf != nil {
			return itf.(error)
		}

	case <-cln.Disconnected():
		return fmt.Errorf("disconnected")
	}

	return nil
}

// ReadRSSI retrieves the current RSSI value of remote peripheral. [Vol 2, Part E, 7.5.4]
func (cln *Client) ReadRSSI() int {
	ch := cln.conn.evl.rssiRead.Listen()
	defer cln.conn.evl.rssiRead.Close()

	cln.conn.prph.ReadRSSI()

	select {
	case itf := <-ch:
		ev := itf.(*eventRSSIRead)
		if ev.err != nil {
			return 0
		}
		return ev.rssi

	case <-cln.Disconnected():
		return 0
	}
}

// ExchangeMTU set the ATT_MTU to the maximum possible value that can be
// supported by both devices [Vol 3, Part G, 4.3.1]
func (cln *Client) ExchangeMTU(mtu int) (int, error) {
	// TODO: find the xpc command to tell OS X the rxMTU we can handle.
	return cln.conn.TxMTU(), nil
}

// Subscribe subscribes to indication (if ind is set true), or notification of a
// characteristic value. [Vol 3, Part G, 4.10 & 4.11]
func (cln *Client) Subscribe(c *ble.Characteristic, ind bool, fn ble.NotificationHandler) error {
	cbchr, err := cln.pc.findCbChr(c)
	if err != nil {
		return err
	}

	cln.conn.addSub(c, fn)

	ch := cln.conn.evl.notifyChanged.Listen()
	defer cln.conn.evl.notifyChanged.Close()

	cln.conn.prph.SetNotify(true, cbchr)

	select {
	case itf := <-ch:
		if itf != nil {
			cln.conn.delSub(c)
			return itf.(error)
		}

	case <-cln.Disconnected():
		cln.conn.delSub(c)
		return fmt.Errorf("disconnected")
	}

	return nil
}

// Unsubscribe unsubscribes to indication (if ind is set true), or notification
// of a specified characteristic value. [Vol 3, Part G, 4.10 & 4.11]
func (cln *Client) Unsubscribe(c *ble.Characteristic, ind bool) error {
	cbchr, err := cln.pc.findCbChr(c)
	if err != nil {
		return err
	}

	ch := cln.conn.evl.notifyChanged.Listen()
	defer cln.conn.evl.notifyChanged.Close()

	cln.conn.prph.SetNotify(false, cbchr)

	select {
	case itf := <-ch:
		if itf != nil {
			return itf.(error)
		}

	case <-cln.Disconnected():
		return fmt.Errorf("disconnected")
	}

	cln.conn.delSub(c)

	return nil
}

// ClearSubscriptions clears all subscriptions to notifications and indications.
func (cln *Client) ClearSubscriptions() error {
	for _, s := range cln.conn.subs {
		if err := cln.Unsubscribe(s.char, false); err != nil {
			return err
		}
	}
	return nil
}

// CancelConnection disconnects the connection.
func (cln *Client) CancelConnection() error {
	cln.cm.CancelConnect(cln.conn.prph)
	return nil
}

// Disconnected returns a receiving channel, which is closed when the client disconnects.
func (cln *Client) Disconnected() <-chan struct{} {
	return cln.conn.Disconnected()
}

// Conn returns the client's current connection.
func (cln *Client) Conn() ble.Conn {
	return cln.conn
}

type sub struct {
	fn   ble.NotificationHandler
	char *ble.Characteristic
}

func (cln *Client) DidDiscoverServices(prph cbgo.Peripheral, err error) {
	cln.conn.evl.svcsDiscovered.RxSignal(err)
}
func (cln *Client) DidDiscoverCharacteristics(prph cbgo.Peripheral, svc cbgo.Service, err error) {
	cln.conn.evl.chrsDiscovered.RxSignal(err)
}
func (cln *Client) DidDiscoverDescriptors(prph cbgo.Peripheral, chr cbgo.Characteristic, err error) {
	cln.conn.evl.dscsDiscovered.RxSignal(err)
}
func (cln *Client) DidUpdateValueForCharacteristic(prph cbgo.Peripheral, chr cbgo.Characteristic, err error) {
	cln.conn.processChrRead(err, chr)
}

func (cln *Client) DidUpdateValueForDescriptor(prph cbgo.Peripheral, dsc cbgo.Descriptor, err error) {
	cln.conn.evl.dscRead.RxSignal(err)
}
func (cln *Client) DidWriteValueForCharacteristic(prph cbgo.Peripheral, chr cbgo.Characteristic, err error) {
	cln.conn.evl.chrWritten.RxSignal(err)
}
func (cln *Client) DidWriteValueForDescriptor(prph cbgo.Peripheral, dsc cbgo.Descriptor, err error) {
	cln.conn.evl.dscWritten.RxSignal(err)
}
func (cln *Client) DidUpdateNotificationState(prph cbgo.Peripheral, chr cbgo.Characteristic, err error) {
	cln.conn.evl.notifyChanged.RxSignal(err)
}
func (cln *Client) DidReadRSSI(prph cbgo.Peripheral, rssi int, err error) {
	cln.conn.evl.rssiRead.RxSignal(&eventRSSIRead{
		err:  err,
		rssi: int(rssi),
	})
}

package darwin

import (
	"fmt"

	"github.com/go-ble/ble"
	"github.com/raff/goble/xpc"
)

// A Client is a GATT client.
type Client struct {
	profile *ble.Profile
	name    string

	id   xpc.UUID
	conn *conn
}

// NewClient ...
func NewClient(c ble.Conn) (*Client, error) {
	return &Client{
		conn: c.(*conn),
		id:   xpc.MakeUUID(c.RemoteAddr().String()),
	}, nil
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
	rsp, err := cln.conn.sendReq(cmdDiscoverServices, xpc.Dict{
		"kCBMsgArgDeviceUUID": cln.id,
		"kCBMsgArgUUIDs":      uuidSlice(ss),
	})
	if err != nil {
		return nil, err
	}
	if err := rsp.err(); err != nil {
		return nil, err
	}
	svcs := []*ble.Service{}
	for _, xss := range rsp.services() {
		xs := msg(xss.(xpc.Dict))
		svcs = append(svcs, &ble.Service{
			UUID:      ble.MustParse(xs.uuid()),
			Handle:    uint16(xs.serviceStartHandle()),
			EndHandle: uint16(xs.serviceEndHandle()),
		})
	}
	if cln.profile == nil {
		cln.profile = &ble.Profile{Services: svcs}
	}
	return svcs, nil
}

// DiscoverIncludedServices finds the included services of a service. [Vol 3, Part G, 4.5.1]
// If filter is specified, only filtered services are returned.
func (cln *Client) DiscoverIncludedServices(ss []ble.UUID, s *ble.Service) ([]*ble.Service, error) {
	rsp, err := cln.conn.sendReq(cmdDiscoverIncludedServices, xpc.Dict{
		"kCBMsgArgDeviceUUID":         cln.id,
		"kCBMsgArgServiceStartHandle": s.Handle,
		"kCBMsgArgServiceEndHandle":   s.EndHandle,
		"kCBMsgArgUUIDs":              uuidSlice(ss),
	})
	if err != nil {
		return nil, err
	}
	if err := rsp.err(); err != nil {
		return nil, err
	}
	return nil, ble.ErrNotImplemented
}

// DiscoverCharacteristics finds all the characteristics within a service. [Vol 3, Part G, 4.6.1]
// If filter is specified, only filtered characteristics are returned.
func (cln *Client) DiscoverCharacteristics(cs []ble.UUID, s *ble.Service) ([]*ble.Characteristic, error) {
	rsp, err := cln.conn.sendReq(cmdDiscoverCharacteristics, xpc.Dict{
		"kCBMsgArgDeviceUUID":         cln.id,
		"kCBMsgArgServiceStartHandle": s.Handle,
		"kCBMsgArgServiceEndHandle":   s.EndHandle,
		"kCBMsgArgUUIDs":              uuidSlice(cs),
	})
	if err != nil {
		return nil, err
	}
	if err := rsp.err(); err != nil {
		return nil, err
	}
	for _, xcs := range rsp.characteristics() {
		xc := msg(xcs.(xpc.Dict))
		s.Characteristics = append(s.Characteristics, &ble.Characteristic{
			UUID:        ble.MustParse(xc.uuid()),
			Property:    ble.Property(xc.characteristicProperties()),
			Handle:      uint16(xc.characteristicHandle()),
			ValueHandle: uint16(xc.characteristicValueHandle()),
		})
	}
	return s.Characteristics, nil
}

// DiscoverDescriptors finds all the descriptors within a characteristic. [Vol 3, Part G, 4.7.1]
// If filter is specified, only filtered descriptors are returned.
func (cln *Client) DiscoverDescriptors(ds []ble.UUID, c *ble.Characteristic) ([]*ble.Descriptor, error) {
	rsp, err := cln.conn.sendReq(cmdDiscoverDescriptors, xpc.Dict{
		"kCBMsgArgDeviceUUID":                cln.id,
		"kCBMsgArgCharacteristicHandle":      c.Handle,
		"kCBMsgArgCharacteristicValueHandle": c.ValueHandle,
		"kCBMsgArgUUIDs":                     uuidSlice(ds),
	})
	if err != nil {
		return nil, err
	}
	if err := rsp.err(); err != nil {
		return nil, err
	}
	for _, xds := range rsp.descriptors() {
		xd := msg(xds.(xpc.Dict))
		c.Descriptors = append(c.Descriptors, &ble.Descriptor{
			UUID:   ble.MustParse(xd.uuid()),
			Handle: uint16(xd.descriptorHandle()),
		})
	}
	return c.Descriptors, nil
}

// ReadCharacteristic reads a characteristic value from a server. [Vol 3, Part G, 4.8.1]
func (cln *Client) ReadCharacteristic(c *ble.Characteristic) ([]byte, error) {
	rsp, err := cln.conn.sendReq(cmdReadCharacteristic, xpc.Dict{
		"kCBMsgArgDeviceUUID":                cln.id,
		"kCBMsgArgCharacteristicHandle":      c.Handle,
		"kCBMsgArgCharacteristicValueHandle": c.ValueHandle,
	})
	if err != nil {
		return nil, err
	}
	if rsp.err() != nil {
		return nil, rsp.err()
	}
	c.Value = rsp.data()
	return rsp.data(), nil
}

// ReadLongCharacteristic reads a characteristic value which is longer than the MTU. [Vol 3, Part G, 4.8.3]
func (cln *Client) ReadLongCharacteristic(c *ble.Characteristic) ([]byte, error) {
	return nil, ble.ErrNotImplemented
}

// WriteCharacteristic writes a characteristic value to a server. [Vol 3, Part G, 4.9.3]
func (cln *Client) WriteCharacteristic(c *ble.Characteristic, b []byte, noRsp bool) error {
	args := xpc.Dict{
		"kCBMsgArgDeviceUUID":                cln.id,
		"kCBMsgArgCharacteristicHandle":      c.Handle,
		"kCBMsgArgCharacteristicValueHandle": c.ValueHandle,
		"kCBMsgArgData":                      b,
		"kCBMsgArgType":                      map[bool]int{false: 0, true: 1}[noRsp],
	}
	if noRsp {
		return cln.conn.sendCmd(cmdWriteCharacteristic, args)
	}
	m, err := cln.conn.sendReq(cmdWriteCharacteristic, args)
	if err != nil {
		return err
	}
	return m.err()
}

// ReadDescriptor reads a characteristic descriptor from a server. [Vol 3, Part G, 4.12.1]
func (cln *Client) ReadDescriptor(d *ble.Descriptor) ([]byte, error) {
	rsp, err := cln.conn.sendReq(cmdReadDescriptor, xpc.Dict{
		"kCBMsgArgDeviceUUID":       cln.id,
		"kCBMsgArgDescriptorHandle": d.Handle,
	})
	if err != nil {
		return nil, err
	}
	if err := rsp.err(); err != nil {
		return nil, err
	}
	d.Value = rsp.data()
	return rsp.data(), nil
}

// WriteDescriptor writes a characteristic descriptor to a server. [Vol 3, Part G, 4.12.3]
func (cln *Client) WriteDescriptor(d *ble.Descriptor, b []byte) error {
	rsp, err := cln.conn.sendReq(cmdWriteDescriptor, xpc.Dict{
		"kCBMsgArgDeviceUUID":       cln.id,
		"kCBMsgArgDescriptorHandle": d.Handle,
		"kCBMsgArgData":             b,
	})
	if err != nil {
		return err
	}
	return rsp.err()
}

// ReadRSSI retrieves the current RSSI value of remote peripheral. [Vol 2, Part E, 7.5.4]
func (cln *Client) ReadRSSI() int {
	rsp, err := cln.conn.sendReq(cmdReadRSSI, xpc.Dict{"kCBMsgArgDeviceUUID": cln.id})
	if err != nil {
		return 0
	}
	if rsp.err() != nil {
		return 0
	}
	return rsp.rssi()
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
	cln.conn.Lock()
	defer cln.conn.Unlock()
	cln.conn.subs[c.Handle] = &sub{fn: fn, char: c}
	rsp, err := cln.conn.sendReq(cmdSubscribeCharacteristic, xpc.Dict{
		"kCBMsgArgDeviceUUID":                cln.id,
		"kCBMsgArgCharacteristicHandle":      c.Handle,
		"kCBMsgArgCharacteristicValueHandle": c.ValueHandle,
		"kCBMsgArgState":                     1,
	})
	if err != nil {
		delete(cln.conn.subs, c.Handle)
		return err
	}
	if err := rsp.err(); err != nil {
		delete(cln.conn.subs, c.Handle)
		return err
	}
	return nil
}

// Unsubscribe unsubscribes to indication (if ind is set true), or notification
// of a specified characteristic value. [Vol 3, Part G, 4.10 & 4.11]
func (cln *Client) Unsubscribe(c *ble.Characteristic, ind bool) error {
	rsp, err := cln.conn.sendReq(cmdSubscribeCharacteristic, xpc.Dict{
		"kCBMsgArgDeviceUUID":                cln.id,
		"kCBMsgArgCharacteristicHandle":      c.Handle,
		"kCBMsgArgCharacteristicValueHandle": c.ValueHandle,
		"kCBMsgArgState":                     0,
	})
	if err != nil {
		return err
	}
	if err := rsp.err(); err != nil {
		return err
	}
	cln.conn.Lock()
	defer cln.conn.Unlock()
	delete(cln.conn.subs, c.Handle)
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
	rsp, err := cln.conn.sendReq(cmdDisconnect, xpc.Dict{"kCBMsgArgDeviceUUID": cln.id})
	if err != nil {
		return err
	}
	return rsp.err()
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

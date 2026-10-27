package darwin

import (
	"fmt"
	"sync"

	"github.com/JuulLabs-OSS/cbgo"
	"github.com/go-ble/ble"
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

	bindMtx   sync.Mutex
	bound     chan struct{} // closed while no rebindProfile runs
	rebinding bool
	bindAgain bool // a Service Changed arrived during rebindProfile
}

// NewClient ...
func NewClient(cm cbgo.CentralManager, c ble.Conn) (*Client, error) {
	as := c.RemoteAddr().String()
	id, err := ble.Parse(as)
	if err != nil {
		return nil, fmt.Errorf("connection has invalid address: addr=%s", as)
	}

	cln := &Client{
		conn:  c.(*conn),
		pc:    newProfCache(),
		cm:    cm,
		id:    id,
		bound: make(chan struct{}),
	}
	close(cln.bound)

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
	// rebindPass appends to the service list while callers read it.
	cln.bindMtx.Lock()
	defer cln.bindMtx.Unlock()
	if cln.profile == nil {
		return nil
	}
	return &ble.Profile{Services: append([]*ble.Service(nil), cln.profile.Services...)}
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
	cln.bindMtx.Lock()
	cln.profile = &ble.Profile{Services: ss}
	cln.bindMtx.Unlock()
	return cln.profile, nil
}

// DiscoverServices finds all the primary services on a server. [Vol 3, Part G, 4.4.1]
// If filter is specified, only filtered services are returned.
func (cln *Client) DiscoverServices(ss []ble.UUID) ([]*ble.Service, error) {
	cln.waitBound()

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
	cln.bindMtx.Lock()
	if cln.profile == nil {
		cln.profile = &ble.Profile{Services: svcs}
	}
	cln.bindMtx.Unlock()
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
	cln.waitBound()

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
	cln.waitBound()

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
	cln.waitBound()

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
	cln.waitBound()

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

// ReadDescriptor reads a characteristic descriptor from a server. [Vol 3, Part G, 4.12.1]
func (cln *Client) ReadDescriptor(d *ble.Descriptor) ([]byte, error) {
	cln.waitBound()

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
	cln.waitBound()

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
	return cln.conn.ReadRSSI()
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
	cln.waitBound()

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
	cln.waitBound()

	cbchr, err := cln.pc.findCbChr(c)
	if err != nil {
		return err
	}

	// Nothing to stop: no request, so no wait on a notification-state callback.
	if !cbchr.IsNotifying() {
		cln.conn.delSub(c)
		return nil
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

// DidModifyServices handles a Service Changed indication. A request on an
// invalidated object gets no callback and would wait until disconnect, so
// those services leave the cache at once, then rebindProfile restores them.
func (cln *Client) DidModifyServices(prph cbgo.Peripheral, invSvcs []cbgo.Service) {
	for _, cbs := range invSvcs {
		cln.pc.delSvc(cbs)
	}

	cln.bindMtx.Lock()
	defer cln.bindMtx.Unlock()
	if cln.rebinding {
		cln.bindAgain = true
		return
	}
	cln.rebinding = true
	cln.bound = make(chan struct{})
	// Discovery waits on callbacks this delegate thread delivers.
	go cln.rebindProfile()
}

// waitBound holds a request back while rebindProfile runs: the request would
// find no object for an invalidated attribute, or share an event slot with
// rebindProfile and take its callback.
func (cln *Client) waitBound() {
	cln.bindMtx.Lock()
	bound := cln.bound
	cln.bindMtx.Unlock()

	select {
	case <-bound:
	case <-cln.Disconnected():
	}
}

// rebindProfile repeats rebindPass until no Service Changed arrived during the
// last pass, then releases the requests waitBound holds.
func (cln *Client) rebindProfile() {
	for {
		cln.rebindPass()

		cln.bindMtx.Lock()
		if !cln.bindAgain {
			cln.rebinding = false
			close(cln.bound)
			cln.bindMtx.Unlock()
			return
		}
		cln.bindAgain = false
		cln.bindMtx.Unlock()
	}
}

// rebindPass binds each profile service that has no CoreBluetooth object -
// invalidated, or hidden at the previous discovery - to the objects a new
// discovery returns, matched by UUID and order, and adds to the profile each
// discovered service it lacks. The ble objects callers hold stay valid. Each service gets its subscriptions back, and a read of each
// readable one, as soon as its characteristics are bound: notifications sent
// before that were dropped, so the read delivers the value they carried.
// Descriptors are bound last; no subscription waits on them.
func (cln *Client) rebindPass() {
	if cln.profile == nil {
		return
	}
	if cln.await(&cln.conn.evl.svcsDiscovered, func() { cln.conn.prph.DiscoverServices(nil) }) != nil {
		return
	}

	cln.conn.RLock()
	subscribed := map[*ble.Characteristic]bool{}
	for _, sb := range cln.conn.subs {
		subscribed[sb.char] = true
	}
	cln.conn.RUnlock()

	type pending struct {
		c   *ble.Characteristic
		cbc cbgo.Characteristic
	}
	var withDescriptors []pending

	cbsvcs := cln.conn.prph.Services()
	svcUUIDs := make([]ble.UUID, len(cln.profile.Services))
	for i, s := range cln.profile.Services {
		svcUUIDs[i] = s.UUID
	}
	for i, s := range cln.profile.Services {
		if _, err := cln.pc.findCbSvc(s); err == nil {
			continue
		}
		k := nthWithUUID(len(cbsvcs), func(j int) ble.UUID { return ble.UUID(cbsvcs[j].UUID()) }, s.UUID, occurrence(svcUUIDs, i))
		if k < 0 {
			continue // still hidden
		}
		cbs := cbsvcs[k]
		cln.pc.addSvc(s, cbs)
		if cln.await(&cln.conn.evl.chrsDiscovered, func() { cln.conn.prph.DiscoverCharacteristics(nil, cbs) }) != nil {
			return
		}

		cbcs := cbs.Characteristics()
		chrUUIDs := make([]ble.UUID, len(s.Characteristics))
		for j, c := range s.Characteristics {
			chrUUIDs[j] = c.UUID
		}
		for j, c := range s.Characteristics {
			k := nthWithUUID(len(cbcs), func(m int) ble.UUID { return ble.UUID(cbcs[m].UUID()) }, c.UUID, occurrence(chrUUIDs, j))
			if k < 0 {
				continue
			}
			cbc := cbcs[k]
			cln.pc.addChr(c, cbc)
			if subscribed[c] {
				// The handler stays registered whatever the outcome, so a
				// notification the peripheral still sends keeps its route.
				cln.await(&cln.conn.evl.notifyChanged, func() { cln.conn.prph.SetNotify(true, cbc) })
				if c.Property&ble.CharRead != 0 {
					cln.conn.prph.ReadCharacteristic(cbc)
				}
			}
			if len(c.Descriptors) > 0 {
				withDescriptors = append(withDescriptors, pending{c, cbc})
			}
		}
	}

	// A service the peripheral hid at the first discovery joins the profile once it appears,
	// built whole before it is published so a reader of Profile never sees it half-filled.
	for _, cbs := range cbsvcs {
		if _, err := cln.pc.findSvc(cbs); err == nil {
			continue
		}
		s := &ble.Service{UUID: ble.UUID(cbs.UUID())}
		cln.pc.addSvc(s, cbs)
		if cln.await(&cln.conn.evl.chrsDiscovered, func() { cln.conn.prph.DiscoverCharacteristics(nil, cbs) }) != nil {
			return
		}
		for _, cbc := range cbs.Characteristics() {
			c := &ble.Characteristic{UUID: ble.UUID(cbc.UUID()), Property: ble.Property(cbc.Properties())}
			cln.pc.addChr(c, cbc)
			if cln.await(&cln.conn.evl.dscsDiscovered, func() { cln.conn.prph.DiscoverDescriptors(cbc) }) != nil {
				return
			}
			for _, cbd := range cbc.Descriptors() {
				d := &ble.Descriptor{UUID: ble.UUID(cbd.UUID())}
				cln.pc.addDsc(d, cbd)
				c.Descriptors = append(c.Descriptors, d)
			}
			s.Characteristics = append(s.Characteristics, c)
		}
		cln.bindMtx.Lock()
		cln.profile.Services = append(cln.profile.Services, s)
		cln.bindMtx.Unlock()
	}

	for _, p := range withDescriptors {
		cbc := p.cbc
		if cln.await(&cln.conn.evl.dscsDiscovered, func() { cln.conn.prph.DiscoverDescriptors(cbc) }) != nil {
			return
		}
		cbds := cbc.Descriptors()
		dscUUIDs := make([]ble.UUID, len(p.c.Descriptors))
		for m, d := range p.c.Descriptors {
			dscUUIDs[m] = d.UUID
		}
		for m, d := range p.c.Descriptors {
			if k := nthWithUUID(len(cbds), func(n int) ble.UUID { return ble.UUID(cbds[n].UUID()) }, d.UUID, occurrence(dscUUIDs, m)); k >= 0 {
				cln.pc.addDsc(d, cbds[k])
			}
		}
	}
}

// await starts a CoreBluetooth request and waits for the delegate callback it
// signals on slot.
func (cln *Client) await(slot *eventSlot, start func()) error {
	ch := slot.Listen()
	defer slot.Close()

	start()

	select {
	case itf := <-ch:
		if itf != nil {
			return itf.(error)
		}
		return nil
	case <-cln.Disconnected():
		return fmt.Errorf("disconnected")
	}
}

// occurrence returns how many entries of uuids before index i equal uuids[i].
func occurrence(uuids []ble.UUID, i int) int {
	n := 0
	for _, u := range uuids[:i] {
		if u.Equal(uuids[i]) {
			n++
		}
	}
	return n
}

// nthWithUUID returns the index of the n-th of count entries whose UUID, read
// by uuidAt, equals u, or -1.
func nthWithUUID(count int, uuidAt func(int) ble.UUID, u ble.UUID, n int) int {
	for i := 0; i < count; i++ {
		if uuidAt(i).Equal(u) {
			if n == 0 {
				return i
			}
			n--
		}
	}
	return -1
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

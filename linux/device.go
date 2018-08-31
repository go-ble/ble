package linux

import (
	"context"
	"io"
	"log"

	"github.com/go-ble/ble"
	"github.com/go-ble/ble/linux/att"
	"github.com/go-ble/ble/linux/gatt"
	"github.com/go-ble/ble/linux/hci"
	"github.com/pkg/errors"
)

// NewDevice returns the default HCI device.
func NewDevice(opts ...ble.Option) (*Device, error) {
	return NewDeviceWithName("Gopher", opts...)
}

// NewDeviceWithName returns the default HCI device.
func NewDeviceWithName(name string, opts ...ble.Option) (*Device, error) {
	return NewDeviceWithNameAndHandler(name, nil, opts...)
}

func NewDeviceWithNameAndHandler(name string, handler ble.NotifyHandler, opts ...ble.Option) (*Device, error) {
	dev, err := hci.NewHCI(opts...)
	if err != nil {
		return nil, errors.Wrap(err, "can't create hci")
	}
	if err = dev.Init(); err != nil {
		return nil, errors.Wrap(err, "can't init hci")
	}

	srv, err := gatt.NewServerWithNameAndHandler(name, handler)
	if err != nil {
		return nil, errors.Wrap(err, "can't create server")
	}

	// mtu := ble.DefaultMTU
	mtu := ble.MaxMTU // TODO: get this from user using Option.
	if mtu > ble.MaxMTU {
		return nil, errors.Wrapf(err, "maximum ATT_MTU is %d", ble.MaxMTU)
	}

	go loop(dev, srv, mtu)

	return &Device{HCI: dev, Server: srv}, nil
}

func loop(dev *hci.HCI, s *gatt.Server, mtu int) {
	for {
		l2c, err := dev.Accept()
		if err != nil {
			// An EOF error indicates that the HCI socket was closed during
			// the read.  Don't report this as an error.
			if err != io.EOF {
				log.Printf("can't accept: %s", err)
			}
			return
		}

		// Initialize the per-connection cccd values.
		l2c.SetContext(context.WithValue(l2c.Context(), ble.ContextKeyCCC, make(map[uint16]uint16)))
		l2c.SetRxMTU(mtu)

		s.Lock()
		as, err := att.NewServer(s.DB(), l2c)
		s.Unlock()
		if err != nil {
			log.Printf("can't create ATT server: %s", err)
			continue

		}
		go as.Loop()
	}
}

// Device ...
type Device struct {
	HCI    *hci.HCI
	Server *gatt.Server
}

// AddService adds a service to database.
func (d *Device) AddService(svc *ble.Service) error {
	return d.Server.AddService(svc)
}

// RemoveAllServices removes all services that are currently in the database.
func (d *Device) RemoveAllServices() error {
	return d.Server.RemoveAllServices()
}

// SetServices set the specified service to the database.
// It removes all currently added services, if any.
func (d *Device) SetServices(svcs []*ble.Service) error {
	return d.Server.SetServices(svcs)
}

// Stop stops gatt server.
func (d *Device) Stop() error {
	return d.HCI.Close()
}

func (d *Device) Advertise(ctx context.Context, adv ble.Advertisement) error {
	if err := d.HCI.AdvertiseAdv(adv); err != nil {
		return err
	}
	<-ctx.Done()
	d.HCI.StopAdvertising()
	return ctx.Err()

}

// AdvertiseNameAndServices advertises device name, and specified service UUIDs.
// It tres to fit the UUIDs in the advertising packet as much as possible.
// If name doesn't fit in the advertising packet, it will be put in scan response.
func (d *Device) AdvertiseNameAndServices(ctx context.Context, name string, uuids ...ble.UUID) error {
	if err := d.HCI.AdvertiseNameAndServices(name, uuids...); err != nil {
		return err
	}
	<-ctx.Done()
	d.HCI.StopAdvertising()
	return ctx.Err()
}

// AdvertiseMfgData avertises the given manufacturer data.
func (d *Device) AdvertiseMfgData(ctx context.Context, id uint16, b []byte) error {
	if err := d.HCI.AdvertiseMfgData(id, b); err != nil {
		return err
	}
	<-ctx.Done()
	d.HCI.StopAdvertising()
	return ctx.Err()
}

// AdvertiseServiceData16 advertises data associated with a 16bit service uuid
func (d *Device) AdvertiseServiceData16(ctx context.Context, id uint16, b []byte) error {
	if err := d.HCI.AdvertiseServiceData16(id, b); err != nil {
		return err
	}
	<-ctx.Done()
	d.HCI.StopAdvertising()
	return ctx.Err()
}

// AdvertiseIBeaconData advertise iBeacon with given manufacturer data.
func (d *Device) AdvertiseIBeaconData(ctx context.Context, b []byte) error {
	if err := d.HCI.AdvertiseIBeaconData(b); err != nil {
		return err
	}
	<-ctx.Done()
	d.HCI.StopAdvertising()
	return ctx.Err()
}

// AdvertiseIBeacon advertises iBeacon with specified parameters.
func (d *Device) AdvertiseIBeacon(ctx context.Context, u ble.UUID, major, minor uint16, pwr int8) error {
	if err := d.HCI.AdvertiseIBeacon(u, major, minor, pwr); err != nil {
		return err
	}
	<-ctx.Done()
	d.HCI.StopAdvertising()
	return ctx.Err()
}

// Scan starts scanning. Duplicated advertisements will be filtered out if allowDup is set to false.
func (d *Device) Scan(ctx context.Context, allowDup bool, h ble.AdvHandler) error {
	if err := d.HCI.SetAdvHandler(h); err != nil {
		return err
	}
	if err := d.HCI.Scan(allowDup); err != nil {
		return err
	}
	<-ctx.Done()
	d.HCI.StopScanning()
	return ctx.Err()
}

// Dial ...
func (d *Device) Dial(ctx context.Context, a ble.Addr) (ble.Client, error) {
	// d.HCI.Dial is a blocking call, although most of time it should return immediately.
	// But in case passing wrong device address or the device went non-connectable, it blocks.
	cln, err := d.HCI.Dial(ctx, a)
	return cln, errors.Wrap(err, "can't dial")
}

// Address returns the listener's device address.
func (d *Device) Address() ble.Addr {
	return d.HCI.Addr()
}

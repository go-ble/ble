package ble

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/pkg/errors"
)

// ErrDefaultDevice ...
var ErrDefaultDevice = errors.New("default device is not set")

var defaultDevice Device

// SetDefaultDevice returns the default HCI device.
func SetDefaultDevice(d Device) {
	defaultDevice = d
}

// AddService adds a service to database.
func AddService(svc *Service) error {
	if defaultDevice == nil {
		return ErrDefaultDevice
	}
	return defaultDevice.AddService(svc)
}

// RemoveAllServices removes all services that are currently in the database.
func RemoveAllServices() error {
	if defaultDevice == nil {
		return ErrDefaultDevice
	}
	return defaultDevice.RemoveAllServices()
}

// SetServices set the specified service to the database.
// It removes all currently added services, if any.
func SetServices(svcs []*Service) error {
	if defaultDevice == nil {
		return ErrDefaultDevice
	}
	return defaultDevice.SetServices(svcs)
}

// Stop detatch the GATT server from a peripheral device.
func Stop() error {
	if defaultDevice == nil {
		return ErrDefaultDevice
	}
	return defaultDevice.Stop()
}

// AdvertiseNameAndServices advertises device name, and specified service UUIDs.
// It tres to fit the UUIDs in the advertising packet as much as possi
// If name doesn't fit in the advertising packet, it will be put in scan response.
func AdvertiseNameAndServices(ctx context.Context, name string, uuids ...UUID) error {
	if defaultDevice == nil {
		return ErrDefaultDevice
	}
	defer untrap(trap(ctx))
	return defaultDevice.AdvertiseNameAndServices(ctx, name, uuids...)
}

// AdvertiseIBeaconData advertise iBeacon with given manufacturer data.
func AdvertiseIBeaconData(ctx context.Context, b []byte) error {
	if defaultDevice == nil {
		return ErrDefaultDevice
	}
	defer untrap(trap(ctx))
	return defaultDevice.AdvertiseIBeaconData(ctx, b)
}

// AdvertiseIBeacon advertises iBeacon with specified parameters.
func AdvertiseIBeacon(ctx context.Context, u UUID, major, minor uint16, pwr int8) error {
	if defaultDevice == nil {
		return ErrDefaultDevice
	}
	defer untrap(trap(ctx))
	return defaultDevice.AdvertiseIBeacon(ctx, u, major, minor, pwr)
}

// Scan starts scanning. Duplicated advertisements will be filtered out if allowDup is set to false.
func Scan(ctx context.Context, allowDup bool, h AdvHandler, f AdvFilter) error {
	if defaultDevice == nil {
		return ErrDefaultDevice
	}
	defer untrap(trap(ctx))

	if f == nil {
		return defaultDevice.Scan(ctx, allowDup, h)
	}

	h2 := func(a Advertisement) {
		if f(a) {
			h(a)
		}
	}
	return defaultDevice.Scan(ctx, allowDup, h2)
}

// Find ...
func Find(ctx context.Context, allowDup bool, f AdvFilter) ([]Advertisement, error) {
	if defaultDevice == nil {
		return nil, ErrDefaultDevice
	}
	var advs []Advertisement
	h := func(a Advertisement) {
		advs = append(advs, a)
	}
	defer untrap(trap(ctx))
	return advs, Scan(ctx, allowDup, h, f)
}

// Dial ...
func Dial(ctx context.Context, a Addr) (Client, error) {
	if defaultDevice == nil {
		return nil, ErrDefaultDevice
	}
	defer untrap(trap(ctx))
	return defaultDevice.Dial(ctx, a)
}

// Connect searches for and connects to a Peripheral which matches specified condition.
func Connect(ctx context.Context, f AdvFilter) (Client, error) {
	ctx2, cancel := context.WithCancel(ctx)
	go func() {
		select {
		case <-ctx.Done():
			cancel()
		case <-ctx2.Done():
		}
	}()

	ch := make(chan Advertisement)
	fn := func(a Advertisement) {
		cancel()
		ch <- a
	}
	if err := Scan(ctx2, false, fn, f); err != nil {
		if err != context.Canceled {
			return nil, errors.Wrap(err, "can't scan")
		}
		// If the parent context was cancelled, Dial below would hang.
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}

	cln, err := Dial(ctx, (<-ch).Addr())
	return cln, errors.Wrap(err, "can't dial")
}

// A NotificationHandler handles notification or indication from a server.
type NotificationHandler func(req []byte)

// WithSigHandler ...
func WithSigHandler(ctx context.Context, cancel func()) context.Context {
	return context.WithValue(ctx, ContextKeySig, cancel)
}

// Cleanup for the interrupted case.
func trap(ctx context.Context) chan<- os.Signal {
	v := ctx.Value(ContextKeySig)
	if v == nil {
		return nil
	}
	cancel, ok := v.(func())
	if cancel == nil || !ok {
		return nil
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigs:
			cancel()
		case <-ctx.Done():
		}
	}()
	return sigs
}

func untrap(sigs chan<- os.Signal) {
	if sigs == nil {
		return
	}
	signal.Stop(sigs)
}

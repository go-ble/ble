package ble

import (
	"github.com/armaanhammer/ble/linux/hci/evt"
	"time"

	"github.com/armaanhammer/ble/linux/hci/cmd"
)

// DeviceOption is an interface which the device should implement to allow using configuration options
type DeviceOption interface {
	SetDeviceID(int) error
	SetDialerTimeout(time.Duration) error
	SetListenerTimeout(time.Duration) error
	SetConnParams(cmd.LECreateConnection) error
	SetScanParams(cmd.LESetScanParameters) error
	SetAdvParams(cmd.LESetAdvertisingParameters) error
	SetConnectedHandler(f func(evt.LEConnectionComplete)) error
	SetDisconnectedHandler(f func(evt.DisconnectionComplete)) error
	SetPeripheralRole() error
	SetCentralRole() error
}

// An Option is a configuration function, which configures the device.
type Option func(DeviceOption) error

// OptDeviceID sets HCI device ID.
func OptDeviceID(id int) Option {
	return func(opt DeviceOption) error {
		opt.SetDeviceID(id)
		return nil
	}
}

// OptDialerTimeout sets dialing timeout for Dialer.
func OptDialerTimeout(d time.Duration) Option {
	return func(opt DeviceOption) error {
		opt.SetDialerTimeout(d)
		return nil
	}
}

// OptListenerTimeout sets dialing timeout for Listener.
func OptListenerTimeout(d time.Duration) Option {
	return func(opt DeviceOption) error {
		opt.SetListenerTimeout(d)
		return nil
	}
}

// OptConnParams overrides default connection parameters.
func OptConnParams(param cmd.LECreateConnection) Option {
	return func(opt DeviceOption) error {
		opt.SetConnParams(param)
		return nil
	}
}

// OptScanParams overrides default scanning parameters.
func OptScanParams(param cmd.LESetScanParameters) Option {
	return func(opt DeviceOption) error {
		opt.SetScanParams(param)
		return nil
	}
}

// OptAdvParams overrides default advertising parameters.
func OptAdvParams(param cmd.LESetAdvertisingParameters) Option {
	return func(opt DeviceOption) error {
		opt.SetAdvParams(param)
		return nil
	}
}

func OptConnectHandler(f func(evt.LEConnectionComplete)) Option {
	return func(opt DeviceOption) error {
		opt.SetConnectedHandler(f)
		return nil
	}
}

func OptDisconnectHandler(f func(evt.DisconnectionComplete)) Option {
	return func(opt DeviceOption) error {
		opt.SetDisconnectedHandler(f)
		return nil
	}
}

// OptPeripheralRole configures the device to perform Peripheral tasks.
func OptPeripheralRole() Option {
	return func(opt DeviceOption) error {
		opt.SetPeripheralRole()
		return nil
	}
}

// OptCentralRole configures the device to perform Central tasks.
func OptCentralRole() Option {
	return func(opt DeviceOption) error {
		opt.SetCentralRole()
		return nil
	}
}

package darwin

import (
	"errors"
	"time"

	"github.com/go-ble/ble/linux/hci/cmd"
)

// SetPeripheralRole configures the device to perform Peripheral tasks.
func (d *Device) SetPeripheralRole() error {
	d.role = 1
	return nil
}

// SetCentralRole configures the device to perform Central tasks.
func (d *Device) SetCentralRole() error {
	d.role = 0
	return nil
}

// SetDeviceID sets HCI device ID.
func (d *Device) SetDeviceID(id int) error {
	return errors.New("Not supported")
}

// SetDialerTimeout sets dialing timeout for Dialer.
func (d *Device) SetDialerTimeout(dur time.Duration) error {
	return errors.New("Not supported")
}

// SetListenerTimeout sets dialing timeout for Listener.
func (d *Device) SetListenerTimeout(dur time.Duration) error {
	return errors.New("Not supported")
}

// SetConnParams overrides default connection parameters.
func (d *Device) SetConnParams(param cmd.LECreateConnection) error {
	return errors.New("Not supported")
}

package hci

import (
	"errors"
	"time"

	"github.com/go-ble/ble/linux/hci/cmd"
)

// SetDeviceID sets HCI device ID.
func (h *HCI) SetDeviceID(id int) error {
	h.id = id
	return nil
}

// SetDialerTimeout sets dialing timeout for Dialer.
func (h *HCI) SetDialerTimeout(d time.Duration) error {
	h.dialerTmo = d
	return nil
}

// SetListenerTimeout sets dialing timeout for Listener.
func (h *HCI) SetListenerTimeout(d time.Duration) error {
	h.listenerTmo = d
	return nil
}

// SetConnParams overrides default connection parameters.
func (h *HCI) SetConnParams(param cmd.LECreateConnection) error {
	h.params.connParams = param
	return nil
}

// SetPeripheralRole is not supported
func (h *HCI) SetPeripheralRole() error {
	return errors.New("Not supported")
}

// SetCentralRole is not supported
func (h *HCI) SetCentralRole() error {
	return errors.New("Not supported")
}

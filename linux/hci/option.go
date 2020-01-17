package hci

import (
	"errors"
	"github.com/armaanhammer/ble/linux/hci/evt"
	"time"

	"github.com/armaanhammer/ble/linux/hci/cmd"
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

// SetScanParams overrides default scanning parameters.
func (h *HCI) SetScanParams(param cmd.LESetScanParameters) error {
	h.params.scanParams = param
	return nil
}

// SetConnectedHandler sets handler to be called when new connection is established.
func (h *HCI) SetConnectedHandler(f func(complete evt.LEConnectionComplete)) error {
	h.connectedHandler = f
	return nil
}

// SetDisconnectedHandler sets handler to be called on disconnect.
func (h *HCI) SetDisconnectedHandler(f func(evt.DisconnectionComplete)) error {
	h.disconnectedHandler = f
	return nil
}

// SetAdvParams overrides default advertising parameters.
func (h *HCI) SetAdvParams(param cmd.LESetAdvertisingParameters) error {
	h.params.advParams = param
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

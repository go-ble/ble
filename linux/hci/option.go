package hci

import (
	"errors"
	"fmt"
	"time"

	"github.com/rigado/ble/cache"

	"github.com/rigado/ble/linux/hci/cmd"
)

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

func (h *HCI) EnableSecurity(bm interface{}) error {
	bondManager, ok := bm.(BondManager)
	if !ok {
		return fmt.Errorf("unknown bond manager type")
	}
	h.smpEnabled = true
	if h.smp != nil {
		h.smp.SetBondManager(bondManager)
	}
	return nil
}

// SetScanParams overrides default scanning parameters.
func (h *HCI) SetScanParams(param cmd.LESetScanParameters) error {
	h.params.scanParams = param
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

// SetAdvHandlerSync overrides default advertising handler behavior (async)
func (h *HCI) SetAdvHandlerSync(sync bool) error {
	h.advHandlerSync = sync
	return nil
}

// SetErrorHandler ...
func (h *HCI) SetErrorHandler(handler func(error)) error {
	h.errorHandler = handler
	return nil
}

// SetTransportHCISocket sets HCI device for hci socket
func (h *HCI) SetTransportHCISocket(id int) error {
	h.transport = transport{
		hci: &transportHci{id},
	}
	return nil
}

// SetTransportH4Socket sets h4 socket server
func (h *HCI) SetTransportH4Socket(addr string, timeout time.Duration) error {
	h.transport = transport{
		h4socket: &transportH4Socket{addr, timeout},
	}
	return nil
}

// SetTransportH4Uart sets h4 uart path
func (h *HCI) SetTransportH4Uart(path string) error {
	h.transport = transport{
		h4uart: &transportH4Uart{path},
	}
	return nil
}

func (h *HCI) SetGattCacheFile(filename string) {
	h.cache = cache.New(filename)
}

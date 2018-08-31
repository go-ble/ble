package main

import (
	"github.com/go-ble/ble"
	"github.com/go-ble/ble/linux"
	"github.com/go-ble/ble/linux/hci/cmd"
	"github.com/pkg/errors"
)

func updateLinuxParam(d *linux.Device) error {
	if err := d.HCI.Send(&cmd.LESetAdvertisingParameters{
		AdvertisingIntervalMin:  0x0020,    // 0x0020 - 0x4000; N * 0.625 msec
		AdvertisingIntervalMax:  0x0020,    // 0x0020 - 0x4000; N * 0.625 msec
		AdvertisingType:         0x00,      // 00: ADV_IND, 0x01: DIRECT(HIGH), 0x02: SCAN, 0x03: NONCONN, 0x04: DIRECT(LOW)
		OwnAddressType:          0x00,      // 0x00: public, 0x01: random
		DirectAddressType:       0x00,      // 0x00: public, 0x01: random
		DirectAddress:           [6]byte{}, // Public or Random Address of the Device to be connected
		AdvertisingChannelMap:   0x7,       // 0x07 0x01: ch37, 0x2: ch38, 0x4: ch39
		AdvertisingFilterPolicy: 0x00,
	}, nil); err != nil {
		return errors.Wrap(err, "can't set advertising param")
	}

	if err := d.HCI.Send(&cmd.LESetScanParameters{
		LEScanType:           0x01,   // 0x00: passive, 0x01: active
		LEScanInterval:       0x0004, // 0x0004 - 0x4000; N * 0.625msec
		LEScanWindow:         0x0004, // 0x0004 - 0x4000; N * 0.625msec
		OwnAddressType:       0x00,   // 0x00: public, 0x01: random
		ScanningFilterPolicy: 0x00,   // 0x00: accept all, 0x01: ignore non-white-listed.
	}, nil); err != nil {
		return errors.Wrap(err, "can't set scan param")
	}

	if err := d.HCI.Option(ble.OptConnParams(
		cmd.LECreateConnection{
			LEScanInterval:        0x0004,    // 0x0004 - 0x4000; N * 0.625 msec
			LEScanWindow:          0x0004,    // 0x0004 - 0x4000; N * 0.625 msec
			InitiatorFilterPolicy: 0x00,      // White list is not used
			PeerAddressType:       0x00,      // Public Device Address
			PeerAddress:           [6]byte{}, //
			OwnAddressType:        0x00,      // Public Device Address
			ConnIntervalMin:       0x0006,    // 0x0006 - 0x0C80; N * 1.25 msec
			ConnIntervalMax:       0x0006,    // 0x0006 - 0x0C80; N * 1.25 msec
			ConnLatency:           0x0000,    // 0x0000 - 0x01F3; N * 1.25 msec
			SupervisionTimeout:    0x0048,    // 0x000A - 0x0C80; N * 10 msec
			MinimumCELength:       0x0000,    // 0x0000 - 0xFFFF; N * 0.625 msec
			MaximumCELength:       0x0000,    // 0x0000 - 0xFFFF; N * 0.625 msec
		})); err != nil {
		return errors.Wrap(err, "can't set connection param")
	}
	return nil
}

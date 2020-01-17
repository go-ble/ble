package dev

import (
	"github.com/armaanhammer/ble"
	"github.com/armaanhammer/ble/linux"
)

// DefaultDevice ...
func DefaultDevice(opts ...ble.Option) (d ble.Device, err error) {
	return linux.NewDevice(opts...)
}

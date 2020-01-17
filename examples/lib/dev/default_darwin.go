package dev

import (
	"github.com/armaanhammer/ble"
	"github.com/armaanhammer/ble/darwin"
)

// DefaultDevice ...
func DefaultDevice(opts ...ble.Option) (d ble.Device, err error) {
	return darwin.NewDevice(opts...)
}

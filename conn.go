package ble

import (
	"context"
	"io"
)

// Conn implements a L2CAP connection.
type Conn interface {
	io.ReadWriteCloser

	// Context returns the context that is used by this Conn.
	Context() context.Context

	// SetContext sets the context that is used by this Conn.
	SetContext(ctx context.Context)

	// LocalAddr returns local device's address.
	LocalAddr() Addr

	// RemoteAddr returns remote device's address.
	RemoteAddr() Addr

	// RxMTU returns the ATT_MTU which the local device is capable of accepting.
	RxMTU() int

	// SetRxMTU sets the ATT_MTU which the local device is capable of accepting.
	SetRxMTU(mtu int)

	// TxMTU returns the ATT_MTU which the remote device is capable of accepting.
	TxMTU() int

	// SetTxMTU sets the ATT_MTU which the remote device is capable of accepting.
	SetTxMTU(mtu int)

	// ReadRSSI retrieves the current RSSI value of remote peripheral. [Vol 2, Part E, 7.5.4]
	ReadRSSI() int

	// ReadPHY retrieves the current PHY settings of the connection.
	ReadPHY() (txPhy int, rxPhy int, err error)

	// SetPHY sets the PHY for the connection.
	SetPHY(txPhy int, rxPhy int) error

	// Disconnected returns a receiving channel, which is closed when the connection disconnects.
	Disconnected() <-chan struct{}
}

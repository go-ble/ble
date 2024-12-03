package ble

// ExtendedAdvHandler handles extended advertisements.
type ExtendedAdvHandler func(a ExtendedAdvertisement)

// ExtendedAdvFilter returns true if the extended advertisement matches specified condition.
type ExtendedAdvFilter func(a ExtendedAdvertisement) bool

// ExtendedAdvertisement ...
type ExtendedAdvertisement interface {
	LocalName() string
	ManufacturerData() []byte
	ServiceData() []ServiceData
	Services() []UUID
	OverflowService() []UUID
	TxPowerLevel() int
	Connectable() bool
	SolicitedService() []UUID

	RSSI() int
	Addr() Addr
}

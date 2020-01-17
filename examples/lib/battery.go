package lib

import "github.com/armaanhammer/ble"

// NewBatteryService ...
func NewBatteryService() *ble.Service {
	lv := byte(100)
	s := ble.NewService(ble.UUID16(0x180F))
	c := s.NewCharacteristic(ble.UUID16(0x2A19))
	c.HandleRead(
		ble.ReadHandlerFunc(func(req ble.Request, rsp ble.ResponseWriter) {
			rsp.Write([]byte{lv})
			lv--
		}))

	// Characteristic User Description
	c.NewDescriptor(ble.UUID16(0x2901)).SetValue([]byte("Battery level between 0 and 100 percent"))

	// Characteristic Presentation Format
	c.NewDescriptor(ble.UUID16(0x2904)).SetValue([]byte{4, 1, 39, 173, 1, 0, 0})

	return s
}

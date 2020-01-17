package lib

import "github.com/armaanhammer/ble"

// Private 128-bit UUIDs, which avoids the base of pre-defined 16/32-bits UUIDS
// xxxxxxxx-0000-1000-8000-00805F9B34FB [Vol 3, Part B, 2.5.1].
var (
	TestSvcUUID   = ble.MustParse("00010000-0001-1000-8000-00805F9B34FB")
	CountCharUUID = ble.MustParse("00010000-0002-1000-8000-00805F9B34FB")
	EchoCharUUID  = ble.MustParse("00020000-0002-1000-8000-00805F9B34FB")
)

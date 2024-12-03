package hci

import (
	"sync"

	"github.com/go-ble/ble/linux/hci/cmd"
)

type params struct {
	sync.RWMutex

	advEnable          cmd.LESetAdvertiseEnable
	scanEnable         cmd.LESetScanEnable
	extendedScanEnable cmd.LESetExtendedScanEnable
	connCancel         cmd.LECreateConnectionCancel

	leSetDefaultPHY cmd.LESetDefaultPHY

	advData            cmd.LESetAdvertisingData
	scanResp           cmd.LESetScanResponseData
	advParams          cmd.LESetAdvertisingParameters
	scanParams         cmd.LESetScanParameters
	extendedScanParams cmd.LESetExtendedScanParameters
	connParams         cmd.LECreateConnection
}

func (p *params) init() {
	p.scanParams = cmd.LESetScanParameters{
		LEScanType:           0x01,   // 0x00: passive, 0x01: active
		LEScanInterval:       0x0004, // 0x0004 - 0x4000; N * 0.625msec
		LEScanWindow:         0x0004, // 0x0004 - 0x4000; N * 0.625msec
		OwnAddressType:       0x00,   // 0x00: public, 0x01: random
		ScanningFilterPolicy: 0x00,   // 0x00: accept all, 0x01: ignore non-white-listed.
	}

	p.advParams = cmd.LESetAdvertisingParameters{
		AdvertisingIntervalMin:  0x0020,    // 0x0020 - 0x4000; N * 0.625 msec
		AdvertisingIntervalMax:  0x0020,    // 0x0020 - 0x4000; N * 0.625 msec
		AdvertisingType:         0x00,      // 00: ADV_IND, 0x01: DIRECT(HIGH), 0x02: SCAN, 0x03: NONCONN, 0x04: DIRECT(LOW)
		OwnAddressType:          0x00,      // 0x00: public, 0x01: random
		DirectAddressType:       0x00,      // 0x00: public, 0x01: random
		DirectAddress:           [6]byte{}, // Public or Random Address of the Device to be connected
		AdvertisingChannelMap:   0x7,       // 0x07 0x01: ch37, 0x2: ch38, 0x4: ch39
		AdvertisingFilterPolicy: 0x00,
	}

	p.connParams = cmd.LECreateConnection{
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
	}
}

func (p *params) initForAdvertisingExtensions() {
	p.extendedScanParams = cmd.LESetExtendedScanParameters{
		OwnAddressType:       0x00,   // 0x00: public, 0x01: random
		ScanningFilterPolicy: 0x00,   // 0x00: accept all, 0x01: ignore non-white-listed
		ScanningPHYs:         0x05,   // 0x01: Scan on the LE 1M PHY, 0x04: Scan on the LE Coded PHY
		ScanType1M:           0x01,   // 0x00: passive scan, 0x01: active scan
		ScanInterval1M:       0x0064, // 0x0004 - 0x4000; N * 0.625 msec (2.5 ms)
		ScanWindow1M:         0x0032, // 0x0004 - 0x4000; N * 0.625 msec (2.5 ms)
		ScanTypeCoded:        0x01,   // 0x00: passive scan, 0x01: active scan (for Coded PHY)
		ScanIntervalCoded:    0x0BB8, // N * 0.625 msec, scan interval for Coded PHY
		ScanWindowCoded:      0x0032, // N * 0.625 msec, scan window for Coded PHY
	}

	p.extendedScanParams = cmd.LESetExtendedScanParameters{
		OwnAddressType:       0x00,   // 0x00: public, 0x01: random
		ScanningFilterPolicy: 0x00,   // 0x00: accept all, 0x01: ignore non-white-listed
		ScanningPHYs:         0x05,   // 0x01: Scan on the LE 1M PHY, 0x04: Scan on the LE Coded PHY
		ScanType1M:           0x01,   // 0x00: passive scan, 0x01: active scan
		ScanInterval1M:       0x0064, // 0x0004 - 0x4000; N * 0.625 msec (2.5 ms)
		ScanWindow1M:         0x0032, // 0x0004 - 0x4000; N * 0.625 msec (2.5 ms)
		ScanTypeCoded:        0x01,   // 0x00: passive scan, 0x01: active scan (for Coded PHY)
		ScanIntervalCoded:    0x0BB8, // N * 0.625 msec, scan interval for Coded PHY
		ScanWindowCoded:      0x0032, // N * 0.625 msec, scan window for Coded PHY
	}

	p.leSetDefaultPHY = cmd.LESetDefaultPHY{
		AllPHYs: 0x00, // 0x00: No preference, use all PHYs
		TXPHYs:  0x07, // 0x01: LE 1M, 0x02: LE 2M, 0x04: LE Coded (0x07 to use all PHYs)
		RXPHYs:  0x07, // 0x01: LE 1M, 0x02: LE 2M, 0x04: LE Coded (uses all PHYs at 0x07)
	}
}

package hci

import (
	"fmt"
	"sync"

	"github.com/rigado/ble/linux/hci/cmd"
)

const (
	AddressTypePublic           = 0
	AddressTypeRandom           = 1
	FilterPolicyAcceptAll       = 0
	FilterPolicyAcceptWhitelist = 1
	LEScanTypePassive           = 0
	LEScanTypeActive            = 1
	LEScanIntervalMin           = 0x0006
	LEScanIntervalMax           = 0x4000
	LEScanWindowMin             = 0x0006
	LEScanWindowMax             = 0x4000
	ConnIntervalMin             = 0x0006
	ConnIntervalMax             = 0x0c80
	ConnLatencyMin              = 0x0000
	ConnLatencyMax              = 0x01f3
	SupervisionTimeoutMin       = 0x000a
	SupervisionTimeoutMax       = 0x0c80
	CELengthMin                 = 0x0000
	CELengthMax                 = 0xffff
)

type params struct {
	sync.RWMutex

	advEnable  cmd.LESetAdvertiseEnable
	scanEnable cmd.LESetScanEnable
	connCancel cmd.LECreateConnectionCancel

	advData    cmd.LESetAdvertisingData
	scanResp   cmd.LESetScanResponseData
	advParams  cmd.LESetAdvertisingParameters
	scanParams cmd.LESetScanParameters
	connParams cmd.LECreateConnection
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
		LEScanInterval:        0x0040,    // 0x0004 - 0x4000; N * 0.625 msec
		LEScanWindow:          0x0040,    // 0x0004 - 0x4000; N * 0.625 msec
		InitiatorFilterPolicy: 0x00,      // White list is not used
		PeerAddressType:       0x00,      // Public Device Address
		PeerAddress:           [6]byte{}, //
		OwnAddressType:        0x00,      // Public Device Address
		ConnIntervalMin:       0x006,     // 0x0006 - 0x0C80; N * 1.25 msec
		ConnIntervalMax:       0x006,     // 0x0006 - 0x0C80; N * 1.25 msec
		ConnLatency:           0x0000,    // 0x0000 - 0x01F3; N * 1.25 msec
		SupervisionTimeout:    0x0400,    // 0x000A - 0x0C80; N * 10 msec
		MinimumCELength:       0x0000,    // 0x0000 - 0xFFFF; N * 0.625 msec
		MaximumCELength:       0x0000,    // 0x0000 - 0xFFFF; N * 0.625 msec
	}
}

func (p *params) validate() error {
	if p == nil {
		return fmt.Errorf("params nil")
	}
	if err := ValidateConnParams(p.connParams); err != nil {
		return err
	}
	if err := ValidateScanParams(p.scanParams); err != nil {
		return err
	}
	// todo: validate the rest

	return nil
}

func ValidateScanParams(p cmd.LESetScanParameters) error {
	switch {
	case p.LEScanType != LEScanTypeActive && p.LEScanType != LEScanTypePassive:
		return fmt.Errorf("invalid LEScanType %v", p.LEScanType)

	case p.LEScanInterval < LEScanIntervalMin && p.LEScanInterval > LEScanIntervalMax:
		return fmt.Errorf("invalid LEScanInterval %v", p.LEScanInterval)

	case p.LEScanWindow < LEScanWindowMin && p.LEScanWindow > LEScanWindowMax:
		return fmt.Errorf("invalid LEScanWindow %v", p.LEScanWindow)

	case p.LEScanWindow > p.LEScanInterval:
		return fmt.Errorf("LEScanWindow %v > LEScanInterval %v", p.LEScanWindow, p.LEScanInterval)

	case p.OwnAddressType != AddressTypePublic && p.OwnAddressType != AddressTypeRandom:
		// this probably is filled later
		return fmt.Errorf("invalid OwnAddressType %v", p.OwnAddressType)

	case p.ScanningFilterPolicy != FilterPolicyAcceptAll && p.ScanningFilterPolicy != FilterPolicyAcceptWhitelist:
		return fmt.Errorf("invalid ScanningFilterPolicy %v", p.ScanningFilterPolicy)
	}

	return nil
}

func ValidateConnParams(p cmd.LECreateConnection) error {

	/* The Supervision_Timeout in milliseconds shall be larger than
	(1 + Conn_Latency) * Conn_Interval_Max * 2, where Conn_Interval_Max is
	given in milliseconds.
	*/
	minStoMs := (1 + float64(p.ConnLatency)*1.25) * (float64(p.ConnIntervalMax) * 1.25) * 2
	stoMs := float64(p.SupervisionTimeout) * 10

	//note: cannot calculate valid connSlaveLatency range since we do not have connInterval

	switch {
	case p.LEScanInterval < LEScanIntervalMin && p.LEScanInterval > LEScanIntervalMax:
		return fmt.Errorf("invalid LEScanInterval %v", p.LEScanInterval)

	case p.LEScanWindow < LEScanWindowMin && p.LEScanWindow > LEScanWindowMax:
		return fmt.Errorf("invalid LEScanWindow %v", p.LEScanWindow)

	case p.LEScanWindow > p.LEScanInterval:
		return fmt.Errorf("LEScanWindow %v > LEScanInterval %v", p.LEScanWindow, p.LEScanInterval)

	case p.InitiatorFilterPolicy != FilterPolicyAcceptAll && p.InitiatorFilterPolicy != FilterPolicyAcceptWhitelist:
		return fmt.Errorf("invalid InitiatorFilterPolicy %v", p.InitiatorFilterPolicy)

	case p.OwnAddressType != AddressTypePublic && p.OwnAddressType != AddressTypeRandom:
		// this probably is filled later
		return fmt.Errorf("invalid OwnAddressType %v", p.OwnAddressType)

	case p.PeerAddressType != AddressTypePublic && p.PeerAddressType != AddressTypeRandom:
		// this probably is filled later along with peer addr
		return fmt.Errorf("invalid PeerAddressType %v", p.OwnAddressType)

	case p.ConnIntervalMax < ConnIntervalMin && p.ConnIntervalMax > ConnIntervalMax:
		return fmt.Errorf("invalid ConnIntervalMax %v", p.ConnIntervalMax)

	case p.ConnIntervalMin < ConnIntervalMin && p.ConnIntervalMin > ConnIntervalMax:
		return fmt.Errorf("invalid ConnIntervalMin %v", p.ConnIntervalMin)

	case p.ConnIntervalMin > p.ConnIntervalMax:
		return fmt.Errorf("ConnIntervalMin %v > ConnIntervalMax %v", p.ConnIntervalMin, p.ConnIntervalMax)

	case p.ConnLatency < ConnLatencyMin && p.ConnLatency > ConnLatencyMax:
		return fmt.Errorf("invalid ConnLatency %v", p.ConnLatency)

	case p.SupervisionTimeout < SupervisionTimeoutMin && p.SupervisionTimeout > SupervisionTimeoutMax:
		return fmt.Errorf("invalid SupervisionTimeout %v", p.SupervisionTimeout)

	case stoMs < minStoMs:
		return fmt.Errorf("invalid SupervisionTimeout %v (too small)", p.SupervisionTimeout)

	case p.MinimumCELength < CELengthMin && p.MinimumCELength > CELengthMax:
		return fmt.Errorf("invalid MinimumCELength %v", p.MinimumCELength)

	case p.MaximumCELength < CELengthMin && p.MaximumCELength > CELengthMax:
		return fmt.Errorf("invalid MaximumCELength %v", p.MaximumCELength)

	case p.MinimumCELength > p.MaximumCELength:
		return fmt.Errorf("MinimumCELength %v > MaximumCELength %v", p.MinimumCELength, p.MaximumCELength)

	}

	return nil
}

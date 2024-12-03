package hci

import (
	"fmt"
	"github.com/go-ble/ble"
	"github.com/go-ble/ble/linux/adv"
	"github.com/go-ble/ble/linux/hci/evt"
	"net"
)

const (
	evtTypExtendedAdvInd              = 0b00100011 // ADV_IND (Connectable and Scannable Undirected Advertising)
	evtTypExtendedAdvDirectInd        = 0b00100101 // ADV_DIRECT_IND (Connectable Directed Advertising)
	evtTypExtendedAdvScanInd          = 0b00100100 // ADV_SCAN_IND (Scannable Undirected Advertising)
	evtTypExtendedAdvNonConnInd       = 0b00100000 // ADV_NONCONN_IND (Non-Connectable Undirected Advertising)
	evtTypScanRspToExtendedAdvInd     = 0b00101111 // SCAN_RSP to ADV_IND (Scan Response to ADV_IND)
	evtTypScanRspToExtendedAdvScanInd = 0b00101110 // SCAN_RSP to ADV_SCAN_IND (Scan Response to ADV_SCAN_IND)
)

type LEExtendedAdvertisingReport struct {
	SubeventCode uint8
	NumReports   uint8
	Reports      []ExtendedAdvertisingData
}

// AdvertisingReport represents an individual report within the LE Extended Advertising Report
type ExtendedAdvertisingData struct {
	eventType                   uint16
	addressType                 uint8
	address                     []byte
	primaryPHY                  uint8
	secondaryPHY                uint8
	advertisingSID              uint8
	txPower                     int8
	rssi                        int8
	periodicAdvertisingInterval uint16
	directAddressType           uint8
	directAddress               []byte
	dataLength                  uint8
	data                        []byte
	scanResp                    *ExtendedAdvertisingData

	// cached packets.
	p *adv.Packet
}

// sample

// 0x0d Subevent_Code
// 0x01 Num_Reports
// 0x10  0x00 Event_Type
// 0x01 Address_Type
// 0xde  0xb4  0xc0  0xf9 0x90 0xfa Address
// 0x01 Primary_PHY
// 0x00 Secondary_PHY
// 0xff Advertising_SID
// 0x7f TX_Power
// 0xdb RSSI
// 0x00  0x00 Periodic_Advertising_Interval
// 0x00  Direct_Address_Type
// 0x00
// 0x00  0x00  0x00  0x00  0x00  0x08 Direct_Address
// 0x07
// 0xff  0x4c  0x00  0x12  0x02  0x00  0x03

const reportDataStaticLength = 24

func newLEExtendedAdvertisingReport(data evt.LEExtendedAdvertisingReport) (
	*LEExtendedAdvertisingReport,
	error,
) {
	if len(data) < 2 {
		return nil, fmt.Errorf("invalid data length")
	}

	report := &LEExtendedAdvertisingReport{
		SubeventCode: data[0],
		NumReports:   data[1],
	}

	offset := 2
	for i := 0; i < int(report.NumReports); i++ {
		if offset+reportDataStaticLength > len(data) {
			return nil, fmt.Errorf("invalid data length %d for report %d", len(data), i)
		}

		// Parse individual report
		r := ExtendedAdvertisingData{
			eventType:                   uint16(data[offset]) | uint16(data[offset+1])<<8,
			addressType:                 data[offset+2],
			address:                     data[offset+3 : offset+9],
			primaryPHY:                  data[offset+9],
			secondaryPHY:                data[offset+10],
			advertisingSID:              data[offset+11],
			txPower:                     int8(data[offset+12]),
			rssi:                        int8(data[offset+13]),
			periodicAdvertisingInterval: uint16(data[offset+14]) | uint16(data[offset+15])<<8,
			directAddressType:           data[offset+16],
			directAddress:               data[offset+19 : offset+22],
			dataLength:                  data[offset+23],
			scanResp:                    &ExtendedAdvertisingData{},
		}

		// Parse data field
		dataEnd := offset + reportDataStaticLength + int(r.dataLength)
		if dataEnd > len(data) {
			return nil, fmt.Errorf("invalid data length %d, dataEnd %d, for report %d data", len(data), dataEnd, i)
		}
		r.data = data[offset+reportDataStaticLength : dataEnd]

		// Add to report list
		report.Reports = append(report.Reports, r)
		offset = dataEnd
	}

	return report, nil
}

// packets returns the combined extended advertising packet and scan response (if present).
func (a *ExtendedAdvertisingData) packets() *adv.Packet {
	if a.p != nil {
		return a.p
	}
	b := a.data
	if a.scanResp != nil {
		b = append(b, a.scanResp.data...)
	}
	return adv.NewRawPacket(b)
}

func (a *ExtendedAdvertisingData) EventType() uint16 {
	return a.eventType
}

func (a *ExtendedAdvertisingData) SetScanResp(d *ExtendedAdvertisingData) {
	a.scanResp = d
}

// LocalName returns the LocalName of the remote peripheral.
func (a *ExtendedAdvertisingData) LocalName() string {
	if a.packets().LocalName() != "" {
		return a.packets().LocalName()
	}
	if a.scanResp != nil && a.scanResp.LocalName() != "" {
		return a.scanResp.LocalName()
	}
	return ""
}

// ManufacturerData returns the ManufacturerData of the advertisement.
func (a *ExtendedAdvertisingData) ManufacturerData() []byte {
	return a.packets().ManufacturerData()
}

// ServiceData returns the service data of the advertisement.
func (a *ExtendedAdvertisingData) ServiceData() []ble.ServiceData {
	return a.packets().ServiceData()
}

// Services returns the service UUIDs of the advertisement.
func (a *ExtendedAdvertisingData) Services() []ble.UUID {
	return a.packets().UUIDs()
}

// OverflowService returns the UUIDs of overflowed services.
func (a *ExtendedAdvertisingData) OverflowService() []ble.UUID {
	return a.packets().UUIDs()
}

// TxPowerLevel returns the tx power level of the remote peripheral.
func (a *ExtendedAdvertisingData) TxPowerLevel() int {
	pwr, _ := a.packets().TxPower()
	return pwr
}

// SolicitedService returns UUIDs of solicited services.
func (a *ExtendedAdvertisingData) SolicitedService() []ble.UUID {
	return a.packets().ServiceSol()
}

// Connectable indicates whether the remote peripheral is connectable.
func (a *ExtendedAdvertisingData) Connectable() bool {
	return a.eventType == evtTypAdvDirectInd || a.eventType == evtTypAdvInd
}

func (a *ExtendedAdvertisingData) RSSI() int {
	return int(a.rssi)
}

// Addr returns the address of the remote peripheral.
func (a *ExtendedAdvertisingData) Addr() ble.Addr {
	addr := net.HardwareAddr([]byte{a.address[5], a.address[4], a.address[3], a.address[2], a.address[1], a.address[0]})
	if a.addressType == 1 {
		return RandomAddress{addr}
	}
	return addr
}

// Data returns the extended advertising data of the packet.
// This is Linux specific.
func (a *ExtendedAdvertisingData) Data() []byte {
	return a.data
}

// ScanResponse returns the scan response of the extended packet, if present.
// This is Linux specific.
func (a *ExtendedAdvertisingData) ScanResponse() []byte {
	if a.scanResp == nil {
		return nil
	}
	return a.scanResp.Data()
}

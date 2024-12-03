package evt

import (
	"encoding/binary"
	"fmt"
)

func (e CommandComplete) NumHCICommandPackets() uint8 { return e[0] }
func (e CommandComplete) CommandOpcode() uint16       { return binary.LittleEndian.Uint16(e[1:]) }
func (e CommandComplete) ReturnParameters() []byte    { return e[3:] }

// Per-spec [Vol 2, Part E, 7.7.19], the packet structure should be:
//
//     NumOfHandle, HandleA, HandleB, CompPktNumA, CompPktNumB
//
// But we got the actual packet from BCM20702A1 with the following structure instead.
//
//     NumOfHandle, HandleA, CompPktNumA, HandleB, CompPktNumB
//              02,   40 00,       01 00,   41 00,       01 00

func (e NumberOfCompletedPackets) NumberOfHandles() uint8 { return e[0] }
func (e NumberOfCompletedPackets) ConnectionHandle(i int) uint16 {
	// return binary.LittleEndian.Uint16(e[1+i*2:])
	return binary.LittleEndian.Uint16(e[1+i*4:])
}
func (e NumberOfCompletedPackets) HCNumOfCompletedPackets(i int) uint16 {
	// return binary.LittleEndian.Uint16(e[1+int(e.NumberOfHandles())*2:])
	return binary.LittleEndian.Uint16(e[1+i*4+2:])
}
func (e LEAdvertisingReport) SubeventCode() uint8     { return e[0] }
func (e LEAdvertisingReport) NumReports() uint8       { return e[1] }
func (e LEAdvertisingReport) EventType(i int) uint8   { return e[2+i] }
func (e LEAdvertisingReport) AddressType(i int) uint8 { return e[2+int(e.NumReports())*1+i] }
func (e LEAdvertisingReport) Address(i int) [6]byte {
	e = e[2+int(e.NumReports())*2:]
	b := [6]byte{}
	copy(b[:], e[6*i:])
	return b
}

func (e LEAdvertisingReport) LengthData(i int) uint8 { return e[2+int(e.NumReports())*8+i] }

func (e LEAdvertisingReport) Data(i int) []byte {
	l := 0
	for j := 0; j < i; j++ {
		l += int(e.LengthData(j))
	}
	b := e[2+int(e.NumReports())*9+l:]
	return b[:e.LengthData(i)]
}

func (e LEAdvertisingReport) RSSI(i int) int8 {
	l := 0
	for j := 0; j < int(e.NumReports()); j++ {
		l += int(e.LengthData(j))
	}
	return int8(e[2+int(e.NumReports())*9+l+i])
}

// LEExtendedAdvertisingReport
// 7.7.65.13 LE Extended Advertising Report
// sample
// [0] Subevent_Code
// 0x0d
// [1] Num_Reports
// 0x01
// [2,3] Num_Reports[0].Event_Type
// 0x10 0x00
// [4] Num_Reports[0].Address_Type
// 0x01
// [5,6,7,8,9,10] Num_Reports[0].Address
// 0x87 0xe8 0xdb 0x97 0x13 0x75
// [11] Num_Reports[0].Primary_PHY
// 0x01
// [12] Num_Reports[0].Secondary_PHY
// 0x00
// [13] Num_Reports[0].Advertising_SID
// 0xff
// [14] Num_Reports[0].TX_Power
// 0x7f
// [15] Num_Reports[0].RSSI
// 0xde
// [16,17] Num_Reports[0].Periodic_Advertising_Interval
// 0x00 0x00
// [18] Num_Reports[0].Direct_Address_Type
// 0x00
// [19,20,21,22,23,24] Num_Reports[0].Direct_Address_Type
// 0x00 0x00 0x00 0x00 0x00 0x00
// [25] Num_Reports[0].Data_Length
// 0x1b
// [26~x] Num_Reports[0].Data
// 0x02  0x01  0x1a  0x17  0xff  0x4c  0x00  0x09  0x08  0x13  0x01  0xc0  0xa8  0x00  0x03  0x1b  0x58  0x16  0x08  0x00  0x4e  0x6e  0xe9  0x0b  0x53  0x48  0xdf

const (
	LEExtendedAdvertisingReportMetaDataLength  = 2
	LEExtendedAdvertisingReportFixedDataLength = 24
)

type ExtendedAdvertisingReport struct {
	SubeventCode uint8
	NumReports   uint8
	Reports      []ExtendedAdvertisingData
}

// AdvertisingReport represents an individual report within the LE Extended Advertising Report
type ExtendedAdvertisingData struct {
	EventType                   uint16
	AddressType                 uint8
	Address                     [6]byte
	PrimaryPHY                  uint8
	SecondaryPHY                uint8
	AdvertisingSID              uint8
	TXPower                     int8
	RSSI                        int8
	PeriodicAdvertisingInterval uint16
	DirectAddressType           uint8
	DirectAddress               [6]byte
	DataLength                  uint8
	Data                        []byte
	ScanResp                    *ExtendedAdvertisingData
}

// ParseLEExtendedAdvertisingReport parses a byte array into a leExtendedAdvertisingReport structure
func NewExtendedAdvertisingReport(data LEExtendedAdvertisingReport) (
	*ExtendedAdvertisingReport,
	error,
) {
	if len(data) < 2 {
		return nil, fmt.Errorf("invalid data length")
	}

	report := &ExtendedAdvertisingReport{
		SubeventCode: data[0],
		NumReports:   data[1],
	}

	offset := 2
	for i := 0; i < int(report.NumReports); i++ {
		if offset+26 > len(data) {
			return nil, fmt.Errorf("invalid data length for report %d", i)
		}

		// Parse individual report
		r := ExtendedAdvertisingData{
			EventType:                   uint16(data[offset]) | uint16(data[offset+1])<<8,
			AddressType:                 data[offset+2],
			PrimaryPHY:                  data[offset+11],
			SecondaryPHY:                data[offset+12],
			AdvertisingSID:              data[offset+13],
			TXPower:                     int8(data[offset+14]),
			RSSI:                        int8(data[offset+15]),
			PeriodicAdvertisingInterval: uint16(data[offset+16]) | uint16(data[offset+17])<<8,
			DirectAddressType:           data[offset+18],
			DataLength:                  data[offset+25],
		}
		copy(r.Address[:], data[offset+3:offset+9])
		copy(r.DirectAddress[:], data[offset+19:offset+25])

		// Parse data field
		dataEnd := offset + 26 + int(r.DataLength)
		if dataEnd > len(data) {
			return nil, fmt.Errorf("invalid data length for report %d data", i)
		}
		r.Data = data[offset+26 : dataEnd]

		// Add to report list
		report.Reports = append(report.Reports, r)
		offset = dataEnd
	}

	return report, nil
}

func (e LEExtendedAdvertisingReport) SubeventCode() uint8 { return e[0] }
func (e LEExtendedAdvertisingReport) NumReports() uint8   { return e[1] }

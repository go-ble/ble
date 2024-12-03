package hci

import (
	"context"
	"encoding/binary"
	"fmt"
	"github.com/go-ble/ble/linux/hci/cmd"
	"net"
	"time"

	"github.com/go-ble/ble"
	"github.com/go-ble/ble/linux/adv"
	"github.com/go-ble/ble/linux/gatt"
	"github.com/pkg/errors"
)

// Addr ...
func (h *HCI) Addr() ble.Addr { return h.addr }

// SetAdvHandler ...
func (h *HCI) SetAdvHandler(ah ble.AdvHandler) error {
	h.advHandler = ah
	return nil
}

// SetExtendedAdvHandler ...
func (h *HCI) SetExtendedAdvHandler(ah ble.ExtendedAdvHandler) error {
	h.extendedAdvHandler = ah
	return nil
}

// Scan starts scanning.
func (h *HCI) Scan(allowDup bool) error {
	h.params.scanEnable.FilterDuplicates = 1
	if allowDup {
		h.params.scanEnable.FilterDuplicates = 0
	}
	h.params.scanEnable.LEScanEnable = 1
	h.adHist = make([]*Advertisement, 128)
	h.adLast = 0
	return h.Send(&h.params.scanEnable, nil)
}

// StopScanning stops scanning.
func (h *HCI) Reset() error {
	return h.ResetHCI()
}

// StopScanning stops scanning.
func (h *HCI) StopScanning() error {
	h.params.scanEnable.LEScanEnable = 0
	return h.Send(&h.params.scanEnable, nil)
}

// LESetExtendedScanParameters starts scanning.
func (h *HCI) LESetExtendedScanParameters() error {
	//h.params.extendedScanParams = cmd.LESetExtendedScanParameters{
	//	OwnAddressType:       0x00,   // 0x00: public, 0x01: random
	//	ScanningFilterPolicy: 0x00,   // 0x00: accept all, 0x01: ignore non-white-listed
	//	ScanningPHYs:         0x05,   // 0x01: Scan on the LE 1M PHY, 0x04: Scan on the LE Coded PHY
	//	ScanType1M:           0x01,   // 0x00: passive scan, 0x01: active scan
	//	ScanInterval1M:       0x0064, // 0x0004 - 0x4000; N * 0.625 msec (2.5 ms)
	//	ScanWindow1M:         0x0032, // 0x0004 - 0x4000; N * 0.625 msec (2.5 ms)
	//	ScanTypeCoded:        0x01,   // 0x00: passive scan, 0x01: active scan (for Coded PHY)
	//	ScanIntervalCoded:    0x0BB8, // N * 0.625 msec, scan interval for Coded PHY
	//	ScanWindowCoded:      0x0032, // N * 0.625 msec, scan window for Coded PHY
	//}
	return h.Send(&h.params.extendedScanParams, nil)
}

// LeSetDefaultPHYForExtendedScan starts scanning.
func (h *HCI) LeSetDefaultPHYForExtendedScan() error {
	h.params.leSetDefaultPHY = cmd.LESetDefaultPHY{
		AllPHYs: 0x00, // 0x00: No preference, use all PHYs
		TXPHYs:  0x07, // 0x01: LE 1M, 0x02: LE 2M, 0x04: LE Coded (0x07 to use all PHYs)
		RXPHYs:  0x07, // 0x01: LE 1M, 0x02: LE 2M, 0x04: LE Coded (uses all PHYs at 0x07)
	}
	return h.Send(&h.params.leSetDefaultPHY, nil)
}

// ExtendedScan starts scanning.
func (h *HCI) ExtendedScan(allowDup bool) error {
	h.params.extendedScanEnable = cmd.LESetExtendedScanEnable{
		FilterDuplicates: 0x01,
		Duration:         0xBB8,
		Period:           0x12C,
	}
	if allowDup {
		h.params.extendedScanEnable.FilterDuplicates = 0
	}
	h.params.extendedScanEnable.Enable = 1
	h.extendedAdHist = make([]*ExtendedAdvertisingData, 128)
	h.extendedAdLast = 0
	return h.Send(&h.params.extendedScanEnable, nil)
}

// StopExtendedScan stops scanning.
func (h *HCI) StopExtendedScan() error {
	h.params.extendedScanEnable.Enable = 0
	return h.Send(&h.params.extendedScanEnable, nil)
}

// AdvertiseAdv advertises a given Advertisement
func (h *HCI) AdvertiseAdv(a ble.Advertisement) error {
	ad, err := adv.NewPacket(adv.Flags(adv.FlagGeneralDiscoverable | adv.FlagLEOnly))
	if err != nil {
		return err
	}
	f := adv.AllUUID

	// Current length of ad packet plus two bytes of length and tag.
	l := ad.Len() + 1 + 1
	for _, u := range a.Services() {
		l += u.Len()
	}
	if l > adv.MaxEIRPacketLength {
		f = adv.SomeUUID
	}
	for _, u := range a.Services() {
		if err := ad.Append(f(u)); err != nil {
			if err == adv.ErrNotFit {
				break
			}
			return err
		}
	}
	sr, _ := adv.NewPacket()
	if a.LocalName() != "" {
		switch {
		case ad.Append(adv.CompleteName(a.LocalName())) == nil:
		case sr.Append(adv.CompleteName(a.LocalName())) == nil:
		case sr.Append(adv.ShortName(a.LocalName())) == nil:
		}
	}

	if a.ManufacturerData() != nil {
		man := a.ManufacturerData()
		id := binary.LittleEndian.Uint16(man[0:2])
		manufacuturerData := adv.ManufacturerData(id, man[2:])
		switch {
		case ad.Append(manufacuturerData) == nil:
		case sr.Append(manufacuturerData) == nil:
		}
	}
	if err := h.SetAdvertisement(ad.Bytes(), sr.Bytes()); err != nil {
		return nil
	}
	return h.Advertise()

}

// AdvertiseNameAndServices advertises device name, and specified service UUIDs.
// It tries to fit the UUIDs in the advertising data as much as possible.
// If name doesn't fit in the advertising data, it will be put in scan response.
func (h *HCI) AdvertiseNameAndServices(name string, uuids ...ble.UUID) error {
	ad, err := adv.NewPacket(adv.Flags(adv.FlagGeneralDiscoverable | adv.FlagLEOnly))
	if err != nil {
		return err
	}
	f := adv.AllUUID

	// Current length of ad packet plus two bytes of length and tag.
	l := ad.Len() + 1 + 1
	for _, u := range uuids {
		l += u.Len()
	}
	if l > adv.MaxEIRPacketLength {
		f = adv.SomeUUID
	}
	for _, u := range uuids {
		if err := ad.Append(f(u)); err != nil {
			if err == adv.ErrNotFit {
				break
			}
			return err
		}
	}
	sr, _ := adv.NewPacket()
	switch {
	case ad.Append(adv.CompleteName(name)) == nil:
	case sr.Append(adv.CompleteName(name)) == nil:
	case sr.Append(adv.ShortName(name)) == nil:
	}
	if err := h.SetAdvertisement(ad.Bytes(), sr.Bytes()); err != nil {
		return nil
	}
	return h.Advertise()
}

// AdvertiseMfgData avertises the given manufacturer data.
func (h *HCI) AdvertiseMfgData(id uint16, md []byte) error {
	ad, err := adv.NewPacket(adv.ManufacturerData(id, md))
	if err != nil {
		return err
	}
	if err := h.SetAdvertisement(ad.Bytes(), nil); err != nil {
		return nil
	}
	return h.Advertise()
}

// AdvertiseServiceData16 advertises data associated with a 16bit service uuid
func (h *HCI) AdvertiseServiceData16(id uint16, b []byte) error {
	ad, err := adv.NewPacket(adv.ServiceData16(id, b))
	if err != nil {
		return err
	}
	if err := h.SetAdvertisement(ad.Bytes(), nil); err != nil {
		return nil
	}
	return h.Advertise()
}

// AdvertiseIBeaconData advertise iBeacon with given manufacturer data.
func (h *HCI) AdvertiseIBeaconData(md []byte) error {
	ad, err := adv.NewPacket(adv.IBeaconData(md))
	if err != nil {
		return err
	}
	if err := h.SetAdvertisement(ad.Bytes(), nil); err != nil {
		return nil
	}
	return h.Advertise()
}

// AdvertiseIBeacon advertises iBeacon with specified parameters.
func (h *HCI) AdvertiseIBeacon(u ble.UUID, major, minor uint16, pwr int8) error {
	ad, err := adv.NewPacket(adv.IBeacon(u, major, minor, pwr))
	if err != nil {
		return err
	}
	if err := h.SetAdvertisement(ad.Bytes(), nil); err != nil {
		return nil
	}
	return h.Advertise()
}

// StopAdvertising stops advertising.
func (h *HCI) StopAdvertising() error {
	h.params.advEnable.AdvertisingEnable = 0
	return h.Send(&h.params.advEnable, nil)
}

// Accept starts advertising and accepts connection.
func (h *HCI) Accept() (ble.Conn, error) {
	var tmo <-chan time.Time
	if h.listenerTmo != time.Duration(0) {
		tmo = time.After(h.listenerTmo)
	}
	select {
	case <-h.done:
		return nil, h.err
	case c := <-h.chSlaveConn:
		return c, nil
	case <-tmo:
		return nil, fmt.Errorf("listner timed out")
	}
}

// Dial ...
func (h *HCI) Dial(ctx context.Context, a ble.Addr) (ble.Client, error) {
	b, err := net.ParseMAC(a.String())
	if err != nil {
		return nil, ErrInvalidAddr
	}
	h.params.connParams.PeerAddress = [6]byte{b[5], b[4], b[3], b[2], b[1], b[0]}
	if _, ok := a.(RandomAddress); ok {
		h.params.connParams.PeerAddressType = 1
	}
	if err = h.Send(&h.params.connParams, nil); err != nil {
		return nil, err
	}
	var tmo <-chan time.Time
	if h.dialerTmo != time.Duration(0) {
		tmo = time.After(h.dialerTmo)
	}

	select {
	case <-ctx.Done():
		return h.cancelDial()
	case <-tmo:
		return h.cancelDial()
	case <-h.done:
		return nil, h.err
	case c := <-h.chMasterConn:
		return gatt.NewClient(c)

	}
}

// cancelDial cancels the Dialing
func (h *HCI) cancelDial() (ble.Client, error) {
	err := h.Send(&h.params.connCancel, nil)
	if err == nil {
		// The pending connection was canceled successfully.
		return nil, fmt.Errorf("connection canceled")
	}
	// The connection has been established, the cancel command
	// failed with ErrDisallowed.
	if err == ErrDisallowed {
		return gatt.NewClient(<-h.chMasterConn)
	}
	return nil, errors.Wrap(err, "cancel connection failed")
}

// Advertise starts advertising.
func (h *HCI) Advertise() error {
	h.params.advEnable.AdvertisingEnable = 1
	return h.Send(&h.params.advEnable, nil)
}

// SetAdvertisement sets advertising data and scanResp.
func (h *HCI) SetAdvertisement(ad []byte, sr []byte) error {
	if len(ad) > adv.MaxEIRPacketLength || len(sr) > adv.MaxEIRPacketLength {
		return ble.ErrEIRPacketTooLong
	}

	h.params.advData.AdvertisingDataLength = uint8(len(ad))
	copy(h.params.advData.AdvertisingData[:], ad)
	if err := h.Send(&h.params.advData, nil); err != nil {
		return err
	}

	h.params.scanResp.ScanResponseDataLength = uint8(len(sr))
	copy(h.params.scanResp.ScanResponseData[:], sr)
	if err := h.Send(&h.params.scanResp, nil); err != nil {
		return err
	}
	return nil
}

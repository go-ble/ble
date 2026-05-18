package evt

import "encoding/binary"

const LEPHYUpdateCompleteSubCode = 0x0C

// LEPHYUpdateComplete implements LE PHY Update Complete (0x3E:0x0C) [Vol 2, Part E, 7.7.65.12].
type LEPHYUpdateComplete []byte

func (r LEPHYUpdateComplete) SubeventCode() uint8 { return r[0] }

func (r LEPHYUpdateComplete) Status() uint8 { return r[1] }

func (r LEPHYUpdateComplete) ConnectionHandle() uint16 {
	return binary.LittleEndian.Uint16(r[2:])
}

func (r LEPHYUpdateComplete) TXPHY() uint8 { return r[4] }

func (r LEPHYUpdateComplete) RXPHY() uint8 { return r[5] }

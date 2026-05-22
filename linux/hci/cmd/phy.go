package cmd

// LEReadPHY implements LE Read PHY (0x08|0x0030) [Vol 2, Part E, 7.8.47]
type LEReadPHY struct {
	ConnectionHandle uint16
}

func (c *LEReadPHY) String() string {
	return "LE Read PHY (0x08|0x0030)"
}

func (c *LEReadPHY) OpCode() int { return 0x08<<10 | 0x0030 }

func (c *LEReadPHY) Len() int { return 2 }

func (c *LEReadPHY) Marshal(b []byte) error {
	return marshal(c, b)
}

// LEReadPHYRP returns the return parameter of LE Read PHY
type LEReadPHYRP struct {
	Status           uint8
	ConnectionHandle uint16
	TXPHY            uint8
	RXPHY            uint8
}

func (c *LEReadPHYRP) Unmarshal(b []byte) error {
	return unmarshal(c, b)
}

// LESetDefaultPHY implements LE Set Default PHY (0x08|0x0031) [Vol 2, Part E, 7.8.48]
type LESetDefaultPHY struct {
	ALLPHYS uint8
	TXPHYS  uint8
	RXPHYS  uint8
}

func (c *LESetDefaultPHY) String() string {
	return "LE Set Default PHY (0x08|0x0031)"
}

func (c *LESetDefaultPHY) OpCode() int { return 0x08<<10 | 0x0031 }

func (c *LESetDefaultPHY) Len() int { return 3 }

func (c *LESetDefaultPHY) Marshal(b []byte) error {
	return marshal(c, b)
}

// LESetDefaultPHYRP returns the return parameter of LE Set Default PHY
type LESetDefaultPHYRP struct {
	Status uint8
}

func (c *LESetDefaultPHYRP) Unmarshal(b []byte) error {
	return unmarshal(c, b)
}

// LESetPHY implements LE Set PHY (0x08|0x0032) [Vol 2, Part E, 7.8.49]
type LESetPHY struct {
	ConnectionHandle uint16
	ALLPHYS          uint8
	TXPHYS           uint8
	RXPHYS           uint8
	PHYOptions       uint16
}

func (c *LESetPHY) String() string {
	return "LE Set PHY (0x08|0x0032)"
}

func (c *LESetPHY) OpCode() int { return 0x08<<10 | 0x0032 }

func (c *LESetPHY) Len() int { return 7 }

func (c *LESetPHY) Marshal(b []byte) error {
	return marshal(c, b)
}

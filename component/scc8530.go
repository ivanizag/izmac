package component

/*
SCC8530 is the Zilog 8530 serial controller. Only as much of it as the
Macintosh mouse needs is here: the two data carrier detect inputs, which is where the mouse interrupt
signals X1 and Y1 arrive, and the registers the ROM reads to find out which
of them moved.

	sccRBase $9ffff8   read      bCtl +0   aCtl +2   bData +4   aData +6
	sccWBase $bffff9   write

A channel is reached by writing a register number to its control address and
then reading or writing that address again; the pointer falls back to zero
afterwards, so a bare read of the control address returns RR0. RR0 carries
the data carrier detect on bit 3. RR2 of channel B carries the vector the
level 2 handler dispatches on, with the source on its bits 3 to 1: $2 for the
external status of channel B, which is the Y axis, and $a for channel A,
which is the X axis.
*/
type SCC8530 struct {
	channels [2]channel
}

// channel is one of the two serial channels
type channel struct {
	// pointer is the register the next access reaches
	pointer uint8

	// write holds the write registers, of which only the interrupt enables
	// matter here
	write [16]uint8

	// dcd is the data carrier detect input, driven by the mouse
	dcd bool

	// dcdLatched is what RR0 reports, which the handler compares against
	// what it saw last time to find the line that moved
	dcdLatched bool

	// interrupt is set by a transition and cleared when the handler resets
	// the external status latches
	interrupt bool
}

const (
	// The two channels. On the Macintosh the A carries the modem port and
	// the x axis of the mouse, the B the printer port and the y axis.
	ChannelB = 0
	ChannelA = 1

	// The offsets from the base of each of the two ports
	sccOffsetBControl = 0
	sccOffsetAControl = 2
	sccOffsetBData    = 4
	sccOffsetAData    = 6

	// rr0Dcd is the data carrier detect bit of the status register
	rr0Dcd uint8 = 1 << 3

	// wr1ExternalInterrupt enables the external status interrupt
	wr1ExternalInterrupt uint8 = 1 << 0

	/*
		The commands on the bits 3 to 5 of a write to register 0. Point
		High is the one that matters here: the pointer is only three bits
		wide, so the registers 8 to 15 are reached by asking for the one
		eight lower and setting this command in the same byte. A write of
		$0f selects the register 15 and not the register 7, and reading
		the register 15 back is how the interrupt handler finds out that
		the carrier detect was what moved.
	*/
	wr0PointHigh           uint8 = 1 << 3
	wr0ResetExternalStatus uint8 = 2 << 3
	wr0CommandMask         uint8 = 7 << 3

	// wr15DcdInterrupt enables the interrupt on a carrier detect
	// transition, and is what the handler reads back to identify it
	wr15DcdInterrupt uint8 = 1 << 3

	// The vectors of RR2 on channel B, with the source on the bits 3 to 1
	vectorBExternalStatus uint8 = 0x2
	vectorAExternalStatus uint8 = 0xa
)

// NewSCC8530 returns a chip with both channels quiet
func NewSCC8530() *SCC8530 {
	return &SCC8530{}
}

// Read returns a register of one of the two channels
func (s *SCC8530) Read(channel int, control bool) uint8 {
	c := &s.channels[channel]

	if !control {
		return 0
	}

	register := c.pointer
	c.pointer = 0

	switch register {
	case 0:
		return c.readStatus()
	case 2:
		// Only channel B carries the vector the dispatch uses
		if channel == ChannelB {
			return s.vector()
		}
		return 0
	}

	// The rest read back what was written, which is all the handler wants
	// of them: it reads the register 15 to tell a carrier detect change
	// from the other things that share the interrupt
	return c.write[register&0x0f]
}

// Write sets a register of one of the two channels
func (s *SCC8530) Write(channel int, control bool, value uint8) {
	c := &s.channels[channel]

	if !control {
		return
	}

	register := c.pointer
	c.pointer = 0

	if register == 0 {
		// A write to register 0 carries a command on its high bits and
		// the next register on its low ones
		command := value & wr0CommandMask
		if command == wr0ResetExternalStatus {
			c.resetExternalStatus()
		}

		c.pointer = value & 0x07
		if command == wr0PointHigh {
			c.pointer |= 8
		}
		return
	}

	c.write[register&0x0f] = value
}

// readStatus is RR0, which reports the pins as they were last latched
func (c *channel) readStatus() uint8 {
	var status uint8
	if c.dcdLatched {
		status |= rr0Dcd
	}
	return status
}

/*
vector is RR2 of channel B, the modified vector the level 2 handler reads to
find out what happened. Channel A is the higher priority of the two.
*/
func (s *SCC8530) vector() uint8 {
	if s.channels[ChannelA].interrupt {
		return vectorAExternalStatus
	}
	if s.channels[ChannelB].interrupt {
		return vectorBExternalStatus
	}
	return vectorBExternalStatus
}

/*
setDcd drives one of the data carrier detect pins. A transition either way
raises the interrupt, and the handler compares the latched state with what it
saw before to work out which way the mouse moved.
*/
func (s *SCC8530) SetDcd(channel int, level bool) {
	c := &s.channels[channel]
	if c.dcd == level {
		return
	}
	c.dcd = level

	if c.write[1]&wr1ExternalInterrupt != 0 && c.write[15]&wr15DcdInterrupt != 0 {
		c.interrupt = true
	}
	c.dcdLatched = level
}

/*
resetExternalStatus clears the latch and the interrupt of one channel, which
is what the handler does once it has read the status. It is per channel and
not for the whole chip: the two axes of the mouse are one channel each and
move at the same time, so clearing both here loses every transition of the
axis whose turn it was not.
*/
func (c *channel) resetExternalStatus() {
	c.interrupt = false
	c.dcdLatched = c.dcd
}

// pending tells if a channel still has an interrupt the processor has not
// answered, which is what keeps the mouse from moving out from under it
func (s *SCC8530) Pending(channel int) bool {
	return s.channels[channel].interrupt
}

// interruptAsserted returns the state of the interrupt line, the level 2 of
// the processor
func (s *SCC8530) InterruptAsserted() bool {
	return s.channels[ChannelA].interrupt || s.channels[ChannelB].interrupt
}

func (s *SCC8530) Reset() {
	*s = SCC8530{}
}

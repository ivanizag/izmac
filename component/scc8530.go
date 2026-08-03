package component

/*
SCC8530 is the Zilog 8530 serial controller. Two things go through it here:
the mouse, whose interrupt signals X1 and Y1 arrive on the two data carrier
detect inputs, and whatever is listening on a serial port, which is reached
by the transmitter.

	sccRBase $9ffff8   read      bCtl +0   aCtl +2   bData +4   aData +6
	sccWBase $bffff9   write

A channel is reached by writing a register number to its control address and
then reading or writing that address again; the pointer falls back to zero
afterwards, so a bare read of the control address returns RR0. RR0 carries
the data carrier detect on bit 3 and the transmit buffer empty on bit 2. RR2
of channel B carries the vector the level 2 handler dispatches on, with the
source on its bits 3 to 1: $0 and $8 for the transmitters of the channels B
and A, $2 and $a for their external status, which is where the Y and the X
axis of the mouse arrive.

The receive side is not here. Nothing on the other end of the wire talks
back: a printer is written to and says nothing, and the flow control it would
answer with is not needed by something that never has to be waited for.
*/
type SCC8530 struct {
	channels [2]channel
}

/*
TransmitSink is whatever is on the other end of a serial port. The chip hands
it every byte it shifts out and knows nothing else about it.
*/
type TransmitSink interface {
	Transmit(value uint8)
}

// channel is one of the two serial channels
type channel struct {
	// pointer is the register the next access reaches
	pointer uint8

	// write holds the write registers, of which the interrupt enables and
	// the baud rate generator matter here
	write [16]uint8

	// dcd is the data carrier detect input, driven by the mouse
	dcd bool

	// dcdLatched is what RR0 reports, which the handler compares against
	// what it saw last time to find the line that moved
	dcdLatched bool

	// extInterrupt is set by a transition of the carrier detect and cleared
	// when the handler resets the external status latches
	extInterrupt bool

	// sink is what the bytes shifted out of this channel go to, or nil when
	// there is nothing on the port. A byte still takes the time it would
	// take either way.
	sink TransmitSink

	// txByte is the byte on its way out and txCycles what is left of the
	// time it takes to go, or zero when the transmitter is idle
	txByte   uint8
	txCycles uint64

	// txBuffer is a byte handed over while the wire was busy, waiting for
	// its turn on it
	txBuffer   uint8
	txBuffered bool

	// txInterrupt is set when the byte has gone and the buffer is free
	// again, and cleared by the reset transmit interrupt pending command
	txInterrupt bool

	// cyclesPerSecond is how many of the cycles the chip is ticked with go
	// by in a second, which is what turns a baud rate into a time
	cyclesPerSecond uint64
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

	/*
		The bits of RR0. The transmit buffer empty is what the driver polls
		before handing over every byte. The clear to send is left low: it
		is the handshake input of the port and the driver reads a high one
		as a reason to hold the output back, so a printer that is always
		ready is one that never raises it.
	*/
	rr0TxBufferEmpty uint8 = 1 << 2
	rr0Dcd           uint8 = 1 << 3

	// rr1AllSent says the transmitter has gone quiet, which is what the
	// driver waits for before it closes the port
	rr1AllSent uint8 = 1 << 0

	// wr1ExternalInterrupt enables the external status interrupt and
	// wr1TxInterrupt the one that asks for the next byte
	wr1ExternalInterrupt uint8 = 1 << 0
	wr1TxInterrupt       uint8 = 1 << 1

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
	wr0ResetTxInterrupt    uint8 = 5 << 3
	wr0CommandMask         uint8 = 7 << 3

	// wr15DcdInterrupt enables the interrupt on a carrier detect
	// transition, and is what the handler reads back to identify it
	wr15DcdInterrupt uint8 = 1 << 3

	// wr4ParityEnable puts a parity bit on the wire after the data bits
	wr4ParityEnable uint8 = 1 << 0

	// The vectors of RR2 on channel B, with the source on the bits 3 to 1
	vectorBTxEmpty        uint8 = 0x0
	vectorBExternalStatus uint8 = 0x2
	vectorATxEmpty        uint8 = 0x8
	vectorAExternalStatus uint8 = 0xa

	// sccClockHz is the clock the Macintosh feeds the chip, which is what
	// the baud rate generator counts down
	sccClockHz = 3_672_000

	// defaultBaudRate is what a channel is taken to run at when its
	// generator has not been programmed into saying anything sensible
	defaultBaudRate = 9600
)

/*
NewSCC8530 returns a chip with both channels quiet. The cycles per second is
the rate of the clock Tick() is called with, which is what a baud rate has to
be measured against; a zero is a caller that does not care how long a byte
takes, and then none of them take any time at all.
*/
func NewSCC8530(cyclesPerSecond uint64) *SCC8530 {
	s := &SCC8530{}
	for i := range s.channels {
		s.channels[i].cyclesPerSecond = cyclesPerSecond
	}
	return s
}

// Tick advances both transmitters by the cycles the last instruction took
func (s *SCC8530) Tick(cycles uint64) {
	s.channels[ChannelA].tick(cycles)
	s.channels[ChannelB].tick(cycles)
}

/*
AttachSink puts something on the other end of one of the ports. It survives a
reset, as a cable does.
*/
func (s *SCC8530) AttachSink(channel int, sink TransmitSink) {
	s.channels[channel].sink = sink
}

// Read returns a register of one of the two channels
func (s *SCC8530) Read(channel int, control bool) uint8 {
	c := &s.channels[channel]

	if !control {
		// The receive buffer, RR8, and nothing ever arrives in it
		return 0
	}

	register := c.pointer
	c.pointer = 0

	switch register {
	case 0:
		return c.readStatus()
	case 1:
		return c.readTxStatus()
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
		c.transmit(value)
		return
	}

	register := c.pointer
	c.pointer = 0

	if register == 0 {
		// A write to register 0 carries a command on its high bits and
		// the next register on its low ones
		command := value & wr0CommandMask
		switch command {
		case wr0ResetExternalStatus:
			c.resetExternalStatus()
		case wr0ResetTxInterrupt:
			c.txInterrupt = false
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
	if c.txCycles == 0 {
		status |= rr0TxBufferEmpty
	}
	return status
}

/*
readTxStatus is RR1. Only the all sent bit of it is here: the driver polls it
to know that the last byte has left the shift register before it turns the
port off, and the error bits it shares the register with belong to the
receiver.
*/
func (c *channel) readTxStatus() uint8 {
	if c.txCycles != 0 {
		return 0
	}
	return rr1AllSent
}

/*
transmit takes a byte written to the data register and starts it on its way.

The chip has a buffer as well as a shift register, so it can be given the next
byte before the one going out has left. What is here is the buffer: a byte
written while another is on the wire waits for it rather than being lost, and
the transmit buffer empty of RR0 stays down until the wire is free again. That
is one byte in flight where the real chip has two, and it paces the port at
the baud rate instead of at twice it. The driver asks whether the buffer is
empty before every byte, so it never notices the difference.
*/
func (c *channel) transmit(value uint8) {
	if c.txCycles != 0 {
		// The wire is busy, so the byte waits in the buffer. A byte
		// already waiting there is written over, which is what the chip
		// does to a driver that ignores the status.
		c.txBuffer, c.txBuffered = value, true
		return
	}

	c.start(value)
}

// start puts a byte on the wire for as long as it takes to go
func (c *channel) start(value uint8) {
	c.txByte = value
	c.txCycles = c.byteCycles()

	// A machine that has not said how fast it runs, which is every test
	// that does not care, sends the byte at once
	if c.txCycles == 0 {
		c.sent()
	}
}

// tick moves the byte on its way out along and delivers it when its time on
// the wire is up
func (c *channel) tick(cycles uint64) {
	if c.txCycles == 0 {
		return
	}

	if c.txCycles > cycles {
		c.txCycles -= cycles
		return
	}

	c.txCycles = 0
	c.sent()

	if c.txBuffered {
		c.txBuffered = false
		c.start(c.txBuffer)
	}
}

// sent hands the byte over and asks for the next one
func (c *channel) sent() {
	if c.sink != nil {
		c.sink.Transmit(c.txByte)
	}

	if c.write[1]&wr1TxInterrupt != 0 {
		c.txInterrupt = true
	}
}

/*
byteCycles is how long a byte takes to go out, in processor cycles, worked
out from the registers the driver programmed. The baud rate generator counts
down the constant in the registers 12 and 13 from the 3.672MHz clock the
Macintosh feeds the chip, halved and divided again by the clock mode of the
register 4; the register 5 gives the data bits and the register 4 the parity
and the stop bits, which is the rest of what goes on the wire around them.

The clock mode of one means the clock is coming from a pin rather than from
the generator, and a machine that has not programmed the generator at all
leaves a constant of zero, which is a rate of nearly a megabaud. Neither is
something to take literally, so both fall back to the 9600 baud everything
that has been looked at here actually asks for.
*/
func (c *channel) byteCycles() uint64 {
	if c.cyclesPerSecond == 0 {
		return 0
	}

	baud := uint64(defaultBaudRate)
	if divider := clockModeDividers()[c.write[4]>>6]; divider != 0 {
		constant := uint64(c.write[13])<<8 | uint64(c.write[12])
		baud = sccClockHz / (2 * (constant + 2) * divider)
	}
	if baud == 0 {
		baud = defaultBaudRate
	}

	return c.cyclesPerSecond * c.bitsPerByte() / baud
}

/*
bitsPerByte is what actually goes on the wire for every byte: the start bit,
the data bits, the parity bit if there is one, and the stop bits. The stop
bits can be one and a half of them, which is why this counts in halves.
*/
func (c *channel) bitsPerByte() uint64 {
	halfBits := uint64(2) // the start bit
	halfBits += 2 * dataBitCounts()[(c.write[5]>>5)&0x03]
	if c.write[4]&wr4ParityEnable != 0 {
		halfBits += 2
	}
	halfBits += stopBitHalves()[(c.write[4]>>2)&0x03]

	// Rounding a bit and a half up is the byte taking as long as the
	// slowest thing it can wait for, which is what a receiver does
	return (halfBits + 1) / 2
}

// clockModeDividers is the divider the bits 7 and 6 of the register 4 pick,
// with the clock straight off a pin, which the generator does not drive, as
// a zero
func clockModeDividers() [4]uint64 {
	return [4]uint64{0, 16, 32, 64}
}

// dataBitCounts is the data bits the bits 6 and 5 of the register 5 pick,
// which are not in the order they read in
func dataBitCounts() [4]uint64 {
	return [4]uint64{5, 7, 6, 8}
}

// stopBitHalves is the stop bits the bits 3 and 2 of the register 4 pick,
// counted in halves. The zero is the synchronous modes, which have none.
func stopBitHalves() [4]uint64 {
	return [4]uint64{0, 2, 3, 4}
}

/*
vector is RR2 of channel B, the modified vector the level 2 handler reads to
find out what happened. The order is the priority of the chip: channel A
before channel B, and the transmitter before the external status within a
channel.
*/
func (s *SCC8530) vector() uint8 {
	if s.channels[ChannelA].txInterrupt {
		return vectorATxEmpty
	}
	if s.channels[ChannelA].extInterrupt {
		return vectorAExternalStatus
	}
	if s.channels[ChannelB].txInterrupt {
		return vectorBTxEmpty
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
		c.extInterrupt = true
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
	c.extInterrupt = false
	c.dcdLatched = c.dcd
}

/*
Pending tells if a channel still has a carrier detect interrupt the processor
has not answered, which is what keeps the mouse from moving out from under
it. The transmitter shares the channel and its interrupt is not one of these:
a byte going out of the printer port is no reason to hold the Y axis still.
*/
func (s *SCC8530) Pending(channel int) bool {
	return s.channels[channel].extInterrupt
}

// InterruptAsserted returns the state of the interrupt line, the level 2 of
// the processor
func (s *SCC8530) InterruptAsserted() bool {
	for i := range s.channels {
		if s.channels[i].extInterrupt || s.channels[i].txInterrupt {
			return true
		}
	}
	return false
}

/*
Reset returns the chip to the state it powers up in. What is on the other end
of the ports and how fast the machine runs survive it, as a cable and a
crystal do.
*/
func (s *SCC8530) Reset() {
	for i := range s.channels {
		s.channels[i] = channel{
			sink:            s.channels[i].sink,
			cyclesPerSecond: s.channels[i].cyclesPerSecond,
		}
	}
}

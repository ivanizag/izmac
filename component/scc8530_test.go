package component

import "testing"

// The tests name the channel and the side rather than an address, which is
// the machine's business and not the chip's
const (
	control = true
	data    = false

	// cyclesPerSecondForTest is the clock of the Macintosh Plus, which is
	// what the machine ticks the chip with
	cyclesPerSecondForTest = 7_833_600
)

/*
enableExternalInterrupts sets up a channel the way the ROM does: the master
enable on the register 1, and the carrier detect on the register 15, which is
reached with the point high command because the pointer is only three bits
wide.
*/
func enableExternalInterrupts(s *SCC8530, ch int) {
	s.Write(ch, control, 1)
	s.Write(ch, control, wr1ExternalInterrupt)

	s.Write(ch, control, wr0PointHigh|7)
	s.Write(ch, control, wr15DcdInterrupt)
}

func TestTheControlPortSelectsARegister(t *testing.T) {
	s := NewSCC8530(0)

	// Writing a register number and then the value
	s.Write(ChannelB, control, 1)
	s.Write(ChannelB, control, wr1ExternalInterrupt)

	if s.channels[ChannelB].write[1] != wr1ExternalInterrupt {
		t.Error("the value did not reach the register selected")
	}

	// And the pointer falls back to zero, so a bare read is the status
	if s.channels[ChannelB].pointer != 0 {
		t.Error("the register pointer did not fall back to zero")
	}
}

func TestTheStatusCarriesTheCarrierDetect(t *testing.T) {
	s := NewSCC8530(0)

	if s.Read(ChannelB, control)&rr0Dcd != 0 {
		t.Error("the carrier detect reads high with nothing driving it")
	}

	s.SetDcd(ChannelB, true)
	if s.Read(ChannelB, control)&rr0Dcd == 0 {
		t.Error("the carrier detect did not reach the status register")
	}
}

func TestATransitionRaisesTheInterrupt(t *testing.T) {
	s := NewSCC8530(0)
	enableExternalInterrupts(s, ChannelB)

	if s.InterruptAsserted() {
		t.Fatal("the interrupt is asserted before anything moved")
	}

	s.SetDcd(ChannelB, true)
	if !s.InterruptAsserted() {
		t.Error("a transition did not raise the interrupt")
	}

	// The handler resets the external status latches to clear it
	s.Write(ChannelB, control, wr0ResetExternalStatus)
	if s.InterruptAsserted() {
		t.Error("resetting the external status did not clear the interrupt")
	}
}

func TestBothEdgesInterrupt(t *testing.T) {
	s := NewSCC8530(0)
	enableExternalInterrupts(s, ChannelB)

	for _, level := range []bool{true, false, true} {
		s.SetDcd(ChannelB, level)
		if !s.InterruptAsserted() {
			t.Errorf("the transition to %v did not interrupt", level)
		}
		s.Write(ChannelB, control, wr0ResetExternalStatus)
	}
}

func TestNothingInterruptsWhileItIsDisabled(t *testing.T) {
	s := NewSCC8530(0)

	s.SetDcd(ChannelB, true)
	if s.InterruptAsserted() {
		t.Error("a transition interrupted with the interrupt disabled")
	}
}

/*
The level 2 handler reads RR2 of channel B to find out which channel moved,
with the source on its bits 3 to 1. Channel A is the x axis of the mouse and
channel B the y.
*/
func TestTheVectorNamesTheChannel(t *testing.T) {
	s := NewSCC8530(0)
	enableExternalInterrupts(s, ChannelB)
	enableExternalInterrupts(s, ChannelA)

	s.SetDcd(ChannelB, true)
	s.Write(ChannelB, control, 2)
	if got := s.Read(ChannelB, control); got != vectorBExternalStatus {
		t.Errorf("channel B gave the vector $%02x, wanted $%02x",
			got, vectorBExternalStatus)
	}
	s.Write(ChannelB, control, wr0ResetExternalStatus)

	s.SetDcd(ChannelA, true)
	s.Write(ChannelB, control, 2)
	if got := s.Read(ChannelB, control); got != vectorAExternalStatus {
		t.Errorf("channel A gave the vector $%02x, wanted $%02x",
			got, vectorAExternalStatus)
	}
}

/*
The transmitter. The printer port is written to by putting the byte on the
data side of the channel, and the driver polls the transmit buffer empty bit
of RR0 before every one of them.
*/

// recorder is something on the end of a port that remembers what it was sent
type recorder struct {
	received []uint8
}

func (r *recorder) Transmit(value uint8) {
	r.received = append(r.received, value)
}

// configureFor9600 programs the baud rate generator the way the serial
// driver does for the 9600 baud, eight bits, no parity and one stop bit that
// the printer port is opened with: the time constant 10 counted down from
// the 3.672MHz clock with the divider of 16.
func configureFor9600(s *SCC8530, ch int) {
	s.Write(ch, control, 4)
	s.Write(ch, control, 0x44) // x16 clock, one stop bit, no parity
	s.Write(ch, control, 5)
	s.Write(ch, control, 0x60) // eight data bits
	s.Write(ch, control, 12)
	s.Write(ch, control, 10)
	s.Write(ch, control, 13)
	s.Write(ch, control, 0)
}

func TestAByteReachesWhatIsOnThePort(t *testing.T) {
	s := NewSCC8530(0)
	r := &recorder{}
	s.AttachSink(ChannelB, r)

	s.Write(ChannelB, data, 'A')
	s.Write(ChannelB, data, 'B')

	if string(r.received) != "AB" {
		t.Errorf("the port received %q, wanted %q", r.received, "AB")
	}
}

func TestNothingIsWrittenToAPortWithNothingOnIt(t *testing.T) {
	s := NewSCC8530(0)

	// The bytes fall on the floor, and the chip carries on as if they had
	// gone out, which is what an unplugged port does
	s.Write(ChannelB, data, 'A')

	if s.Read(ChannelB, control)&rr0TxBufferEmpty == 0 {
		t.Error("the buffer did not empty with nothing on the port")
	}
}

func TestTheBufferIsBusyWhileTheByteGoesOut(t *testing.T) {
	// A machine that says how fast it runs gets bytes that take time
	s := NewSCC8530(cyclesPerSecondForTest)
	r := &recorder{}
	s.AttachSink(ChannelB, r)
	configureFor9600(s, ChannelB)

	s.Write(ChannelB, data, 'A')
	if s.Read(ChannelB, control)&rr0TxBufferEmpty != 0 {
		t.Error("the buffer reads empty with a byte still going out")
	}
	if len(r.received) != 0 {
		t.Error("the byte arrived before it had been sent")
	}

	// Ten bits at 9600 baud, and one of them is not enough
	s.Tick(cyclesPerSecondForTest / 9600)
	if len(r.received) != 0 {
		t.Error("the byte arrived a bit into the ten it takes")
	}

	s.Tick(cyclesPerSecondForTest)
	if len(r.received) != 1 {
		t.Error("the byte never arrived")
	}
	if s.Read(ChannelB, control)&rr0TxBufferEmpty == 0 {
		t.Error("the buffer did not empty once the byte had gone")
	}
}

/*
A byte at 9600 baud is a start bit, eight data bits and a stop bit, so a
thousandth of a second and a bit. What matters is that it is that and not the
instant a straight write would be, which is why this allows a tenth either
way rather than pinning the number.
*/
func TestAByteTakesAsLongAsTheBaudRateSaysItDoes(t *testing.T) {
	s := NewSCC8530(cyclesPerSecondForTest)
	configureFor9600(s, ChannelB)

	got := s.channels[ChannelB].byteCycles()
	wanted := uint64(10 * cyclesPerSecondForTest / 9600)

	if got < wanted*9/10 || got > wanted*11/10 {
		t.Errorf("a byte takes %v cycles, wanted about %v", got, wanted)
	}
}

func TestTheTransmitterInterruptsWhenItIsDone(t *testing.T) {
	s := NewSCC8530(0)
	s.Write(ChannelB, control, 1)
	s.Write(ChannelB, control, wr1TxInterrupt)

	s.Write(ChannelB, data, 'A')
	if !s.InterruptAsserted() {
		t.Fatal("the byte going out did not ask for the next one")
	}

	s.Write(ChannelB, control, 2)
	if got := s.Read(ChannelB, control); got != vectorBTxEmpty {
		t.Errorf("the vector was $%02x, wanted $%02x", got, vectorBTxEmpty)
	}

	// The handler with nothing left to send resets the transmit interrupt
	// rather than answering it with another byte
	s.Write(ChannelB, control, wr0ResetTxInterrupt)
	if s.InterruptAsserted() {
		t.Error("the reset transmit interrupt pending command did not clear it")
	}
}

func TestTheTransmitterDoesNotInterruptWhileItIsDisabled(t *testing.T) {
	s := NewSCC8530(0)

	s.Write(ChannelB, data, 'A')
	if s.InterruptAsserted() {
		t.Error("a byte interrupted with the transmit interrupt disabled")
	}
}

/*
The mouse holds an axis still until the processor has read the quadrature
that goes with the edge it interrupted for. A byte going out of the printer
port shares the channel with the y axis and is not one of those.
*/
func TestAByteDoesNotHoldTheMouseUp(t *testing.T) {
	s := NewSCC8530(0)
	s.Write(ChannelB, control, 1)
	s.Write(ChannelB, control, wr1TxInterrupt)

	s.Write(ChannelB, data, 'A')
	if s.Pending(ChannelB) {
		t.Error("the transmitter held the y axis of the mouse still")
	}
}

func TestTheDriverIsToldWhenTheLastByteHasGone(t *testing.T) {
	s := NewSCC8530(cyclesPerSecondForTest)
	configureFor9600(s, ChannelB)

	s.Write(ChannelB, control, 1)
	s.Write(ChannelB, data, 'A')
	if s.Read(ChannelB, control)&rr1AllSent != 0 {
		t.Error("the transmitter said it was done with a byte still going out")
	}

	s.Tick(cyclesPerSecondForTest)
	s.Write(ChannelB, control, 1)
	if s.Read(ChannelB, control)&rr1AllSent == 0 {
		t.Error("the transmitter never said it was done")
	}
}

// What is on the end of a port is a cable, and the machine being reset does
// not unplug it
func TestAResetLeavesThePortConnected(t *testing.T) {
	s := NewSCC8530(0)
	r := &recorder{}
	s.AttachSink(ChannelB, r)

	s.Reset()
	s.Write(ChannelB, data, 'A')

	if len(r.received) != 1 {
		t.Error("the reset unplugged what was on the port")
	}
}

/*
A byte written while the wire is busy waits its turn rather than being thrown
away. The driver looks at the status before every byte and never does this,
but a byte silently lost is the kind of thing that shows up as one wrong dot
in the middle of a page and is never explained.
*/
func TestAByteHandedOverTooEarlyIsNotLost(t *testing.T) {
	s := NewSCC8530(cyclesPerSecondForTest)
	r := &recorder{}
	s.AttachSink(ChannelB, r)
	configureFor9600(s, ChannelB)

	s.Write(ChannelB, data, 'A')
	s.Write(ChannelB, data, 'B')

	s.Tick(cyclesPerSecondForTest)
	s.Tick(cyclesPerSecondForTest)

	if string(r.received) != "AB" {
		t.Errorf("the port received %q, wanted %q", r.received, "AB")
	}
}

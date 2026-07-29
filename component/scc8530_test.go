package component

import "testing"

// The tests name the channel and the side rather than an address, which is
// the machine's business and not the chip's
const (
	control = true
	data    = false
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
	s := NewSCC8530()

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
	s := NewSCC8530()

	if s.Read(ChannelB, control)&rr0Dcd != 0 {
		t.Error("the carrier detect reads high with nothing driving it")
	}

	s.SetDcd(ChannelB, true)
	if s.Read(ChannelB, control)&rr0Dcd == 0 {
		t.Error("the carrier detect did not reach the status register")
	}
}

func TestATransitionRaisesTheInterrupt(t *testing.T) {
	s := NewSCC8530()
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
	s := NewSCC8530()
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
	s := NewSCC8530()

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
	s := NewSCC8530()
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

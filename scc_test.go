package izmac

import "testing"

const sccReadBase = 0x9ffff8

func sccAddress(offset uint32) uint32 {
	return sccReadBase + offset
}

/*
enableExternalInterrupts sets up a channel the way the ROM does: the master
enable on the register 1, and the carrier detect on the register 15, which is
reached with the point high command because the pointer is only three bits
wide.
*/
func enableExternalInterrupts(s *scc, offset uint32) {
	s.poke(sccAddress(offset), 1)
	s.poke(sccAddress(offset), sccWR1ExternalInterrupt)

	s.poke(sccAddress(offset), sccWR0PointHigh|7)
	s.poke(sccAddress(offset), sccWR15DcdInterrupt)
}

func TestTheControlPortSelectsARegister(t *testing.T) {
	s := newScc()

	// Writing a register number and then the value
	s.poke(sccAddress(sccOffsetBControl), 1)
	s.poke(sccAddress(sccOffsetBControl), sccWR1ExternalInterrupt)

	if s.channels[sccChannelB].write[1] != sccWR1ExternalInterrupt {
		t.Error("the value did not reach the register selected")
	}

	// And the pointer falls back to zero, so a bare read is the status
	if s.channels[sccChannelB].pointer != 0 {
		t.Error("the register pointer did not fall back to zero")
	}
}

func TestTheStatusCarriesTheCarrierDetect(t *testing.T) {
	s := newScc()

	if s.peek(sccAddress(sccOffsetBControl))&sccRR0Dcd != 0 {
		t.Error("the carrier detect reads high with nothing driving it")
	}

	s.setDcd(sccChannelB, true)
	if s.peek(sccAddress(sccOffsetBControl))&sccRR0Dcd == 0 {
		t.Error("the carrier detect did not reach the status register")
	}
}

func TestATransitionRaisesTheInterrupt(t *testing.T) {
	s := newScc()
	enableExternalInterrupts(s, sccOffsetBControl)

	if s.interruptAsserted() {
		t.Fatal("the interrupt is asserted before anything moved")
	}

	s.setDcd(sccChannelB, true)
	if !s.interruptAsserted() {
		t.Error("a transition did not raise the interrupt")
	}

	// The handler resets the external status latches to clear it
	s.poke(sccAddress(sccOffsetBControl), sccWR0ResetExternalStatus)
	if s.interruptAsserted() {
		t.Error("resetting the external status did not clear the interrupt")
	}
}

func TestBothEdgesInterrupt(t *testing.T) {
	s := newScc()
	enableExternalInterrupts(s, sccOffsetBControl)

	for _, level := range []bool{true, false, true} {
		s.setDcd(sccChannelB, level)
		if !s.interruptAsserted() {
			t.Errorf("the transition to %v did not interrupt", level)
		}
		s.poke(sccAddress(sccOffsetBControl), sccWR0ResetExternalStatus)
	}
}

func TestNothingInterruptsWhileItIsDisabled(t *testing.T) {
	s := newScc()

	s.setDcd(sccChannelB, true)
	if s.interruptAsserted() {
		t.Error("a transition interrupted with the interrupt disabled")
	}
}

/*
The level 2 handler reads RR2 of channel B to find out which channel moved,
with the source on its bits 3 to 1. Channel A is the x axis of the mouse and
channel B the y.
*/
func TestTheVectorNamesTheChannel(t *testing.T) {
	s := newScc()
	enableExternalInterrupts(s, sccOffsetBControl)
	enableExternalInterrupts(s, sccOffsetAControl)

	s.setDcd(sccChannelB, true)
	s.poke(sccAddress(sccOffsetBControl), 2)
	if got := s.peek(sccAddress(sccOffsetBControl)); got != sccVectorBExternalStatus {
		t.Errorf("channel B gave the vector $%02x, wanted $%02x",
			got, sccVectorBExternalStatus)
	}
	s.poke(sccAddress(sccOffsetBControl), sccWR0ResetExternalStatus)

	s.setDcd(sccChannelA, true)
	s.poke(sccAddress(sccOffsetBControl), 2)
	if got := s.peek(sccAddress(sccOffsetBControl)); got != sccVectorAExternalStatus {
		t.Errorf("channel A gave the vector $%02x, wanted $%02x",
			got, sccVectorAExternalStatus)
	}
}

func TestTheSccIsReachableThroughTheMemoryManager(t *testing.T) {
	mm := newTestMemoryManager(1024)
	s := newScc()
	mm.scc = s
	mm.setOverlay(false)

	mm.Poke(sccAddress(sccOffsetBControl), 1)
	mm.Poke(sccAddress(sccOffsetBControl), sccWR1ExternalInterrupt)

	if s.channels[sccChannelB].write[1] != sccWR1ExternalInterrupt {
		t.Error("the SCC is not reachable through the memory manager")
	}
}

// The mouse drives the two interrupt lines through the SCC, and they have to
// arrive on the channels the ROM expects: A for x, B for y
func TestTheMouseReachesTheSccChannels(t *testing.T) {
	m := newMouse()
	s := newScc()
	enableExternalInterrupts(s, sccOffsetAControl)
	enableExternalInterrupts(s, sccOffsetBControl)

	m.move(4, 0)
	for i := 0; i < 8; i++ {
		m.quadratureRead()
		x1, _, y1, _ := m.tick(true, true)
		s.setDcd(sccChannelA, x1)
		s.setDcd(sccChannelB, y1)
	}

	if !s.channels[sccChannelA].interrupt {
		t.Error("moving along x did not interrupt on channel A")
	}
	if s.channels[sccChannelB].interrupt {
		t.Error("moving along x interrupted on channel B")
	}
}

/*
The pointer is three bits wide, so the registers 8 to 15 are reached with the
point high command in the same byte. A write of $0f is the register 15, not
the register 7, and the interrupt handler reads the register 15 back to tell
a carrier detect change from anything else that shares the interrupt.
*/
func TestPointHighReachesTheUpperRegisters(t *testing.T) {
	s := newScc()

	s.poke(sccAddress(sccOffsetBControl), sccWR0PointHigh|7)
	if s.channels[sccChannelB].pointer != 15 {
		t.Fatalf("$%02x selected the register %v, wanted 15",
			sccWR0PointHigh|7, s.channels[sccChannelB].pointer)
	}

	s.poke(sccAddress(sccOffsetBControl), sccWR15DcdInterrupt)
	if s.channels[sccChannelB].write[15] != sccWR15DcdInterrupt {
		t.Error("the value did not reach the register 15")
	}
	if s.channels[sccChannelB].write[7] != 0 {
		t.Error("the value landed in the register 7 instead")
	}

	// And the handler can read it back
	s.poke(sccAddress(sccOffsetBControl), sccWR0PointHigh|7)
	if got := s.peek(sccAddress(sccOffsetBControl)); got != sccWR15DcdInterrupt {
		t.Errorf("the register 15 reads back $%02x, wanted $%02x",
			got, sccWR15DcdInterrupt)
	}
}

// Without the carrier detect enabled on the register 15 the master enable
// alone must not interrupt
func TestTheCarrierDetectHasToBeEnabledToo(t *testing.T) {
	s := newScc()

	s.poke(sccAddress(sccOffsetBControl), 1)
	s.poke(sccAddress(sccOffsetBControl), sccWR1ExternalInterrupt)

	s.setDcd(sccChannelB, true)
	if s.interruptAsserted() {
		t.Error("a transition interrupted with the carrier detect not enabled")
	}
}

/*
The two axes of the mouse are one channel each and move at the same time, so
the reset has to clear only the channel it was addressed to. Clearing the
whole chip loses every transition of the axis whose turn it was not, which
shows up as a pointer that crawls instead of keeping up.
*/
func TestTheResetOnlyClearsItsOwnChannel(t *testing.T) {
	s := newScc()
	enableExternalInterrupts(s, sccOffsetAControl)
	enableExternalInterrupts(s, sccOffsetBControl)

	s.setDcd(sccChannelA, true)
	s.setDcd(sccChannelB, true)

	s.poke(sccAddress(sccOffsetBControl), sccWR0ResetExternalStatus)

	if s.channels[sccChannelB].interrupt {
		t.Error("the reset did not clear the channel it was sent to")
	}
	if !s.channels[sccChannelA].interrupt {
		t.Error("the reset cleared the other channel as well")
	}
}

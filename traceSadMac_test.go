package izmac

import (
	"strings"
	"testing"
)

func TestTheSadMacCodeIsSplitBetweenTheRegisters(t *testing.T) {
	// The low word of D7 is the class, the high word the internal flags,
	// and D6 is the subcode
	s := newSadMac(0x1234000f, 0x00000003)

	if s.class != 0x000f {
		t.Errorf("the class is $%04x, wanted $000f", s.class)
	}
	if s.flags != 0x1234 {
		t.Errorf("the flags are $%04x, wanted $1234", s.flags)
	}
	if s.subcode != 3 {
		t.Errorf("the subcode is %v, wanted 3", s.subcode)
	}
}

func TestTheRomChecksumFailureIsNamed(t *testing.T) {
	s := newSadMac(sadMacClassRomChecksum, 0)

	got := s.String()
	if !strings.Contains(got, "010000") {
		t.Errorf("%q does not show the code the screen would", got)
	}
	if !strings.Contains(got, "ROM checksum") {
		t.Errorf("%q does not name the test", got)
	}
}

func TestTheExceptionSubcodeIsNamed(t *testing.T) {
	for subcode, wanted := range map[uint32]string{
		1:  "bus error",
		2:  "address error",
		3:  "illegal instruction",
		10: "line 1111 emulator",
	} {
		s := newSadMac(sadMacClassException, subcode)
		if !strings.Contains(s.String(), wanted) {
			t.Errorf("the subcode %v gave %q, wanted it to name the %v",
				subcode, s.String(), wanted)
		}
	}
}

func TestASubcodeThatIsNotAnExceptionIsReported(t *testing.T) {
	s := newSadMac(sadMacClassException, 13)

	if !strings.Contains(s.String(), "not an exception") {
		t.Errorf("%q does not flag the subcode as unknown", s.String())
	}
}

func TestTheMemoryTestsReportTheFailingChips(t *testing.T) {
	s := newSadMac(sadMacClassRamBus, 0x0005)

	if !s.isRamTest() {
		t.Error("the RAM bus subtest is not recognized as a memory test")
	}
	got := s.String()
	if !strings.Contains(got, "failing chips 0, 2") {
		t.Errorf("%q does not list the bits of the mask", got)
	}
}

func TestTheHaltDetectorSpotsATightLoop(t *testing.T) {
	h := newHaltDetector(0x40, 1000)

	// A loop of three instructions with the registers standing still
	halted := false
	for i := 0; i < 500; i++ {
		halted = h.inspect(0x40_0000+uint32(i%3)*2, 0x1234, uint64(i)*10)
	}

	if !halted {
		t.Error("a tight loop was not detected")
	}
}

func TestTheHaltDetectorIgnoresRunningCode(t *testing.T) {
	h := newHaltDetector(0x40, 1000)

	// Code walking forward leaves the window every few instructions
	for i := 0; i < 500; i++ {
		if h.inspect(0x40_0000+uint32(i)*2, 0x1234, uint64(i)*10) {
			t.Fatal("running code was reported as halted")
		}
	}
}

// The ROM checksum and the memory tests are tight loops that run for far
// longer than any threshold, and they must not be taken for a halt
func TestTheHaltDetectorIgnoresALoopMakingProgress(t *testing.T) {
	h := newHaltDetector(0x40, 1000)

	for i := 0; i < 100000; i++ {
		// The same handful of addresses, but a pointer advancing
		if h.inspect(0x40_0000+uint32(i%4)*2, uint32(i), uint64(i)*10) {
			t.Fatal("a loop advancing a register was reported as halted")
		}
	}
}

func TestTheHaltDetectorRecovers(t *testing.T) {
	h := newHaltDetector(0x40, 1000)

	for i := 0; i < 500; i++ {
		h.inspect(0x40_0000, 0x1234, uint64(i)*10)
	}
	if !h.halted {
		t.Fatal("a loop on a single address was not detected")
	}

	// An interrupt taking the processor elsewhere clears it
	h.inspect(0x40_1000, 0x1234, 999999)
	if h.halted {
		t.Error("the detector stayed halted after the processor moved on")
	}
}

// The ROM waits for the tick counter in a loop that changes no register and
// covers two addresses, for as long as a frame at a time. Calling that a halt
// is what an instruction counted detector does, and it is wrong.
func TestTheHaltDetectorWaitsOutAWaitForTheTick(t *testing.T) {
	const cyclesPerFrame = cyclesPerLine * linesPerFrame
	h := newHaltDetector(haltWindow, haltCycles)

	// A whole frame of CMP and BEQ on two addresses, nothing changing
	cycles := uint64(0)
	for cycles < cyclesPerFrame {
		if h.inspect(0x40_07d4+uint32(cycles%2)*4, 0x1234, cycles) {
			t.Fatalf("a wait for the tick was called a halt after %v cycles", cycles)
		}
		cycles += 13
	}

	// The tick arrives, the loop exits and the registers move on
	if h.inspect(0x40_07da, 0x9999, cycles) {
		t.Error("the detector stayed halted once the wait ended")
	}
}

func TestALoopThatNeverEndsIsStillAHalt(t *testing.T) {
	h := newHaltDetector(haltWindow, haltCycles)

	halted := false
	for cycles := uint64(0); cycles < 20*cyclesPerLine*linesPerFrame; cycles += 10 {
		halted = h.inspect(0x40_0000, 0x1234, cycles)
	}

	if !halted {
		t.Error("a loop going nowhere for twenty frames was not called a halt")
	}
}

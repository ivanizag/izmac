package izmac

import (
	"testing"
	"time"

	"github.com/ivanizag/izmac/storage"
)

// newTestMac builds a machine running a ROM that does nothing but branch to
// itself, so that the timing can be measured without a real ROM
func newTestMac(t *testing.T) *Mac {
	t.Helper()

	data := make([]uint8, storage.RomSize)

	// The reset stack pointer, on the RAM as the overlay maps it
	data[0], data[1], data[2], data[3] = 0x00, 0x60, 0x04, 0x00
	// The reset program counter
	data[4], data[5], data[6], data[7] = 0x00, 0x00, 0x00, 0x08
	// BRA.S to itself, 10 cycles per iteration
	data[8], data[9] = 0x60, 0xfe

	config := NewConfiguration()
	config.RomFile = "<test>"

	return mustNewMac(t, config, storage.RomFromData(data), nil, nil)
}

func TestTheMachineRunsAtTheRightRate(t *testing.T) {
	m := newTestMac(t)
	m.RunFrames(10)

	if m.GetFrames() != 10 {
		t.Errorf("ran %v frames, wanted 10", m.GetFrames())
	}

	// A frame is 370 scan lines of 352 cycles each
	const cyclesPerFrame = cyclesPerLine * linesPerFrame
	wanted := uint64(10 * cyclesPerFrame)
	got := m.GetCycles()

	// The last instruction can overshoot the frame boundary
	if got < wanted || got > wanted+20 {
		t.Errorf("ran %v cycles for 10 frames, wanted about %v", got, wanted)
	}
}

func TestTheFrameRateMatchesTheHardware(t *testing.T) {
	// 352 cycles a line and 370 lines a frame have to give the 60.15Hz of
	// the Macintosh Plus at its 7.8336MHz
	const cyclesPerFrame = cyclesPerLine * linesPerFrame
	frameRate := CPUClockMhz * 1_000_000 / cyclesPerFrame

	if frameRate < 60.1 || frameRate > 60.2 {
		t.Errorf("the constants give a frame rate of %.2fHz, wanted 60.15Hz", frameRate)
	}

	// And one sound sample per scan line has to give the 22254Hz sample rate
	sampleRate := CPUClockMhz * 1_000_000 / cyclesPerLine
	if sampleRate < 22250 || sampleRate > 22260 {
		t.Errorf("the constants give a sample rate of %.0fHz, wanted 22254Hz", sampleRate)
	}
}

func TestResetTakesTheVectorsFromTheRom(t *testing.T) {
	m := newTestMac(t)
	m.reset()

	if m.cpu.GetPC() != 0x08 {
		t.Errorf("the program counter after the reset is $%06x, wanted $000008", m.cpu.GetPC())
	}
}

func TestTheOverlayIsSetOnReset(t *testing.T) {
	m := newTestMac(t)
	m.mm.setOverlay(false)
	m.reset()

	if !m.mm.overlay {
		t.Error("the reset did not set the overlay back")
	}
}

/*
The run loop paced against the clock, which is what the throttling is for and
what the cycles per spin decides the granularity of. A spin that got through
a wildly variable amount of emulated time would make the sleep it works out
as variable, and the machine would run fast or slow depending on what code it
happened to be executing.

The tolerance is wide because a loaded host will not keep to it exactly. What
it catches is the gross failures: no throttling at all, or an order of
magnitude out.
*/
func TestTheMachineKeepsToItsClock(t *testing.T) {
	if testing.Short() {
		t.Skip("this one watches the clock for a moment")
	}

	m := newTestMac(t)
	go m.Run()
	defer m.SendCommand(CommandKill)

	time.Sleep(500 * time.Millisecond)

	reached := m.GetCurrentFreqMHz()
	if reached < CPUClockMhz/2 || reached > CPUClockMhz*2 {
		t.Errorf("the machine is running at %.2f MHz, wanted about %.2f",
			reached, CPUClockMhz)
	}
}

/*
The pacing a click asks for. A double click is two clicks close enough
together in ticks of the machine, and the ticks come from the emulated cycles,
so a machine running free counts hundreds of them between two clicks that were
a moment apart on the host and never sees the pair.
*/
func TestAClickHoldsTheMachineToItsClock(t *testing.T) {
	m := newTestMac(t)
	m.setCycleDuration(0)

	if m.pacedCycleDuration() != 0 {
		t.Error("the machine is not running free before the click")
	}

	m.SetMouseButton(true)

	if got := m.pacedCycleDuration(); got != cycleDurationOf(CPUClockMhz) {
		t.Errorf("a cycle takes %vns after the click, wanted the %vns of the real clock",
			got, cycleDurationOf(CPUClockMhz))
	}
}

// Both edges hold, because the gap that has to be measured is the one from
// the release of the first click to the press of the second
func TestReleasingTheButtonHoldsAsWell(t *testing.T) {
	m := newTestMac(t)
	m.setCycleDuration(0)

	m.SetMouseButton(true)

	// Out of the way, so that what is left is what the release asked for
	m.realTimeUntilNs.Store(0)
	m.SetMouseButton(false)

	if m.realTimeUntilNs.Load() == 0 {
		t.Error("the release did not hold")
	}
	if got := m.pacedCycleDuration(); got != cycleDurationOf(CPUClockMhz) {
		t.Errorf("a cycle takes %vns after the release, wanted the %vns of the real clock",
			got, cycleDurationOf(CPUClockMhz))
	}
}

// The button is reported every frame by a frontend and not only when it
// moves, so a state that is already the one held has nothing to hold for
func TestTheButtonStandingStillDoesNotHold(t *testing.T) {
	m := newTestMac(t)
	m.setCycleDuration(0)

	m.SetMouseButton(false)

	if m.realTimeUntilNs.Load() != 0 {
		t.Error("a button that did not change asked for a hold")
	}
	if m.pacedCycleDuration() != 0 {
		t.Error("the machine is not running free with the button untouched")
	}
}

// Once the hold has run out the machine goes back to what it was told to run
// at, and the deadline is dropped so that the loop has nothing left to check
func TestTheHoldRunsOutAndIsForgotten(t *testing.T) {
	m := newTestMac(t)
	m.setCycleDuration(0)

	m.SetMouseButton(true)
	m.realTimeUntilNs.Store(time.Now().Add(-time.Millisecond).UnixNano())

	if got := m.pacedCycleDuration(); got != 0 {
		t.Errorf("a cycle takes %vns after the hold ran out, wanted the machine running free", got)
	}
	if m.realTimeUntilNs.Load() != 0 {
		t.Error("the hold that ran out was left behind")
	}
}

// A machine put below its own clock to watch something happen is left there,
// the hold only ever slows one down
func TestTheHoldDoesNotSpeedUpASlowMachine(t *testing.T) {
	m := newTestMac(t)
	slow := cycleDurationOf(1.0)
	m.setCycleDuration(slow)

	m.SetMouseButton(true)

	if got := m.pacedCycleDuration(); got != slow {
		t.Errorf("a cycle takes %vns after the click, wanted the %vns it was configured for",
			got, slow)
	}
}

/*
And the run loop honouring the hold, which is the part of it the frequency it
reports can be watched for. A machine running free comes into the throttled
branch with a reference time from long before, so this is also the check that
it resynchronizes there rather than sitting out the hold in one long sleep.
*/
func TestAClickSlowsTheRunningMachineDown(t *testing.T) {
	if testing.Short() {
		t.Skip("this one watches the clock for a moment")
	}

	m := newTestMac(t)
	m.setCycleDuration(0)
	go m.Run()
	defer m.SendCommand(CommandKill)

	time.Sleep(200 * time.Millisecond)

	free := m.GetCurrentFreqMHz()
	if free < CPUClockMhz*2 {
		t.Skipf("the host only reaches %.2f MHz running free, there is nothing to slow down", free)
	}

	m.SetMouseButton(true)

	// Long enough for the speed to be measured again, which is once every
	// million cycles and so about every eighth of a second once it is held
	// down, and well short of the second the hold lasts
	time.Sleep(400 * time.Millisecond)

	if held := m.GetCurrentFreqMHz(); held > CPUClockMhz*2 {
		t.Errorf("the machine is running at %.2f MHz after a click, wanted about %.2f",
			held, CPUClockMhz)
	}
}

// A spin is one scan line of emulated time, so the loop looks at the clock
// and at the command channel that often
func TestASpinIsAScanLine(t *testing.T) {
	if cyclesPerSpin != cyclesPerLine {
		t.Errorf("a spin is %v cycles, wanted the %v of a scan line",
			cyclesPerSpin, cyclesPerLine)
	}
}

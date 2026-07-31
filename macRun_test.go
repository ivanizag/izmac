package izmac

import (
	"os"
	"path/filepath"
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

	return ensureNewMac(t, config, storage.RomFromData(data), nil, nil)
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

// A spin is one scan line of emulated time, so the loop looks at the clock
// and at the command channel that often
func TestASpinIsAScanLine(t *testing.T) {
	if cyclesPerSpin != cyclesPerLine {
		t.Errorf("a spin is %v cycles, wanted the %v of a scan line",
			cyclesPerSpin, cyclesPerLine)
	}
}

/*
Killing the machine has to put a changed diskette back on the host before the
run loop ends. It is the last chance: a diskette is written back when its
motor stops, and the driver leaves the motor running for a couple of seconds
after it has finished, so quitting just after saving a file would lose it.

The image is made to differ from the file it came from by writing over the
file behind the diskette's back. Nothing else has changed, so the file coming
back to what it was is the flush having happened.
*/
func TestKillingTheMachineWritesADisketteBack(t *testing.T) {
	wanted := make([]uint8, 800*1024)
	for i := range wanted {
		wanted[i] = uint8(i)
	}

	filename := filepath.Join(t.TempDir(), "work.dsk")
	if err := os.WriteFile(filename, wanted, 0666); err != nil {
		t.Fatal(err)
	}

	m := newTestMac(t)
	if err := m.InsertDiskette(DriveInternal, filename); err != nil {
		t.Fatal(err)
	}

	// The diskette holds the image now, so the file can be emptied under it
	if err := os.WriteFile(filename, make([]uint8, 800*1024), 0666); err != nil {
		t.Fatal(err)
	}

	// A track read and written straight back leaves the diskette changed
	// as far as it knows, which is what a flush acts on
	disk := m.iwm.drives[DriveInternal].disk
	nibbles, err := disk.ReadTrack(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := disk.WriteTrack(0, 0, nibbles); err != nil {
		t.Fatal(err)
	}

	go m.Run()
	m.SendCommand(CommandKill)

	if !m.WaitUntilStopped(5 * time.Second) {
		t.Fatal("the machine did not stop when it was killed")
	}

	got, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	for i := range wanted {
		if got[i] != wanted[i] {
			t.Fatalf("the diskette was not written back, the image differs at %v", i)
		}
	}
}

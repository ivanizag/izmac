package izmac

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

/*
The whole machine, booting a real System from a real disk image to the
Finder. It needs a ROM and a disk image that are not part of the repository,
so it skips when either is missing. With them it costs about a second: the
emulation is only slow when a tracer is on, and the halt detector in
particular fingerprints all sixteen registers on every instruction.

What it checks is the menu bar. The desktop is a fifty per cent stipple, so
the top rows of the screen are about half black while the machine is showing
the question mark floppy, and turn nearly white with a thin line of text once
the Finder draws its menu bar. That tells the two apart without depending on
any particular System version or set of icons.
*/
func TestBootsToTheFinder(t *testing.T) {
	m := bootedMac(t)

	if black := blackRatio(m, 2, 16); black > 0.2 {
		t.Errorf("the top of the screen is %.0f%% black after %v frames, "+
			"so there is no menu bar and the Finder did not start",
			black*100, bootFrames)
	}

	// And the desktop below it is still the stipple, which rules out a
	// screen that simply went blank
	if black := blackRatio(m, 120, 200); black < 0.2 {
		t.Errorf("the desktop is only %.0f%% black, the screen looks empty", black*100)
	}
}

// blackRatio returns the share of black pixels on a band of scan lines
func blackRatio(m *Mac, from int, to int) float64 {
	buffer := m.video.frameBuffer()

	set := 0
	for y := from; y < to; y++ {
		for x := 0; x < bytesPerLine; x++ {
			bits := buffer[y*bytesPerLine+x]
			for bit := 0; bit < 8; bit++ {
				if bits&(0x80>>bit) != 0 {
					set++
				}
			}
		}
	}

	return float64(set) / float64((to-from)*width)
}

/*
The mouse, end to end on the booted machine. The ROM counts the quadrature
transitions in its interrupt handlers and keeps the pointer in low memory at
RawMouse, $082c for the vertical and $082e for the horizontal, so moving the
host mouse has to move those.

This is the check that the quadrature is being generated the right way round.
Getting the phase backwards still moves the pointer, just the wrong way, and
nothing short of watching where it goes catches that.
*/
func TestTheMouseMovesThePointer(t *testing.T) {
	m := bootedMac(t)

	readPoint := func() (int16, int16) {
		h, v := pointerAt(m)
		return v, h
	}

	startV, startH := readPoint()

	// Right and down, far enough that a stray transition cannot account
	// for it but not so far that the pointer hits the edge of the screen
	m.MoveMouse(60, 40)
	m.RunFrames(30)

	v, h := readPoint()
	if h <= startH {
		t.Errorf("moving right took the pointer from %v to %v", startH, h)
	}
	if v <= startV {
		t.Errorf("moving down took the pointer from %v to %v", startV, v)
	}

	// And it has to keep up. The ROM scales the movement, so this is not
	// an exact count, but a pointer that crawls a few pixels when it was
	// pushed sixty is the symptom of losing transitions.
	if h-startH < 20 {
		t.Errorf("the pointer moved %v across for a push of 60, it is not keeping up",
			h-startH)
	}

	// And back the other way
	m.MoveMouse(-60, -40)
	m.RunFrames(30)

	backV, backH := readPoint()
	if backH >= h {
		t.Errorf("moving left took the pointer from %v to %v", h, backH)
	}
	if backV >= v {
		t.Errorf("moving up took the pointer from %v to %v", v, backV)
	}
}

/*
realConfig points a configuration at the real ROM and disk image, which are
not part of the repository, and skips when either is missing. A test that
wants to change a setting starts here and builds the machine itself.
*/
func realConfig(t *testing.T) *Configuration {
	t.Helper()

	const (
		diskFile = "frontend/macebiten/HD20SC.vhd"
		romFile  = defaultRomFile
	)

	for _, name := range []string{diskFile, romFile} {
		if _, err := os.Stat(name); err != nil {
			t.Skipf("%v is not here, this test needs it", name)
		}
	}

	config := NewConfiguration()
	config.RomFile = romFile
	config.HardDisks = []string{diskFile}
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	return config
}

// realMac is that machine at its reset, not yet run, for a test that wants to
// watch it come up
func realMac(t *testing.T) *Mac {
	t.Helper()

	m, err := NewMac(realConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// bootFrames is how long the machine takes to reach the Finder, about forty
// emulated seconds and about a second of real time
const bootFrames = 2400

// bootedMac runs a machine up to the Finder
func bootedMac(t *testing.T) *Mac {
	t.Helper()

	m := realMac(t)
	m.RunFrames(bootFrames)
	return m
}

/*
The keyboard, end to end on the booted machine. The ROM keeps a bitmap of
which keys are down in KeyMap, eight bytes from $0174, so pressing a key has
to set a bit there and releasing it has to clear it again.

Whether a key ends up in the right bit is what this checks, and that is worth
checking: the driver strips the release bit from the byte the keyboard sends
and shifts the rest one place right, so an off by one in the table would
still register a key press, just the wrong key.
*/
func TestAKeyPressReachesTheKeyMap(t *testing.T) {
	const keyMap = 0x0174

	m := bootedMac(t)

	readKeyMap := func() [8]uint8 {
		var bits [8]uint8
		for i := range bits {
			bits[i] = m.mm.Peek(uint32(keyMap + i))
		}
		return bits
	}

	// The A key, which the driver reads as the key code 0
	code := KeyCodes()["A"]
	const wantByte, wantBit = 0, 0

	before := readKeyMap()
	if before[wantByte]&(1<<wantBit) != 0 {
		t.Fatal("the A key is already down before anything was pressed")
	}

	m.PutKey(code, true)
	m.RunFrames(20)

	down := readKeyMap()
	if down[wantByte]&(1<<wantBit) == 0 {
		t.Errorf("pressing A left the key map at %x, with nothing set for it", down)
	}

	m.PutKey(code, false)
	m.RunFrames(20)

	up := readKeyMap()
	if up[wantByte]&(1<<wantBit) != 0 {
		t.Errorf("releasing A left the key map at %x, still holding it down", up)
	}
}

/*
Small repeated movements, which is how a mouse is actually used and what a
single long sweep does not exercise. Over a long push the sampling errors
average out and the direction still reads correctly, so a sweep test passes
on a mouse that is unusable in the hand; over the two or three edges of a
short push there is nothing to average them away.

The distances are not checked closely because the ROM scales the movement by
how fast it is going, only that each push moves the pointer the way it was
pushed and by a distance of the right order.
*/
func TestSmallMouseMovementsTrack(t *testing.T) {
	const (
		pushes = 10
		pixels = 3
	)

	m := bootedMac(t)

	readPoint := func() (int16, int16) {
		h, v := pointerAt(m)
		return v, h
	}

	for _, c := range []struct {
		name   string
		dx, dy int
	}{
		{"right", pixels, 0},
		{"left", -pixels, 0},
		{"down", 0, pixels},
		{"up", 0, -pixels},
	} {
		startV, startH := readPoint()

		for i := 0; i < pushes; i++ {
			m.MoveMouse(c.dx, c.dy)
			m.RunFrames(4)
		}

		v, h := readPoint()
		movedH, movedV := int(h-startH), int(v-startV)

		wantedH, wantedV := c.dx*pushes, c.dy*pushes
		for _, axis := range []struct {
			name   string
			moved  int
			wanted int
		}{
			{"across", movedH, wantedH},
			{"down", movedV, wantedV},
		} {
			if wanted := axis.wanted; wanted == 0 {
				if axis.moved != 0 {
					t.Errorf("pushing %v moved the pointer %v %v, it should not have",
						c.name, axis.moved, axis.name)
				}
				continue
			}

			if axis.moved == 0 || (axis.moved > 0) != (axis.wanted > 0) {
				t.Errorf("pushing %v %v moved the pointer %v %v, the wrong way",
					wantedAbs(axis.wanted), c.name, axis.moved, axis.name)
				continue
			}
			if wantedAbs(axis.moved)*4 < wantedAbs(axis.wanted) {
				t.Errorf("pushing %v %v moved the pointer only %v %v",
					wantedAbs(axis.wanted), c.name, axis.moved, axis.name)
			}
		}
	}
}

func wantedAbs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

/*
Two disks on the bus, which is the point of taking more than one: the ROM has
to find both, load a driver for each and put both in the drive queue, and the
Finder then shows a volume for each.

The queue is walked rather than the screen looked at, because counting icons
would depend on the System and on where it decided to put them.
*/
func TestTwoDisksBothMount(t *testing.T) {
	const (
		drvQHead = 0x030a
		qLink    = 0
	)

	first := "frontend/macebiten/HD20SC.vhd"
	if _, err := os.Stat(first); err != nil {
		t.Skipf("%v is not here, this test needs it", first)
	}
	if _, err := os.Stat(defaultRomFile); err != nil {
		t.Skipf("%v is not here, this test needs it", defaultRomFile)
	}

	second := copyFile(t, first)

	config := NewConfiguration()
	config.RomFile = defaultRomFile
	config.HardDisks = []string{first, second}
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}

	m, err := NewMac(config)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.GetDisks()) != 2 {
		t.Fatalf("%v disks reached the bus, wanted 2", len(m.GetDisks()))
	}

	m.RunFrames(bootFrames)

	// Walk the drive queue. One entry is the floppy the IWM reports, the
	// other two are the disks.
	readLong := func(address uint32) uint32 {
		return uint32(m.mm.Peek(address))<<24 | uint32(m.mm.Peek(address+1))<<16 |
			uint32(m.mm.Peek(address+2))<<8 | uint32(m.mm.Peek(address+3))
	}

	drives := 0
	for element := readLong(drvQHead); element != 0 && drives < 16; drives++ {
		element = readLong(element + qLink)
	}

	if drives < 3 {
		t.Errorf("the drive queue holds %v drives, wanted the diskette and both disks",
			drives)
	}
}

func copyFile(t *testing.T, from string) string {
	t.Helper()

	source, err := os.Open(from)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()

	to := filepath.Join(t.TempDir(), "second.img")
	target, err := os.Create(to)
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()

	if _, err := io.Copy(target, source); err != nil {
		t.Fatal(err)
	}
	return to
}

/*
The startup sound, all the way from the ROM to a sink. The Macintosh plays a
tone as it comes up, so booting has to produce something audible: samples
away from the middle, not just a stream of silence.

This is the check that the buffer, the volume and the enable are all wired to
the right bits of the VIA. Any one of them wrong gives silence, which is what
no sound at all sounds like.
*/
func TestTheMachineMakesASoundAsItStarts(t *testing.T) {
	m := realMac(t)

	sink := &countingSink{}
	m.SetAudioSink(sink)
	m.Reset()
	m.RunFrames(240)

	if sink.samples == 0 {
		t.Fatal("no samples at all reached the sink")
	}

	// One for every scan line of every frame
	if wanted := 240 * soundSamplesPerFrame; sink.samples < wanted {
		t.Errorf("%v samples in 240 frames, wanted %v", sink.samples, wanted)
	}

	if sink.loudest == 0 {
		t.Error("the machine started in silence, nothing was played")
	}
	if sink.loudest > 1 {
		t.Errorf("the loudest sample is %v, past what a speaker takes", sink.loudest)
	}
}

// countingSink counts what it is given and remembers the loudest
type countingSink struct {
	samples int
	loudest float32
}

func (c *countingSink) PushSample(sample float32) {
	c.samples++
	if sample > c.loudest {
		c.loudest = sample
	} else if -sample > c.loudest {
		c.loudest = -sample
	}
}

/*
The clock read by the machine rather than by a test of the chip. The ROM
copies the counter into the Time global as it comes up, so a booted machine
has to know roughly what the host thinks the time is.

The tolerance is generous in one direction on purpose: the machine keeps its
own seconds and RunFrames does not throttle, so booting for 2400 frames
advances the clock forty emulated seconds in about a second of real time.
What matters is that the year is right and not 1904, which is what a clock
the ROM failed to read looks like.
*/
func TestTheMachineKnowsTheTime(t *testing.T) {
	const timeGlobal = 0x020c

	m := bootedMac(t)

	stored := uint32(m.mm.Peek(timeGlobal))<<24 | uint32(m.mm.Peek(timeGlobal+1))<<16 |
		uint32(m.mm.Peek(timeGlobal+2))<<8 | uint32(m.mm.Peek(timeGlobal+3))

	epoch := time.Date(1904, 1, 1, 0, 0, 0, 0, time.Local)
	machine := epoch.Add(time.Duration(stored) * time.Second)
	now := time.Now()

	if machine.Before(now.Add(-time.Minute)) || machine.After(now.Add(10*time.Minute)) {
		t.Errorf("the machine says it is %v, the host says %v", machine, now)
	}
}

/*
The same, on a clock that answers with the time of the host on every read.
This is the one worth running end to end rather than against the chip alone:
the ROM reads the counter twice and compares the halves to catch a tick
landing in the middle of the read, and a clock that ticks on its own while it
is being read is exactly what that guards against. Failing it twice leaves the
machine at the epoch, so a machine that knows the year proves the read went
through.

The window is tight in both directions here, which is the point of the
option: forty emulated seconds of booting move this clock not at all.
*/
func TestTheWallClockReachesTheMachine(t *testing.T) {
	const timeGlobal = 0x020c

	config := realConfig(t)
	config.WallClock = true

	m, err := NewMac(config)
	if err != nil {
		t.Fatal(err)
	}
	m.RunFrames(bootFrames)

	stored := uint32(m.mm.Peek(timeGlobal))<<24 | uint32(m.mm.Peek(timeGlobal+1))<<16 |
		uint32(m.mm.Peek(timeGlobal+2))<<8 | uint32(m.mm.Peek(timeGlobal+3))

	epoch := time.Date(1904, 1, 1, 0, 0, 0, 0, time.Local)
	machine := epoch.Add(time.Duration(stored) * time.Second)
	now := time.Now()

	if machine.Before(now.Add(-time.Minute)) || machine.After(now.Add(time.Minute)) {
		t.Errorf("the machine says it is %v, the host says %v", machine, now)
	}
}

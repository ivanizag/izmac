package izmac

import (
	"fmt"
	"math"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ivanizag/iz68000"
	"github.com/ivanizag/izmac/component"
	"github.com/ivanizag/izmac/scsi"
	"github.com/ivanizag/izmac/storage"
)

// Mac represents all the components and state of the emulated machine
type Mac struct {
	Name string

	config   *Configuration
	cpu      *iz68000.State
	mm       *memoryManager
	rom      *storage.Rom
	video    *video
	via      *via
	rtc      *component.AppleRTC
	keyboard *keyboard
	mouse    *mouse
	scc      *component.SCC8530
	sound    *sound
	scsi     *scsi.Bus
	iwm      *iwm

	// clipboard is nil when the machine keeps its clipboard to itself
	clipboard *clipboard

	commandChannel chan command

	cycles uint64

	// lineCycles counts towards the next scan line, the tick the whole
	// machine runs on
	lineCycles   uint64
	secondCycles uint64
	line         int
	frames       uint64

	/*
		cycleDurationNs is how long a cycle lasts, or zero to run as fast
		as the host can. It is kept as the bits of a float so that it can
		be changed by the emulation goroutine, which the speed toggle
		does, while a frontend reads it to put the speed on the window.
	*/
	cycleDurationNs atomic.Uint64

	// currentFreqMHz is what the emulation is reaching, written by the run
	// loop and read by a frontend putting it on the window, so it is kept
	// where both can reach it safely
	currentFreqMHz atomic.Uint64

	/*
		realTimeUntilNs is the wall clock, as Unix nanoseconds, that a click
		is holding the machine down to its own speed until, or zero when
		nothing is. A frontend sets it from its own goroutine and the run
		loop clears it once it has run out, which leaves the loop with a
		single atomic to read while the machine is running free.
	*/
	realTimeUntilNs atomic.Int64

	paused  atomic.Bool
	started bool

	// pastePending guards the clipboard hook in the instruction loop, which
	// has nothing to do until there is a paste waiting to be delivered
	pastePending bool

	cpuTrace     bool
	toolboxTrace bool
	sadMacTrace  bool
	halt         *haltDetector
}

/*
scsiFirstDiskId is where the first disk sits on the bus and the rest follow
it upwards. The Macintosh keeps the id 7 for itself and 0 is the usual place
for the internal disk, so the disks given on the command line take 0, 1, 2
and so on up to 6.
*/
const scsiFirstDiskId = 0

const (
	// haltWindow is the span of addresses the loop the ROM ends on covers,
	// and haltCycles how long it has to run with nothing changing before
	// it is called a halt. Ten frames is far longer than any wait for an
	// interrupt and no time at all next to a loop that never ends.
	haltWindow = 0x40
	haltCycles = 10 * cyclesPerLine * linesPerFrame
)

// NewMac builds an emulated Macintosh Plus from a configuration. The default
// ROM is downloaded if it is the one wanted and it is not on the working
// directory, and so is a diskette to boot if nothing was named to boot from.
func NewMac(config *Configuration) (*Mac, error) {
	err := ensureRom(config, os.Stdout)
	if err != nil {
		return nil, err
	}

	err = config.ensureStartupDiskette(os.Stdout)
	if err != nil {
		return nil, err
	}

	r, err := storage.LoadRom(config.RomFile)
	if err != nil {
		return nil, err
	}

	// The SCSI driver is found once and given to every bare volume on the
	// bus. A machine with nothing but proper disks on it gets none and
	// wants none.
	scsiDriver, err := config.ensureScsiDriver(os.Stdout)
	if err != nil {
		return nil, err
	}

	disks := make([]storage.BlockDisk, 0, len(config.DiskFiles))
	for _, filename := range config.DiskFiles {
		disk, err := storage.NewBlockDisk(filename, scsiDriver, false)
		if err != nil {
			return nil, err
		}
		disks = append(disks, disk)
	}

	// The diskettes go in the drives in the order they were named, the
	// internal one first
	diskettes := make([]*storage.FloppyDisk, 0, len(config.Diskettes))
	for _, filename := range config.Diskettes {
		diskette, err := storage.NewFloppyDisk(filename, false)
		if err != nil {
			return nil, err
		}
		diskettes = append(diskettes, diskette)
	}

	return newMac(config, r, disks, diskettes)
}

// newMac assembles the machine around an already loaded ROM. The tests use
// it to run code that no real ROM would contain.
func newMac(config *Configuration, r *storage.Rom, disks []storage.BlockDisk,
	diskettes []*storage.FloppyDisk) (*Mac, error) {
	mm := newMemoryManager(config.RamSizeKb, r.Data(), config.hasTracer("floppy"))

	v := newVideo(mm)
	c := component.NewAppleRTC(config.PramFile, config.WallClock)
	k := newKeyboard()
	mo := newMouse()
	so := newSound(mm)

	for i, d := range disks {
		mm.scsi.Attach(scsi.NewDisk(uint8(scsiFirstDiskId+i), d, config.hasTracer("scsi")))
	}

	m := &Mac{
		Name:           "Macintosh Plus",
		config:         config,
		rom:            r,
		mm:             mm,
		video:          v,
		rtc:            c,
		keyboard:       k,
		mouse:          mo,
		sound:          so,
		scsi:           mm.scsi,
		scc:            mm.scc,
		iwm:            mm.iwm,
		via:            newVia(mm, v, mm.iwm, c, k, mo, so),
		commandChannel: make(chan command, commandChannelSize),
		cpuTrace:       config.hasTracer("cpu"),
		toolboxTrace:   config.hasTracer("toolbox"),
		sadMacTrace:    config.hasTracer("sadmac"),
		halt:           newHaltDetector(haltWindow, haltCycles),
	}

	m.setCycleDuration(config.cycleDurationNs)
	mm.via = m.via

	if config.Clipboard {
		m.clipboard = newClipboard()
	}

	m.cpu = iz68000.NewM68000(mm)
	m.cpu.SetTrace(m.cpuTrace)

	for i, diskette := range diskettes {
		if i >= driveCount {
			return nil, fmt.Errorf("the machine has %v diskette drives, %v were given",
				driveCount, len(diskettes))
		}
		if err := mm.iwm.drives[i].insert(diskette); err != nil {
			return nil, err
		}
	}

	return m, nil
}

// DiskDescription names an attached disk for a frontend to report
type DiskDescription struct {
	Id     int
	Name   string
	Blocks uint32
}

// GetDisks describes the disks on the bus
func (m *Mac) GetDisks() []DiskDescription {
	attached := m.scsi.Attached()

	described := make([]DiskDescription, 0, len(attached))
	for _, a := range attached {
		described = append(described, DiskDescription{
			Id:     a.Id,
			Name:   a.Name,
			Blocks: a.Blocks,
		})
	}
	return described
}

// DisketteDescription is what is in a diskette drive, for a frontend to
// report and to build its menu from
type DisketteDescription struct {
	// Drive is DriveInternal or DriveExternal
	Drive int
	// Name is how the drive is known, "internal" or "external"
	Name string
	// Image is the diskette in it, empty when the drive is
	Image string
	// ReadOnly tells whether the diskette is locked
	ReadOnly bool
}

// The diskette drives of the machine, the one inside and the one on the port
// at the back
const (
	DriveInternal = driveInternal
	DriveExternal = driveExternal

	// DriveCount is how many of them there are
	DriveCount = driveCount
)

/*
GetDiskettes describes both drives, empty or not. It is safe to call from a
frontend while the machine runs: what it reports is published by the
emulation as it changes rather than being read out from under it.
*/
func (m *Mac) GetDiskettes() []DisketteDescription {
	described := make([]DisketteDescription, 0, driveCount)

	for drive := range m.iwm.drives {
		described = append(described, m.GetDiskette(drive))
	}

	return described
}

// GetDiskette describes one drive, which is what a menu line asks about as it
// is drawn. It is safe to call while the machine runs, as GetDiskettes is.
func (m *Mac) GetDiskette(drive int) DisketteDescription {
	if drive < 0 || drive >= driveCount {
		return DisketteDescription{Drive: drive}
	}

	d := m.iwm.drives[drive]
	image, readOnly := d.mounted()

	return DisketteDescription{
		Drive:    drive,
		Name:     d.name,
		Image:    image,
		ReadOnly: readOnly,
	}
}

/*
InsertDiskette puts an image in one of the drives. It is how the machine is
set up before it runs; once it is running a frontend goes through
SendInsertDiskette instead, so that the drive is not changed under the
emulation.
*/
func (m *Mac) InsertDiskette(drive int, filename string) error {
	if drive < 0 || drive >= driveCount {
		return fmt.Errorf("the machine has no diskette drive %v", drive)
	}

	disk, err := storage.NewFloppyDisk(filename, false)
	if err != nil {
		return err
	}

	return m.iwm.drives[drive].insert(disk)
}

// EjectDiskette takes the diskette out of a drive, writing back anything the
// machine had left on it
func (m *Mac) EjectDiskette(drive int) error {
	if drive < 0 || drive >= driveCount {
		return fmt.Errorf("the machine has no diskette drive %v", drive)
	}

	return m.iwm.drives[drive].eject()
}

/*
FlushDiskettes writes back whatever the machine has changed and not yet been
asked to store, which happens when it is closed down with a disk still in a
drive. The emulation does it by itself as a motor stops, so this is the
unusual path rather than the usual one.
*/
func (m *Mac) FlushDiskettes() error {
	return m.iwm.flush()
}

// PutKey queues a key transition for the keyboard. The code is the raw one
// the hardware sends, from KeyCodes().
func (m *Mac) PutKey(code uint8, down bool) {
	m.keyboard.putKey(code, down)
}

// KeyCodes returns the raw transition codes of the United States keyboard,
// keyed by a name a frontend can map its own keys to
func KeyCodes() map[string]uint8 {
	return keyCodes()
}

// MoveMouse adds to the movement waiting to be reported, in pixels, positive
// to the right and down
func (m *Mac) MoveMouse(dx int, dy int) {
	m.mouse.move(dx, dy)
}

/*
SetMouseButton reports the state of the only button the machine has.

A click is also what puts the machine back on its own clock for a moment. The
ROM tells a double click from two single ones by the ticks between them, and a
tick comes from the vertical blanking, which is counted in emulated cycles:
running free, the fifth of a second a person leaves between two clicks is
hundreds of ticks to the machine and the second click lands long after the
window the first one opened. Slowing down around the click puts the two back
within the same distance of each other that they are on the host.
*/
func (m *Mac) SetMouseButton(pressed bool) {
	if m.mouse.setButton(pressed) {
		m.holdRealTime()
	}
}

// GetCycles returns the cycles run since the reset
func (m *Mac) GetCycles() uint64 {
	return m.cycles
}

// GetFrames returns the frames elapsed since the reset
func (m *Mac) GetFrames() uint64 {
	return m.frames
}

// GetCurrentFreqMHz returns the speed the emulation is running at
func (m *Mac) GetCurrentFreqMHz() float64 {
	return math.Float64frombits(m.currentFreqMHz.Load())
}

// GetPC returns the program counter, to report where a run ended
func (m *Mac) GetPC() uint32 {
	return m.cpu.GetPC()
}

// WatchWrites reports on the standard output every write to a range of the
// RAM, with the instruction that made it. It is how a low memory global that
// should have been filled and was not is tracked down.
func (m *Mac) WatchWrites(from uint32, to uint32) {
	m.mm.setWatch(from, to, func(address uint32, value uint8) {
		fmt.Printf("%09d  $%06x <- $%02x  from $%06x\n",
			m.cycles, address, value, m.cpu.GetPC())
	})
}

// Disasm returns a listing of the instructions from an address, the way a
// debugger would show them
func (m *Mac) Disasm(from uint32, instructions int) string {
	var sb strings.Builder

	pc := from
	for i := 0; i < instructions; i++ {
		line, next := m.cpu.DisasmInstruction(pc)
		sb.WriteString(line)
		sb.WriteByte('\n')
		pc = next
	}

	return sb.String()
}

// DumpRegisters returns the processor registers, to report where a run ended
func (m *Mac) DumpRegisters() string {
	var sb strings.Builder

	for i := 0; i < 8; i++ {
		fmt.Fprintf(&sb, "D%v %08x  A%v %08x\n",
			i, m.cpu.GetD(i), i, m.cpu.GetA(i))
	}
	fmt.Fprintf(&sb, "PC %08x  SR %04x\n", m.cpu.GetPC(), m.cpu.GetSR())

	return sb.String()
}

// setCycleDuration and cycleDuration keep the speed where both goroutines can
// reach it safely
func (m *Mac) setCycleDuration(ns float64) {
	m.cycleDurationNs.Store(math.Float64bits(ns))
}

func (m *Mac) cycleDuration() float64 {
	return math.Float64frombits(m.cycleDurationNs.Load())
}

/*
realTimeHold is how long a click keeps the machine on its own clock. What has
to fit inside it is the gap a person leaves between the two clicks of a double
one, measured against the DoubleTime low memory global: eight ticks, an
eighth of a second, as the machine comes out of the box and about half a
second at the slowest the control panel goes. A second leaves room for the
slowest setting and is short enough that the machine is running free again
before anyone waiting on it notices.
*/
const realTimeHold = time.Second

// holdRealTime asks the run loop to pace the machine at its own clock for a
// while, whatever speed it was told to run at
func (m *Mac) holdRealTime() {
	m.realTimeUntilNs.Store(time.Now().Add(realTimeHold).UnixNano())
}

/*
pacedCycleDuration is how long a cycle has to take right now: the duration
configured, or the one of the real clock while a click is holding the machine
back.

The hold only ever slows the machine down. One asked to run below its own
clock, to watch something happen, is left where it was put.
*/
func (m *Mac) pacedCycleDuration() float64 {
	ns := m.cycleDuration()

	until := m.realTimeUntilNs.Load()
	if until == 0 {
		return ns
	}

	if time.Now().UnixNano() >= until {
		/*
			The hold has run out. Compared before it is swapped so that a
			click arriving between the load and here is not dropped along
			with it: the swap fails, and the deadline that click wrote
			stands.
		*/
		m.realTimeUntilNs.CompareAndSwap(until, 0)
		return ns
	}

	if realNs := cycleDurationOf(CPUClockMhz); ns == 0 || ns < realNs {
		return realNs
	}
	return ns
}

// clockMhz returns the clock the emulation is throttled to, or the one of
// the real machine when it is running free
func (m *Mac) clockMhz() float64 {
	ns := m.cycleDuration()
	if ns == 0 {
		return CPUClockMhz
	}
	return 1000.0 / ns
}

// IsFullSpeed tells if the emulation runs as fast as the host can go
func (m *Mac) IsFullSpeed() bool {
	return m.cycleDuration() == 0
}

// IsPaused returns true when the emulation is stopped
func (m *Mac) IsPaused() bool {
	return m.paused.Load()
}

// IsProfiling returns true when the CPU profiler is requested
func (m *Mac) IsProfiling() bool {
	return m.config.Profile
}

/*
Summary describes the machine as it was configured, one line at a time: the
ROM it came up on, what reached the SCSI bus and what could not be used. The
frontends differ in where they put it and not in what it says, so the lines
are made here and printed there.
*/
func (m *Mac) Summary() []string {
	lines := []string{m.rom.String()}

	if !isPreferredRom(m.rom) {
		version := m.rom.Version()
		lines = append(lines, fmt.Sprintf(
			"Warning: %v is not the revision izmac targets, %v",
			version.Nickname, version.Notes))
	}

	for _, disk := range m.GetDisks() {
		lines = append(lines, fmt.Sprintf("SCSI %v: %v, %v blocks",
			disk.Id, disk.Name, disk.Blocks))
	}

	for _, diskette := range m.GetDiskettes() {
		if diskette.Image == "" {
			lines = append(lines, fmt.Sprintf("Floppy %v: empty", diskette.Name))
			continue
		}

		locked := ""
		if diskette.ReadOnly {
			locked = ", locked"
		}
		lines = append(lines, fmt.Sprintf("Floppy %v: %v%v",
			diskette.Name, diskette.Image, locked))
	}

	return lines
}

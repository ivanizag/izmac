package izmac

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ivanizag/izmac/storage"
)

// ensureNewMac assembles a machine for a test, failing it rather than making
// every caller deal with a configuration it wrote itself
func ensureNewMac(t *testing.T, config *Configuration, r *storage.Rom,
	disks []storage.BlockDisk, diskettes []*storage.FloppyDisk) *Mac {
	t.Helper()

	m, err := newMac(config, r, disks, diskettes)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// A file named on the command line that turns out to be a diskette goes in a
// drive, the internal one first, and not on the SCSI bus
func TestADisketteGoesInTheInternalDrive(t *testing.T) {
	// A startup diskette, which goes in at once: one without boot blocks
	// waits for the machine to have started
	data := make([]uint8, 400*1024)
	data[0], data[1] = 'L', 'K'
	floppy := filepath.Join(t.TempDir(), "floppy.img")
	if err := os.WriteFile(floppy, data, 0o600); err != nil {
		t.Fatal(err)
	}

	config := NewConfiguration()
	config.RomFile = "<test>"
	if err := config.AddFiles([]string{floppy}); err != nil {
		t.Fatal(err)
	}
	if len(config.Diskettes) != 1 {
		t.Fatalf("%v was not taken for a diskette", floppy)
	}

	diskette, err := storage.NewFloppyDisk(floppy, false)
	if err != nil {
		t.Fatal(err)
	}

	m := ensureNewMac(t, config, storage.RomFromData(make([]uint8, storage.RomSize)),
		nil, []*storage.FloppyDisk{diskette})

	drives := m.GetDiskettes()
	if len(drives) != DriveCount {
		t.Fatalf("the machine reports %v drives, wanted %v", len(drives), DriveCount)
	}
	if drives[DriveInternal].Image != floppy {
		t.Errorf("the internal drive holds %q, wanted %q",
			drives[DriveInternal].Image, floppy)
	}
	if drives[DriveExternal].Image != "" {
		t.Errorf("the external drive holds %q, wanted nothing",
			drives[DriveExternal].Image)
	}

	named := 0
	for _, line := range m.Summary() {
		if strings.Contains(line, floppy) {
			named++
		}
	}
	if named != 1 {
		t.Errorf("the diskette was named on %v lines of the summary, wanted one", named)
	}

	if len(m.GetDisks()) != 0 {
		t.Error("the diskette was put on the SCSI bus")
	}
}

func TestTheDisksTakeTheIdsInOrder(t *testing.T) {
	config := NewConfiguration()
	config.RomFile = "<test>"

	disks := []storage.BlockDisk{
		storage.NewBlockDiskMemory(16),
		storage.NewBlockDiskMemory(32),
		storage.NewBlockDiskMemory(64),
	}
	m := ensureNewMac(t, config, storage.RomFromData(make([]uint8, storage.RomSize)), disks, nil)

	described := m.GetDisks()
	if len(described) != len(disks) {
		t.Fatalf("%v disks reached the bus, wanted %v", len(described), len(disks))
	}

	for i, d := range described {
		if d.Id != scsiFirstDiskId+i {
			t.Errorf("the disk %v took the id %v, wanted %v", i, d.Id, scsiFirstDiskId+i)
		}
		if d.Blocks != disks[i].Blocks() {
			t.Errorf("the disk at the id %v has %v blocks, wanted %v",
				d.Id, d.Blocks, disks[i].Blocks())
		}
	}
}

/*
Stopping the machine writes back what it left on a diskette whose motor had not
stopped yet. That is the window being closed with the drive still turning, and
the writes of the last few seconds would otherwise not reach the file.
*/
func TestStoppingTheMachineWritesTheDiskettesBack(t *testing.T) {
	floppy := writeImage(t, "floppy.dsk", 800*1024, false)

	config, _ := quietConfiguration()
	config.RomFile = "<test>"
	m := ensureNewMac(t, config, storage.RomFromData(make([]uint8, storage.RomSize)), nil, nil)
	if err := m.InsertDiskette(DriveInternal, floppy); err != nil {
		t.Fatal(err)
	}

	// The machine writes the first track, the same but for one byte, which
	// is taken from a diskette that has that byte changed
	changed := make([]uint8, 800*1024)
	changed[0] = 'W'
	source, err := storage.NewFloppyDiskData("changed", changed)
	if err != nil {
		t.Fatal(err)
	}
	track, err := source.ReadTrack(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	disk := m.iwm.drives[DriveInternal].disk
	if stored, err := disk.WriteTrack(0, 0, track); err != nil || stored == 0 {
		t.Fatalf("writing a track stored %v sectors: %v", stored, err)
	}

	stopped := make(chan struct{})
	go func() {
		m.Run()
		close(stopped)
	}()
	m.SendCommand(CommandKill)
	<-stopped

	data, err := os.ReadFile(floppy)
	if err != nil {
		t.Fatal(err)
	}
	if data[0] != 'W' {
		t.Errorf("the write to the diskette did not reach the file")
	}
}

/*
resetOnceRom is a ROM that counts its starts in RAM and executes RESET on the
first one, then branches to itself. The count is what says the RESET started
the machine again.
*/
func resetOnceRom() *storage.Rom {
	data := make([]uint8, storage.RomSize)
	copy(data, []uint8{
		0x00, 0x60, 0x04, 0x00, // the reset stack pointer
		0x00, 0x00, 0x00, 0x08, // the reset program counter
		0x52, 0x79, 0x00, 0x60, 0x01, 0x00, // ADDQ.W #1,$600100
		0x0c, 0x79, 0x00, 0x01, 0x00, 0x60, 0x01, 0x00, // CMPI.W #1,$600100
		0x66, 0x02, // BNE.S past the RESET
		0x4e, 0x70, // RESET
		0x60, 0xfe, // BRA.S to itself
	})
	return storage.RomFromData(data)
}

// starts is the count the ROM keeps
func starts(m *Mac) int {
	return int(m.mm.Peek(0x600100))<<8 | int(m.mm.Peek(0x600101))
}

func TestTheResetInstructionStartsTheMachineAgain(t *testing.T) {
	config, _ := quietConfiguration()
	config.RomFile = "<test>"
	m := ensureNewMac(t, config, resetOnceRom(), nil, nil)

	m.reset()
	m.RunFrames(2)

	if n := starts(m); n != 2 {
		t.Fatalf("the machine started %v times, wanted twice: once and again after RESET", n)
	}

	// With no disk to start from the machine waits for one, and that is not
	// it switched off
	if m.IsReadyToSwitchOff() {
		t.Errorf("a RESET that left no disk to start from was taken for the machine " +
			"being switched off")
	}
}

func TestAResetWithADiskIsARestart(t *testing.T) {
	config, _ := quietConfiguration()
	config.RomFile = "<test>"
	m := ensureNewMac(t, config, resetOnceRom(),
		[]storage.BlockDisk{storage.NewBlockDiskMemory(64)}, nil)

	m.reset()
	m.RunFrames(2)

	if n := starts(m); n != 2 {
		t.Fatalf("the machine started %v times, wanted twice", n)
	}
	if m.IsReadyToSwitchOff() {
		t.Errorf("a RESET with a hard disk on the bus was taken for a Shut Down")
	}
}

/*
A diskette with no boot blocks, named at startup, waits for the machine to have
started before it goes in its drive, since the ROM would eject it as it looked
for something to start from. A startup diskette goes in at once.
*/
func TestADisketteThatDoesNotStartTheMachineWaitsForIt(t *testing.T) {
	documents, err := storage.NewFloppyDiskData("documents", make([]uint8, 800*1024))
	if err != nil {
		t.Fatal(err)
	}
	startup := make([]uint8, 800*1024)
	startup[0], startup[1] = 'L', 'K'
	system, err := storage.NewFloppyDiskData("system", startup)
	if err != nil {
		t.Fatal(err)
	}

	config, _ := quietConfiguration()
	config.RomFile = "<test>"
	m := ensureNewMac(t, config, storage.RomFromData(make([]uint8, storage.RomSize)),
		nil, []*storage.FloppyDisk{system, documents})

	if m.GetDiskette(DriveInternal).Image != "system" {
		t.Errorf("the startup diskette is not in its drive from the start")
	}
	if m.GetDiskette(DriveExternal).Image != "" {
		t.Fatalf("the diskette of documents is in its drive before the machine has started")
	}

	// Something that is not an event trap leaves it waiting
	m.mm.setOverlay(false)
	m.mm.Poke(0x1000, 0x4e)
	m.mm.Poke(0x1001, 0x71) // NOP
	m.insertHeldDiskettes(0x1000)
	if m.GetDiskette(DriveExternal).Image != "" {
		t.Errorf("the diskette went in before an application asked for an event")
	}

	// And the first event asked for puts it in
	m.mm.Poke(0x1000, 0xa9)
	m.mm.Poke(0x1001, 0x70) // _GetNextEvent
	m.insertHeldDiskettes(0x1000)
	if m.GetDiskette(DriveExternal).Image != "documents" {
		t.Errorf("the diskette did not go in when the Finder asked for its first event")
	}
	if m.disketteHeld {
		t.Errorf("the hook in the instruction loop is still on with nothing to put in")
	}
}

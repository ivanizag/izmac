package izmac

import (
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
	floppy := writeImage(t, "floppy.img", 400*1024, false)

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

	if got := m.GetDiskette(DriveInternal).Image; got != floppy {
		t.Errorf("the internal drive holds %q, wanted %q", got, floppy)
	}
	if got := m.GetDiskette(DriveExternal).Image; got != "" {
		t.Errorf("the external drive holds %q, wanted nothing", got)
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

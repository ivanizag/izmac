package izmac

import (
	"strings"
	"testing"

	"github.com/ivanizag/izmac/storage"
)

// A diskette is reported rather than quietly dropped, because the drives are
// not emulated yet and a file that vanishes without a word is worse than one
// refused
func TestADisketteIsReported(t *testing.T) {
	floppy := writeImage(t, "floppy.img", 400*1024, false)

	config := NewConfiguration()
	config.RomFile = "<test>"
	if err := config.AddFiles([]string{floppy}); err != nil {
		t.Fatal(err)
	}

	m := newMac(config, storage.RomFromData(make([]uint8, storage.RomSize)), nil)

	warnings := 0
	for _, line := range m.Summary() {
		if strings.Contains(line, floppy) {
			warnings++
		}
	}
	if warnings != 1 {
		t.Fatalf("the diskette was named on %v lines of the summary, wanted one", warnings)
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
	m := newMac(config, storage.RomFromData(make([]uint8, storage.RomSize)), disks)

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

package izmac

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/ivanizag/izmac/storage"
)

// writeImage makes a file of the given size, optionally starting with the
// driver descriptor map a partitioned Macintosh disk carries
func writeImage(t *testing.T, name string, size int, partitioned bool) string {
	t.Helper()

	data := make([]uint8, size)
	if partitioned {
		data[0], data[1] = 0x45, 0x52 // 'ER'
		data[2], data[3] = 0x02, 0x00 // 512 byte blocks
	}

	filename := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(filename, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return filename
}

func TestADriverDescriptorMapMeansAHardDisk(t *testing.T) {
	// Even at a size a diskette could be, the map settles it
	filename := writeImage(t, "disk.img", floppySize800K, true)

	kind, err := classifyImage(filename)
	if err != nil {
		t.Fatal(err)
	}
	if kind != mediaHardDisk {
		t.Errorf("an image with a driver descriptor map was taken for a %v", kind)
	}
}

func TestTheDisketteSizesAreRecognized(t *testing.T) {
	for _, size := range []int{floppySize400K, floppySize800K} {
		filename := writeImage(t, "disk.img", size, false)

		kind, err := classifyImage(filename)
		if err != nil {
			t.Fatal(err)
		}
		if kind != mediaFloppy {
			t.Errorf("an image of %v bytes was taken for a %v", size, kind)
		}
	}
}

func TestAnythingElseIsAHardDisk(t *testing.T) {
	for _, size := range []int{storage.BlockSize, 20 * 1024 * 1024, 1440 * 1024} {
		filename := writeImage(t, "disk.img", size, false)

		kind, err := classifyImage(filename)
		if err != nil {
			t.Fatal(err)
		}
		if kind != mediaHardDisk {
			t.Errorf("an image of %v bytes was taken for a %v", size, kind)
		}
	}
}

func TestTheFilesOnTheCommandLineAreSorted(t *testing.T) {
	hard := writeImage(t, "hard.img", 4*1024*1024, true)
	floppy := writeImage(t, "floppy.img", floppySize800K, false)

	c := NewConfiguration()
	err := c.ParseFlags("izmac", []string{"-rom", "rom.bin", hard, floppy}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	if len(c.DiskFiles) != 1 || c.DiskFiles[0] != hard {
		t.Errorf("the hard disk did not reach the bus, the disks are %v", c.DiskFiles)
	}
	if len(c.Diskettes) != 1 || c.Diskettes[0] != floppy {
		t.Errorf("the diskette was not set aside, the diskettes are %v", c.Diskettes)
	}
}

func TestTheDiskFlagCanBeRepeated(t *testing.T) {
	c := NewConfiguration()
	err := c.ParseFlags("izmac",
		[]string{"-rom", "rom.bin", "-disk", "one.img", "-disk", "two.img"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	if len(c.DiskFiles) != 2 || c.DiskFiles[0] != "one.img" || c.DiskFiles[1] != "two.img" {
		t.Errorf("the disks are %v, wanted both in the order given", c.DiskFiles)
	}
}

func TestMoreDisksThanTheBusTakesIsRefused(t *testing.T) {
	args := []string{"-rom", "rom.bin"}
	for i := 0; i <= scsiTargetCount; i++ {
		args = append(args, "-disk", "disk.img")
	}

	c := NewConfiguration()
	if err := c.ParseFlags("izmac", args, io.Discard); err == nil {
		t.Errorf("%v disks were accepted on a bus that takes %v",
			scsiTargetCount+1, scsiTargetCount)
	}
}

// A diskette is reported rather than quietly dropped, because the drives are
// not emulated yet and a file that vanishes without a word is worse than one
// refused
func TestADisketteIsReported(t *testing.T) {
	floppy := writeImage(t, "floppy.img", floppySize400K, false)

	config := NewConfiguration()
	config.RomFile = "<test>"
	if err := config.AddFiles([]string{floppy}); err != nil {
		t.Fatal(err)
	}

	m := newMac(config, &rom{data: make([]uint8, romSize)}, nil)

	warnings := m.MediaWarnings()
	if len(warnings) != 1 {
		t.Fatalf("the diskette gave %v warnings, wanted one", len(warnings))
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
	m := newMac(config, &rom{data: make([]uint8, romSize)}, disks)

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

// Each device answers only to its own id, so the driver reaches the one it
// selected and not another
func TestEachDiskAnswersToItsOwnId(t *testing.T) {
	bus := newScsi5380()
	for id := 0; id < 3; id++ {
		disk := storage.NewBlockDiskMemory(uint32(16 + id))
		bus.attach(newScsiTarget(uint8(id), disk, false))
	}

	for id := uint8(0); id < 3; id++ {
		selectTarget(bus, id)
		if bus.selected == nil {
			t.Fatalf("selecting the id %v answered nothing", id)
		}
		if bus.selected.id != id {
			t.Errorf("selecting the id %v answered the id %v", id, bus.selected.id)
		}

		// Read the capacity, which is different on each of them
		data, status := runCommand(bus,
			[]uint8{scsiCmdReadCapacity, 0, 0, 0, 0, 0, 0, 0, 0, 0})
		if status != scsiStatusGood {
			t.Fatalf("the id %v answered the status $%02x", id, status)
		}

		last := uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])
		if wanted := uint32(16+id) - 1; last != wanted {
			t.Errorf("the id %v reports the last block %v, wanted %v", id, last, wanted)
		}
	}
}

func TestSelectingAnIdWithNothingOnItAnswersNothing(t *testing.T) {
	bus := newScsi5380()
	bus.attach(newScsiTarget(0, storage.NewBlockDiskMemory(16), false))

	selectTarget(bus, 3)
	if bus.selected != nil {
		t.Errorf("the id 3 was answered by the device at the id %v", bus.selected.id)
	}
	if bus.phase() != phaseBusFree {
		t.Errorf("the bus is on %v after selecting nothing", bus.phase())
	}
}

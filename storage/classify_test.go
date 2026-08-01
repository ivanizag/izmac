package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeImage makes a file of the given size, optionally starting with the
// driver descriptor map a partitioned Macintosh disk carries
func writeImage(t *testing.T, size int, partitioned bool) string {
	t.Helper()

	data := make([]uint8, size)
	if partitioned {
		data[0], data[1] = 0x45, 0x52 // 'ER'
		data[2], data[3] = 0x02, 0x00 // 512 byte blocks
	}

	filename := filepath.Join(t.TempDir(), "disk.img")
	if err := os.WriteFile(filename, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return filename
}

func TestADriverDescriptorMapMeansAHardDisk(t *testing.T) {
	// Even at a size a diskette could be, the map settles it
	filename := writeImage(t, floppySize800K, true)

	kind, err := Classify(filename)
	if err != nil {
		t.Fatal(err)
	}
	if kind != KindHardDisk {
		t.Errorf("an image with a driver descriptor map was taken for a %v", kind)
	}
}

func TestTheDisketteSizesAreRecognized(t *testing.T) {
	for _, size := range []int{floppySize400K, floppySize800K} {
		filename := writeImage(t, size, false)

		kind, err := Classify(filename)
		if err != nil {
			t.Fatal(err)
		}
		if kind != KindFloppy {
			t.Errorf("an image of %v bytes was taken for a %v", size, kind)
		}
	}
}

func TestAnythingElseIsAHardDisk(t *testing.T) {
	for _, size := range []int{BlockSize, 20 * 1024 * 1024, 5 * 1024 * 1024} {
		filename := writeImage(t, size, false)

		kind, err := Classify(filename)
		if err != nil {
			t.Fatal(err)
		}
		if kind != KindHardDisk {
			t.Errorf("an image of %v bytes was taken for a %v", size, kind)
		}
	}
}

/*
An image too big for a drive with an HFS volume where the driver descriptor
map should be is one of the images made for the emulators that patch the ROM,
and the ROM izmac runs can do nothing with it.
*/
func TestAVolumeWithNoMapInFrontOfItIsRecognized(t *testing.T) {
	kind, err := Classify(writeVolume(t, 40))
	if err != nil {
		t.Fatal(err)
	}
	if kind != KindBareVolume {
		t.Errorf("a volume with no map in front of it was taken for a %v", kind)
	}
}

/*
A diskette is a volume with no map in front of it as well, and the only thing
telling the two apart is that a drive can hold one. Were the sizes not looked
at first, every 800K image would come out a bare volume and be turned away.
*/
func TestADisketteSizedVolumeIsStillADiskette(t *testing.T) {
	for _, size := range []int{floppySize400K, floppySize800K} {
		kind, err := Classify(writeVolume(t, size/BlockSize))
		if err != nil {
			t.Fatal(err)
		}
		if kind != KindFloppy {
			t.Errorf("an HFS volume of %v bytes was taken for a %v", size, kind)
		}
	}
}

/*
A blank image has no map and no volume either. It is what gets attached to be
formatted from the machine, so it stays a hard disk.
*/
func TestABlankImageIsAHardDisk(t *testing.T) {
	kind, err := Classify(writeImage(t, 20*1024*1024, false))
	if err != nil {
		t.Fatal(err)
	}
	if kind != KindHardDisk {
		t.Errorf("a blank image was taken for a %v", kind)
	}
}

// An image too short to hold the block a volume is known by is not one, and
// the read past the end of it is not an error
func TestAnImageTooShortToHoldAVolumeIsAHardDisk(t *testing.T) {
	short := filepath.Join(t.TempDir(), "short.img")
	if err := os.WriteFile(short, []uint8{'L', 'K'}, 0o600); err != nil {
		t.Fatal(err)
	}

	kind, err := Classify(short)
	if err != nil {
		t.Fatal(err)
	}
	if kind != KindHardDisk {
		t.Errorf("a two byte file was taken for a %v", kind)
	}
}

func TestAnImageThatIsNotThereIsAnError(t *testing.T) {
	if _, err := Classify(filepath.Join(t.TempDir(), "missing.img")); err == nil {
		t.Error("an image that is not there was classified anyway")
	}
}

/*
A 1.44Mb image is a diskette and is sorted as one, even though this machine
can not read it. It would otherwise be left to fall through to the SCSI bus
and be attached there as a hard disk, which looks like it worked and is the
worst of the answers available: what the file really is is the right System on
the wrong kind of disk, and saying so is the only useful thing to do with it.
*/
func TestAnUnreadableDisketteIsStillADiskette(t *testing.T) {
	for _, size := range []int{floppySize720K, floppySize1440K} {
		filename := writeImage(t, size, false)

		kind, err := Classify(filename)
		if err != nil {
			t.Fatal(err)
		}
		if kind != KindFloppy {
			t.Errorf("an image of %v bytes was taken for a %v", size, kind)
		}

		_, err = NewFloppyDisk(filename, false)
		if err == nil {
			t.Fatalf("an image of %v bytes was opened as a diskette", size)
		}
		if !strings.Contains(err.Error(), "800Kb") {
			t.Errorf("opening a %v byte image said %q, which does not say what "+
				"the machine can read", size, err)
		}
	}
}

package storage

import (
	"os"
	"path/filepath"
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
	for _, size := range []int{BlockSize, 20 * 1024 * 1024, 1440 * 1024} {
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

func TestAnImageThatIsNotThereIsAnError(t *testing.T) {
	if _, err := Classify(filepath.Join(t.TempDir(), "missing.img")); err == nil {
		t.Error("an image that is not there was classified anyway")
	}
}

package storage

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newServer serves the given bytes as a download
func newServer(data []uint8, status int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			w.Write(data)
		}))
}

func TestTheRomIsDownloadedAndSaved(t *testing.T) {
	data := buildTestRom(PlusRomVersions()[2].Checksum)
	server := newServer(data, http.StatusOK)
	defer server.Close()

	filename := filepath.Join(t.TempDir(), "rom.bin")
	r, err := DownloadRom(filename, server.URL)
	if err != nil {
		t.Fatal(err)
	}

	saved, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != RomSize {
		t.Errorf("the file saved is %v bytes, wanted %v", len(saved), RomSize)
	}

	// And it says what it turned out to be, which is what the caller
	// reports
	if !strings.Contains(r.String(), "Loud Harmonicas") {
		t.Errorf("the download identified itself as %q", r.String())
	}
}

func TestABadDownloadLeavesNoFile(t *testing.T) {
	// Something that is not a ROM must not be saved
	server := newServer([]uint8("<html>not a rom</html>"), http.StatusOK)
	defer server.Close()

	filename := filepath.Join(t.TempDir(), "rom.bin")

	_, err := DownloadRom(filename, server.URL)
	if err == nil {
		t.Fatal("a download that is not a ROM was accepted")
	}

	if _, err := os.Stat(filename); err == nil {
		t.Error("a broken download was left on the disk")
	}
}

func TestAServerErrorIsReported(t *testing.T) {
	server := newServer(nil, http.StatusNotFound)
	defer server.Close()

	_, err := DownloadRom(filepath.Join(t.TempDir(), "rom.bin"), server.URL)
	if err == nil {
		t.Fatal("a failed download was accepted")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("%q does not report what the server answered", err)
	}
}

/*
zipOf packs one file, the way the disks that carry a driver are published.
The image is padded out because a real one is far bigger than the part taken
off the front of it.
*/
func zipOf(t *testing.T, name string, image []uint8, blocks int) []uint8 {
	t.Helper()

	padded := make([]uint8, blocks*BlockSize)
	copy(padded, image)

	return zipOfAll(t, zipEntry{name, padded})
}

// zipEntry is one file to pack, as it goes in the archive
type zipEntry struct {
	name string
	data []uint8
}

// zipOfAll packs several files, in the order they are given
func zipOfAll(t *testing.T, entries ...zipEntry) []uint8 {
	t.Helper()

	buffer := &bytes.Buffer{}
	writer := zip.NewWriter(buffer)
	for _, entry := range entries {
		file, err := writer.Create(entry.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(entry.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// donorBytes is the made up donor as it sits on disk
func donorBytes(t *testing.T) []uint8 {
	t.Helper()

	data, err := os.ReadFile(writeDonor(t))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

/*
The front of the image is what is kept, and it has to be enough of one for
the driver to be read straight out of it afterwards.
*/
func TestTheFrontOfAZippedImageIsTaken(t *testing.T) {
	archive := zipOf(t, "20mb.img", donorBytes(t), 4096)

	data, err := frontOfZippedImage(archive)
	if err != nil {
		t.Fatal(err)
	}

	if len(data) != driverFileBlocks*BlockSize {
		t.Errorf("%v bytes were kept, wanted the %v blocks the layout asks for",
			len(data), driverFileBlocks)
	}

	// And the point of it: the driver comes out of what was kept
	driver, err := readDriverAt(bytes.NewReader(data), "downloaded")
	if err != nil {
		t.Fatalf("the driver could not be read back out of the front of the image: %v", err)
	}
	if driver.Blocks() != donorDriverBlocks {
		t.Errorf("the driver came out %v blocks, wanted %v",
			driver.Blocks(), donorDriverBlocks)
	}
	if driver.Processor != "68000" {
		t.Errorf("the driver is for %q", driver.Processor)
	}
}

// An image shorter than the part wanted is not quietly padded out into
// something that looks like a disk
func TestAZippedImageTooShortIsAnError(t *testing.T) {
	archive := zipOf(t, "tiny.img", donorBytes(t), driverFileBlocks-1)

	if _, err := frontOfZippedImage(archive); err == nil {
		t.Error("an image shorter than the blocks kept was accepted")
	}
}

func TestSomethingThatIsNotAZipIsAnError(t *testing.T) {
	if _, err := frontOfZippedImage([]uint8("this is not a zip at all")); err == nil {
		t.Error("a file that is not a zip was unpacked anyway")
	}
}

/*
A zip holding a file with no driver in it is refused before anything is
written, so a bad download leaves nothing behind to be picked up next time.
*/
func TestAZippedImageWithNoDriverIsRefused(t *testing.T) {
	archive := zipOf(t, "blank.img", nil, 4096)

	data, err := frontOfZippedImage(archive)
	if err != nil {
		t.Fatal(err)
	}

	_, err = readDriverAt(bytes.NewReader(data), "downloaded")
	if err == nil {
		t.Fatal("an image with no driver descriptor map gave up a driver")
	}
	if !strings.Contains(err.Error(), "driver descriptor map") {
		t.Errorf("the error was %q, which does not say what is missing", err)
	}
}

func TestTheDriverIsDownloadedAndSaved(t *testing.T) {
	server := newServer(zipOf(t, "20mb.img", donorBytes(t), 4096), http.StatusOK)
	defer server.Close()

	filename := filepath.Join(t.TempDir(), "hddriver.rom")
	driver, err := DownloadDriver(filename, server.URL)
	if err != nil {
		t.Fatal(err)
	}

	if driver.Blocks() != donorDriverBlocks {
		t.Errorf("the driver came out %v blocks, wanted %v",
			driver.Blocks(), donorDriverBlocks)
	}
	// It says where it ended up rather than where it came from, which is
	// what the summary reports
	if driver.source != filename {
		t.Errorf("the driver says it came from %v, wanted %v", driver.source, filename)
	}

	saved, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != driverFileBlocks*BlockSize {
		t.Errorf("the file saved is %v bytes, wanted the %v blocks kept",
			len(saved), driverFileBlocks)
	}

	// And what was saved is a disk image in its own right, so it is read
	// back on the next run the same way any donor is
	if _, err := ReadDriver(filename); err != nil {
		t.Errorf("the file saved does not parse as a disk with a driver: %v", err)
	}
}

// diskette400K is a plain image of the size the smaller drives write, which
// is what the startup diskette comes as
func diskette400K() []uint8 {
	image := make([]uint8, floppySize400K)
	image[0], image[1] = 'L', 'K' // the boot blocks
	return image
}

func TestADisketteIsDownloadedAndSaved(t *testing.T) {
	// A plain image, which is how the one izmac fetches is published
	server := newServer(diskette400K(), http.StatusOK)
	defer server.Close()

	filename := filepath.Join(t.TempDir(), "macpaint.dsk")
	diskette, err := DownloadDiskette(filename, server.URL)
	if err != nil {
		t.Fatal(err)
	}

	if diskette.Sides() != 1 {
		t.Errorf("the diskette came out with %v sides, wanted the one a 400Kb image has",
			diskette.Sides())
	}

	saved, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != floppySize400K {
		t.Errorf("the file saved is %v bytes, wanted the %v of the image",
			len(saved), floppySize400K)
	}

	// And the point of saving it: it goes in a drive on the next run
	// without being downloaded again
	if _, err := NewFloppyDisk(filename, false); err != nil {
		t.Errorf("the file saved does not read back as a diskette: %v", err)
	}
}

/*
A diskette also comes zipped, which is how the other collections publish
them. A zip packed on a Macintosh carries a second entry for every file,
under __MACOSX and holding what was in the resource fork; it comes first in
the archive and taking it for the image would leave the download refused.
*/
func TestAZippedDisketteIsUnpacked(t *testing.T) {
	archive := zipOfAll(t,
		zipEntry{"__MACOSX/._MacPaint 1.0.img", []uint8("the resource fork")},
		zipEntry{"MacPaint 1.0.img", diskette400K()})

	server := newServer(archive, http.StatusOK)
	defer server.Close()

	filename := filepath.Join(t.TempDir(), "macpaint.dsk")
	if _, err := DownloadDiskette(filename, server.URL); err != nil {
		t.Fatalf("the image next to the fork it was packed with was not found: %v", err)
	}

	saved, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != floppySize400K {
		t.Errorf("what was saved is %v bytes, so it is not the image", len(saved))
	}
}

// An image of a size no drive of this machine writes is refused before
// anything is written, the way the others are
func TestABadDisketteDownloadLeavesNoFile(t *testing.T) {
	for _, download := range [][]uint8{
		make([]uint8, floppySize400K/2),
		zipOfAll(t, zipEntry{"half.img", make([]uint8, floppySize400K/2)}),
		[]uint8("<html>not a diskette at all</html>"),
	} {
		server := newServer(download, http.StatusOK)

		filename := filepath.Join(t.TempDir(), "macpaint.dsk")
		if _, err := DownloadDiskette(filename, server.URL); err == nil {
			t.Error("a download that is not a diskette was accepted")
		}
		if _, err := os.Stat(filename); err == nil {
			t.Error("a broken download was left on the disk")
		}

		server.Close()
	}
}

func TestABadDriverDownloadLeavesNoFile(t *testing.T) {
	server := newServer([]uint8("<html>not a disk</html>"), http.StatusOK)
	defer server.Close()

	filename := filepath.Join(t.TempDir(), "hddriver.rom")

	if _, err := DownloadDriver(filename, server.URL); err == nil {
		t.Fatal("a download that is not a disk image was accepted")
	}
	if _, err := os.Stat(filename); err == nil {
		t.Error("a broken download was left on the disk")
	}
}

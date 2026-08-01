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

	buffer := &bytes.Buffer{}
	writer := zip.NewWriter(buffer)
	file, err := writer.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(padded); err != nil {
		t.Fatal(err)
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

package storage

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newRomServer serves the given bytes as a ROM download
func newRomServer(data []uint8, status int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			w.Write(data)
		}))
}

func TestTheRomIsDownloadedAndSaved(t *testing.T) {
	data := buildTestRom(PlusRomVersions()[2].Checksum)
	server := newRomServer(data, http.StatusOK)
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
	server := newRomServer([]uint8("<html>not a rom</html>"), http.StatusOK)
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
	server := newRomServer(nil, http.StatusNotFound)
	defer server.Close()

	_, err := DownloadRom(filepath.Join(t.TempDir(), "rom.bin"), server.URL)
	if err == nil {
		t.Fatal("a failed download was accepted")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("%q does not report what the server answered", err)
	}
}

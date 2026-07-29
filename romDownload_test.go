package izmac

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
	data := buildTestRom(preferredRomChecksum)
	server := newRomServer(data, http.StatusOK)
	defer server.Close()

	filename := filepath.Join(t.TempDir(), defaultRomFile)
	out := &strings.Builder{}

	err := downloadRom(filename, server.URL, out)
	if err != nil {
		t.Fatal(err)
	}

	saved, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != romSize {
		t.Errorf("the file saved is %v bytes, wanted %v", len(saved), romSize)
	}

	// The download has to say what it is doing and where it comes from
	report := out.String()
	if !strings.Contains(report, "is not here") {
		t.Errorf("%q does not say the ROM was missing", report)
	}
	if !strings.Contains(report, server.URL) {
		t.Errorf("%q does not name the source", report)
	}
	if !strings.Contains(report, "Loud Harmonicas") {
		t.Errorf("%q does not identify what was downloaded", report)
	}
}

func TestABadDownloadLeavesNoFile(t *testing.T) {
	// Something that is not a ROM must not be saved
	server := newRomServer([]uint8("<html>not a rom</html>"), http.StatusOK)
	defer server.Close()

	filename := filepath.Join(t.TempDir(), defaultRomFile)

	err := downloadRom(filename, server.URL, &strings.Builder{})
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

	err := downloadRom(filepath.Join(t.TempDir(), defaultRomFile),
		server.URL, &strings.Builder{})
	if err == nil {
		t.Fatal("a failed download was accepted")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("%q does not report what the server answered", err)
	}
}

func TestAnExistingRomIsNotDownloaded(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, defaultRomFile)
	err := os.WriteFile(filename, buildTestRom(preferredRomChecksum), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	config := NewConfiguration()
	config.RomFile = filename
	config.romIsDefault = true

	out := &strings.Builder{}
	err = ensureRom(config, out)
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != "" {
		t.Errorf("a ROM already there was reported as missing: %q", out.String())
	}
}

func TestARomNamedOnTheCommandLineIsNotDownloaded(t *testing.T) {
	config := NewConfiguration()
	config.RomFile = filepath.Join(t.TempDir(), "missing.rom")
	config.romIsDefault = false

	out := &strings.Builder{}
	err := ensureRom(config, out)

	// Nothing is fetched and nothing is said, the caller reports the
	// missing file when it tries to read it
	if err != nil {
		t.Errorf("a missing named ROM was treated as an error too early: %v", err)
	}
	if out.String() != "" {
		t.Errorf("a named ROM triggered a download: %q", out.String())
	}
}

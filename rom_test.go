package izmac

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ivanizag/izmac/storage"
)

/*
Whether to fetch a ROM at all is the machine's decision and not the image's,
so it is tested here. A ROM named on the command line is never downloaded: the
user said where it is, and quietly fetching a different one over the top of a
missing file is not what they asked for.
*/
func TestAnExistingRomIsNotDownloaded(t *testing.T) {
	filename := filepath.Join(t.TempDir(), defaultRomFile)
	if err := os.WriteFile(filename, make([]uint8, storage.RomSize), 0o600); err != nil {
		t.Fatal(err)
	}

	config := NewConfiguration()
	config.RomFile = filename
	config.romIsDefault = true

	out := &strings.Builder{}
	if err := ensureRom(config, out); err != nil {
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

	// Nothing is fetched and nothing is said. The caller reports the
	// missing file when it tries to read it.
	if err != nil {
		t.Errorf("a missing named ROM was treated as an error too early: %v", err)
	}
	if out.String() != "" {
		t.Errorf("a named ROM triggered a download: %q", out.String())
	}
}

// The revision izmac targets is the one that copes with a target answering a
// unit attention after a reset, which is what an emulated disk does
func TestTheTargetedRevisionIsTheOneThatCopesWithUnitAttention(t *testing.T) {
	versions := storage.PlusRomVersions()

	var preferred storage.RomVersion
	for _, v := range versions {
		if v.Checksum == preferredRomChecksum {
			preferred = v
		}
	}

	if preferred.Nickname != "Loud Harmonicas" {
		t.Errorf("izmac targets %q, wanted Loud Harmonicas", preferred.Nickname)
	}
	if preferred.Notes != "" {
		t.Errorf("the revision targeted carries the caveat %q", preferred.Notes)
	}

	// And the others are recognised but not preferred
	for _, v := range versions {
		if v.Checksum == preferredRomChecksum {
			continue
		}
		if v.Notes == "" {
			t.Errorf("%v is not targeted and says nothing about why", v.Nickname)
		}
	}
}

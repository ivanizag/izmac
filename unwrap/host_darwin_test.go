//go:build darwin

package unwrap

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

// macOS keeps the resource fork and the Finder information with the file, and
// that is where they are read from
func TestTheForksOfAMacOSFileAreRead(t *testing.T) {
	name := filepath.Join(t.TempDir(), "Game")
	if err := os.WriteFile(name, []uint8("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	resource := someImage(800)
	if err := os.WriteFile(name+"/..namedfork/rsrc", resource, 0o600); err != nil {
		t.Skipf("the file system does not keep resource forks: %v", err)
	}
	finder := make([]uint8, 32)
	copy(finder, "APPLGAME\x20\x00")
	if err := unix.Setxattr(name, "com.apple.FinderInfo", finder, 0); err != nil {
		t.Skipf("the file system does not keep Finder information: %v", err)
	}

	if !HasHostMetadata(name) {
		t.Errorf("the file was not taken for a Macintosh one")
	}

	files, err := ReadHost(name)
	if err != nil {
		t.Fatal(err)
	}
	f := files[0]
	if !bytes.Equal(f.Resource, resource) || string(f.Data) != "data" {
		t.Errorf("the forks came back as %v and %v bytes", len(f.Data), len(f.Resource))
	}
	if string(f.Type[:]) != "APPL" || string(f.Creator[:]) != "GAME" || f.Flags != 0x2000 {
		t.Errorf("the Finder information came back as %q %q $%04x", f.Type, f.Creator, f.Flags)
	}
}

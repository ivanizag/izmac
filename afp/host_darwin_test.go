//go:build darwin

package afp

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

// hasXattr tells whether a file has an extended attribute
func hasXattr(host string, name string) bool {
	_, err := unix.Getxattr(host, name, nil)
	return err == nil
}

// What the machine writes goes to the AppleDouble file and nothing else
func TestNothingIsKeptOutOfSightOnMacOS(t *testing.T) {
	c := newTestClient(t)
	c.call(errNoErr, fpCreateFile, 0, uint16(volumeID), uint32(rootID), longPath("App"))
	ref := c.openFork(true, accessRead|accessWrite, "App")
	c.writeFork(ref, 0, "resources")
	c.call(errNoErr, fpCloseFork, 0, ref)
	finder := make([]uint8, 32)
	copy(finder, "APPLizmc")
	c.call(errNoErr, fpSetFileParms, 0, uint16(volumeID), uint32(rootID), uint16(paramFinderInfo),
		longPath("App"), uint8(0), finder)

	app := filepath.Join(c.folder, "App")
	for _, name := range []string{"com.apple.FinderInfo", "com.apple.ResourceFork"} {
		if hasXattr(app, name) {
			t.Errorf("the file has the extended attribute %v", name)
		}
	}
	if _, err := os.Stat(filepath.Join(c.folder, "._App")); err != nil {
		t.Errorf("there is no ._App: %v", err)
	}
}

/*
A Macintosh file that macOS keeps with extended attributes, an application
unpacked from an archive, is served with them; what the machine changes goes
to an AppleDouble file with the rest of what it had, and the attributes are
left as they were
*/
func TestExtendedAttributesAreReadAndLeftAlone(t *testing.T) {
	c := newTestClient(t)
	app := filepath.Join(c.folder, "App")
	os.WriteFile(app, nil, 0o644)
	original := make([]uint8, 32)
	copy(original, "APPLold!")
	unix.Setxattr(app, "com.apple.FinderInfo", original, 0)
	unix.Setxattr(app, "com.apple.ResourceFork", []uint8("the fork"), 0)

	reply := c.call(errNoErr, fpGetFileDirParms, 0, uint16(volumeID), uint32(rootID),
		uint16(paramFinderInfo|fileResourceLength), uint16(0), longPath("App"))
	if !bytes.Equal(reply[6:14], []uint8("APPLold!")) || binary.BigEndian.Uint32(reply[38:]) != 8 {
		t.Fatalf("the file is served as %x", reply[6:])
	}

	changed := make([]uint8, 32)
	copy(changed, "APPLnew!")
	c.call(errNoErr, fpSetFileParms, 0, uint16(volumeID), uint32(rootID), uint16(paramFinderInfo),
		longPath("App"), uint8(0), changed)

	store := newMetadataStore(c.folder)
	f, err := os.Open(store.sidecar(app))
	if err != nil {
		t.Fatalf("the change went to no AppleDouble file: %v", err)
	}
	defer f.Close()
	kept := readAppleDouble(f, true)
	if string(kept.finder[:8]) != "APPLnew!" || string(kept.resource) != "the fork" {
		t.Errorf("the AppleDouble file has %q and %q", kept.finder[:8], kept.resource)
	}
	attribute := make([]uint8, 32)
	unix.Getxattr(app, "com.apple.FinderInfo", attribute)
	if !bytes.Equal(attribute, original) {
		t.Errorf("the extended attribute was changed to %q", attribute[:8])
	}
}

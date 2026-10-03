package izmac

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ivanizag/izmac/hfs"
)

// buildDiskette makes an 800K diskette with the files given on it, which has
// no System and does not start the machine
func buildDiskette(t *testing.T, name string, files ...*hfs.File) string {
	t.Helper()
	root := &hfs.Folder{Modified: time.Now()}
	for _, f := range files {
		root.Add(nil, f)
	}
	data, err := hfs.Build(name, root, 800*1024)
	if err != nil {
		t.Fatal(err)
	}
	image := filepath.Join(t.TempDir(), name+".dsk")
	if err := os.WriteFile(image, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return image
}

/*
The Finder copies a file from one diskette to the other, dragged from the
window of one to the icon of the other: both drives, read on one and written
on the other, the external drive included, and the mouse dragging. What says
it worked is the image of the diskette it went to, on the host.
*/
func TestTheFinderCopiesBetweenDiskettes(t *testing.T) {
	const text = "copied between diskettes"
	m := bootedMac(t)
	source := buildDiskette(t, "Source", &hfs.File{
		Name: "Note", Data: []uint8(text), Modified: time.Now(),
		Type: [4]uint8{'T', 'E', 'X', 'T'}, Creator: [4]uint8{'t', 't', 'x', 't'},
	})
	target := buildDiskette(t, "Target")

	// Both in once the Finder runs, there being no System on them
	if err := m.InsertDiskette(DriveInternal, source); err != nil {
		t.Fatal(err)
	}
	m.RunFrames(600)
	if err := m.InsertDiskette(DriveExternal, target); err != nil {
		t.Fatal(err)
	}
	m.RunFrames(600)
	if volumes := mountedVolumes(m); len(volumes) != 3 {
		t.Fatalf("the machine has %q mounted, wanted its disk and both diskettes", volumes)
	}

	// The source opened, its file dragged to the icon of the target
	doubleClickAt(t, m, 472, 110)
	m.RunFrames(900)
	moveMouseTo(t, m, 119, 125)
	m.SetMouseButton(true)
	m.RunFrames(20)
	moveMouseTo(t, m, 300, 150)
	moveMouseTo(t, m, 472, 172)
	m.RunFrames(20)
	m.SetMouseButton(false)

	copied := func() bool {
		if err := m.FlushDiskettes(); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(target)
		return bytes.Contains(data, []uint8(text)) && bytes.Contains(data, []uint8("Note"))
	}
	if !waitUntil(m, 60, copied) {
		t.Fatalf("the file never reached the target diskette")
	}

	// The Finder finishes with the desktop file, and the source is as it was
	m.RunFrames(600)
	if data, _ := os.ReadFile(source); !bytes.Contains(data, []uint8(text)) {
		t.Errorf("the source diskette lost its file")
	}
}

/*
An application handed to izmac packed, as one comes from the archives where
they are kept: TeachText in MacBinary. izmac unpacks it onto a volume of its
own, which goes in once the Finder runs, and the application runs from there,
which says both of its forks came through.
*/
func TestAnArchivedApplicationRuns(t *testing.T) {
	config := realConfig(t)
	if err := config.AddFiles([]string{testImage(t, testImages+"/teachtext.bin")}); err != nil {
		t.Fatal(err)
	}
	m := buildTestMac(t, config)
	waitForApplication(t, m, "Finder", 60)
	if !waitUntil(m, 30, func() bool { return len(mountedVolumes(m)) == 2 }) {
		t.Fatalf("the machine has %q mounted, wanted the volume of the archive too", mountedVolumes(m))
	}
	m.RunFrames(600)

	// The volume, under the disk, and TeachText in it
	doubleClickAt(t, m, 472, 110)
	m.RunFrames(900)
	doubleClickAt(t, m, 119, 125)
	waitForApplication(t, m, "TeachText", 20)
	waitUntil(m, 20, func() bool { return applicationVolume(m) != "" })

	if volume := applicationVolume(m); volume != "teachtext" {
		t.Errorf("TeachText runs from %q, wanted the volume of the archive", volume)
	}
}

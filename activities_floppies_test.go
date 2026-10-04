package izmac

import (
	"os"
	"path/filepath"
	"testing"
)

/*
floppiesScreenshots is a Macintosh with one diskette drive and System 6 on a
diskette: the startup diskette ejected, a blank one initialized, a file
copied from one to the other with the swapping that takes, and the new
diskette put away in the Trash
*/
func floppiesScreenshots(t *testing.T) {
	const page = "floppies"
	config := testConfig(t)
	system := testImage(t, testSystemSixDiskette)
	config.Diskettes = []string{system}
	m := buildTestMac(t, config)
	waitForApplication(t, m, "Finder", 60)
	m.RunFrames(600)

	// The startup diskette ejected, its icon left dimmed
	moveMouseTo(t, m, 472, 50)
	clickMouse(m)
	pressCommand(m, "E")
	m.RunFrames(600)
	screenshot(t, m, page, "ejected")

	// A blank diskette, a file of zeros as dd makes it, put in the drive
	blank := filepath.Join(t.TempDir(), "blank.dsk")
	if err := os.WriteFile(blank, make([]uint8, 800*1024), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.InsertDiskette(0, blank); err != nil {
		t.Fatal(err)
	}
	m.RunFrames(600)
	screenshot(t, m, page, "not-a-macintosh-disk")

	// Two-sided, erased, named, and formatted
	moveMouseTo(t, m, 344, 159)
	clickMouse(m)
	m.RunFrames(300)
	moveMouseTo(t, m, 345, 159)
	clickMouse(m)
	m.RunFrames(600)
	typeString(m, "Letters")
	m.RunFrames(30)
	screenshot(t, m, page, "name")
	pressKey(m, "Return")
	m.RunFrames(600)
	screenshot(t, m, page, "formatting")
	m.RunFrames(3000)
	screenshot(t, m, page, "initialized")

	// The startup diskette opened again, which wants it back in the drive
	doubleClickAt(t, m, 472, 50)
	swapDiskettes(t, m, nil, 1200, system, blank)

	// The Read Me dragged to Letters, recorded with the swapping it takes
	moveMouseTo(t, m, 172, 105)
	m.RunFrames(30)
	copying := record(m, "")
	copying.capture(50)
	copying.press(true)
	copying.run(20, 2)
	copying.glide(t, 472, 115)
	copying.run(20, 2)
	copying.press(false)
	swapDiskettes(t, m, copying, 3600, system, blank)
	copying.save(t, page, "swapping", 300)

	// Letters opened, with the Read Me on it
	doubleClickAt(t, m, 472, 115)
	swapDiskettes(t, m, nil, 1200, system, blank)
	screenshot(t, m, page, "copied")

	// Letters put away in the Trash, which ejects it and forgets it
	trash := record(m, "")
	moveMouseTo(t, m, 472, 115)
	m.RunFrames(30)
	trash.capture(50)
	trash.press(true)
	trash.run(20, 2)
	trash.glide(t, 472, 318)
	trash.run(20, 2)
	trash.press(false)
	swapDiskettes(t, m, trash, 600, system, blank)
	trash.save(t, page, "put-away", 300)
}

/*
swapDiskettes runs the machine for some frames, as a person at a Macintosh of
one drive did: whenever it ejects the diskette in the drive, the other one
goes in. A recording, if there is one, takes a picture every half second.
*/
func swapDiskettes(t *testing.T, m *Mac, rec *recording, frames int, a string, b string) {
	t.Helper()
	inDrive := m.GetDiskette(0).Image
	for frame := 0; frame < frames; frame += 30 {
		if rec != nil {
			rec.run(30, 30)
		} else {
			m.RunFrames(30)
		}
		if image := m.GetDiskette(0).Image; image != "" {
			inDrive = image
			continue
		}
		next := a
		if filepath.Base(inDrive) == filepath.Base(a) {
			next = b
		}
		m.RunFrames(60)
		if err := m.InsertDiskette(0, next); err != nil {
			t.Fatal(err)
		}
		inDrive = next
	}
}

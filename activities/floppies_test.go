package activities

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ivanizag/izmac/operator"
)

/*
floppiesScreenshots is a Macintosh with one diskette drive and System 6 on a
diskette: the startup diskette ejected, a blank one initialized, a file
copied from one to the other with the swapping that takes, and the new
diskette put away in the Trash
*/
func floppiesScreenshots(t *testing.T) {
	const page = "floppies"
	album := NewAlbum(filepath.Join(activityImages, page))
	config := testConfig(t)
	system := testImage(t, testSystemSixDiskette)
	config.Diskettes = []string{system}
	m := buildTestMac(t, config)
	must(t, operator.New(m).WaitForApplication("Finder", 60))
	m.RunFrames(600)

	// The startup diskette ejected, its icon left dimmed
	must(t, operator.New(m).MoveTo(472, 50))
	operator.New(m).Click()
	must(t, operator.New(m).Command("E"))
	m.RunFrames(600)
	must(t, album.Screenshot(m, "ejected"))

	// A blank diskette, a file of zeros as dd makes it, put in the drive
	blank := filepath.Join(t.TempDir(), "blank.dsk")
	if err := os.WriteFile(blank, make([]uint8, 800*1024), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.InsertDiskette(0, blank); err != nil {
		t.Fatal(err)
	}
	m.RunFrames(600)
	must(t, album.Screenshot(m, "not-a-macintosh-disk"))

	// Two-sided, erased, named, and formatted
	must(t, operator.New(m).MoveTo(344, 159))
	operator.New(m).Click()
	m.RunFrames(300)
	must(t, operator.New(m).MoveTo(345, 159))
	operator.New(m).Click()
	m.RunFrames(600)
	must(t, operator.New(m).TypeString("Letters"))
	m.RunFrames(30)
	must(t, album.Screenshot(m, "name"))
	must(t, operator.New(m).PressKey("Return"))
	m.RunFrames(600)
	must(t, album.Screenshot(m, "formatting"))
	m.RunFrames(3000)
	must(t, album.Screenshot(m, "initialized"))

	// The startup diskette opened again, which wants it back in the drive
	must(t, operator.New(m).DoubleClickAt(472, 50))
	must(t, operator.New(m).SwapDiskettes(1200, system, blank, nil))

	// The Read Me dragged to Letters, recorded with the swapping it takes
	must(t, operator.New(m).MoveTo(172, 105))
	m.RunFrames(30)
	copying := Record(operator.New(m), "")
	copying.Capture(50)
	copying.Press(true)
	copying.Run(20, 2)
	must(t, copying.Glide(472, 115))
	copying.Run(20, 2)
	copying.Press(false)
	must(t, operator.New(m).SwapDiskettes(3600, system, blank, func() { copying.Capture(50) }))
	must(t, album.SaveRecording(copying, "swapping", 300))

	// Letters opened, with the Read Me on it
	must(t, operator.New(m).DoubleClickAt(472, 115))
	must(t, operator.New(m).SwapDiskettes(1200, system, blank, nil))
	must(t, album.Screenshot(m, "copied"))

	// Letters put away in the Trash, which ejects it and forgets it
	trash := Record(operator.New(m), "")
	must(t, operator.New(m).MoveTo(472, 115))
	m.RunFrames(30)
	trash.Capture(50)
	trash.Press(true)
	trash.Run(20, 2)
	must(t, trash.Glide(472, 318))
	trash.Run(20, 2)
	trash.Press(false)
	must(t, operator.New(m).SwapDiskettes(600, system, blank, func() { trash.Capture(50) }))
	must(t, album.SaveRecording(trash, "put-away", 300))
}

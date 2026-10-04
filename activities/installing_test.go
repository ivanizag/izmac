package activities

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ivanizag/izmac/operator"
)

/*
installingScreenshots is a blank hard disk made a startup disk: formatted
with HD SC Setup from the Utilities 1 diskette of System 6.0.8, System 6
installed on it by the Installer of System Tools with the diskettes it asks
for, and the machine restarted from it
*/
func installingScreenshots(t *testing.T) {
	const page = "installing"
	album := NewAlbum(filepath.Join(activityImages, page))
	folder := t.TempDir()
	disk := filepath.Join(folder, "blank.img")
	if err := os.WriteFile(disk, make([]uint8, 20<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	utilitiesOne := testImage(t, testUtilitiesOneDiskette)
	systemTools := testImage(t, testSystemToolsDiskette)
	utilitiesTwo := testImage(t, testUtilitiesTwoDiskette)
	printingTools := testImage(t, testPrintingToolsDiskette)

	config := testConfig(t)
	config.Diskettes = []string{utilitiesOne}
	config.DiskFiles = []string{disk}
	m := buildTestMac(t, config)
	must(t, operator.New(m).WaitForApplication("Finder", 60))
	m.RunFrames(600)

	// Utilities 1, and HD SC Setup, which finds the blank disk
	must(t, operator.New(m).DoubleClickAt(472, 45))
	m.RunFrames(900)
	must(t, album.Screenshot(m, "utilities"))
	must(t, operator.New(m).DoubleClickAt(196, 100))
	m.RunFrames(1800)
	must(t, album.Screenshot(m, "hd-sc-setup"))

	// Initialize, confirmed, formatted and named
	must(t, operator.New(m).MoveTo(180, 121))
	operator.New(m).Click()
	m.RunFrames(300)
	must(t, album.Screenshot(m, "initialize"))
	must(t, operator.New(m).MoveTo(365, 194))
	operator.New(m).Click()
	m.RunFrames(600)
	must(t, album.Screenshot(m, "formatting"))
	m.RunFrames(6600)
	must(t, operator.New(m).TypeString("Macintosh HD"))
	m.RunFrames(30)
	must(t, album.Screenshot(m, "name"))
	must(t, operator.New(m).PressKey("Return"))
	m.RunFrames(1200)

	// HD SC Setup quit, and the disk on the desktop
	must(t, operator.New(m).MoveTo(180, 221))
	operator.New(m).Click()
	m.RunFrames(1200)
	must(t, album.Screenshot(m, "formatted"))

	// System Tools in the other drive, and its Installer
	if err := m.InsertDiskette(1, systemTools); err != nil {
		t.Fatal(err)
	}
	m.RunFrames(1200)
	must(t, operator.New(m).DoubleClickAt(472, 175))
	m.RunFrames(1200)
	must(t, album.Screenshot(m, "system-tools"))
	must(t, operator.New(m).DoubleClickAt(305, 100))
	m.RunFrames(2400)
	must(t, album.Screenshot(m, "installer"))
	must(t, operator.New(m).MoveTo(409, 296))
	operator.New(m).Click()
	m.RunFrames(1200)
	must(t, album.Screenshot(m, "easy-install"))

	// Install, recorded ten times faster, with the diskettes it asks for
	// given to it in the order it asks for them
	must(t, operator.New(m).MoveTo(424, 122))
	m.RunFrames(30)
	installing := Record(operator.New(m), "")
	installing.Capture(100)
	installing.Press(true)
	installing.Run(6, 2)
	installing.Press(false)
	must(t, operator.New(m).FeedDiskettes(24000, []string{utilitiesTwo, printingTools, systemTools},
		func() { installing.Capture(5) }))
	must(t, album.SaveRecording(installing, "installing", 300))

	// Quit, and Restart, which ejects the diskettes: the Macintosh starts
	// from the hard disk
	must(t, operator.New(m).MoveTo(378, 236))
	operator.New(m).Click()
	must(t, operator.New(m).WaitForApplication("Finder", 60))
	m.RunFrames(600)
	must(t, operator.New(m).ChooseFromMenu(185, 107))
	m.RunFrames(1200)
	must(t, operator.New(m).WaitForApplication("Finder", 120))
	m.RunFrames(1200)
	must(t, operator.New(m).DoubleClickAt(472, 50))
	m.RunFrames(1200)
	must(t, album.Screenshot(m, "started"))
}

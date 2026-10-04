package izmac

import (
	"os"
	"path/filepath"
	"testing"
)

/*
installingScreenshots is a blank hard disk made a startup disk: formatted
with HD SC Setup from the Utilities 1 diskette of System 6.0.8, System 6
installed on it by the Installer of System Tools with the diskettes it asks
for, and the machine restarted from it
*/
func installingScreenshots(t *testing.T) {
	const page = "installing"
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
	waitForApplication(t, m, "Finder", 60)
	m.RunFrames(600)

	// Utilities 1, and HD SC Setup, which finds the blank disk
	doubleClickAt(t, m, 472, 45)
	m.RunFrames(900)
	screenshot(t, m, page, "utilities")
	doubleClickAt(t, m, 196, 100)
	m.RunFrames(1800)
	screenshot(t, m, page, "hd-sc-setup")

	// Initialize, confirmed, formatted and named
	moveMouseTo(t, m, 180, 121)
	clickMouse(m)
	m.RunFrames(300)
	screenshot(t, m, page, "initialize")
	moveMouseTo(t, m, 365, 194)
	clickMouse(m)
	m.RunFrames(600)
	screenshot(t, m, page, "formatting")
	m.RunFrames(6600)
	typeString(m, "Macintosh HD")
	m.RunFrames(30)
	screenshot(t, m, page, "name")
	pressKey(m, "Return")
	m.RunFrames(1200)

	// HD SC Setup quit, and the disk on the desktop
	moveMouseTo(t, m, 180, 221)
	clickMouse(m)
	m.RunFrames(1200)
	screenshot(t, m, page, "formatted")

	// System Tools in the other drive, and its Installer
	if err := m.InsertDiskette(1, systemTools); err != nil {
		t.Fatal(err)
	}
	m.RunFrames(1200)
	doubleClickAt(t, m, 472, 175)
	m.RunFrames(1200)
	screenshot(t, m, page, "system-tools")
	doubleClickAt(t, m, 305, 100)
	m.RunFrames(2400)
	screenshot(t, m, page, "installer")
	moveMouseTo(t, m, 409, 296)
	clickMouse(m)
	m.RunFrames(1200)
	screenshot(t, m, page, "easy-install")

	// Install, recorded ten times faster, with the diskettes it asks for
	// given to it in the order it asks for them
	moveMouseTo(t, m, 424, 122)
	m.RunFrames(30)
	installing := record(m, "")
	installing.capture(100)
	installing.press(true)
	installing.run(6, 2)
	installing.press(false)
	feedDiskettes(t, m, installing, 24000, utilitiesTwo, printingTools, systemTools)
	installing.save(t, page, "installing", 300)

	// Quit, and Restart, which ejects the diskettes: the Macintosh starts
	// from the hard disk
	moveMouseTo(t, m, 378, 236)
	clickMouse(m)
	waitForApplication(t, m, "Finder", 60)
	m.RunFrames(600)
	chooseFromMenu(t, m, 185, 107)
	m.RunFrames(1200)
	waitForApplication(t, m, "Finder", 120)
	m.RunFrames(1200)
	doubleClickAt(t, m, 472, 50)
	m.RunFrames(1200)
	screenshot(t, m, page, "started")
}

/*
feedDiskettes runs the machine for some frames as someone at it with a pile of
diskettes would: whenever a drive is empty, the next diskette of the pile
goes in. The recording takes a picture every half second, shown for a
twentieth of a second, which is ten times faster than it happens.
*/
func feedDiskettes(t *testing.T, m *Mac, rec *recording, frames int, pile ...string) {
	t.Helper()
	for frame := 0; frame < frames; frame += 30 {
		m.RunFrames(30)
		rec.capture(5)
		for drive := 0; drive < 2 && len(pile) > 0; drive++ {
			if m.GetDiskette(drive).Image != "" {
				continue
			}
			m.RunFrames(60)
			if err := m.InsertDiskette(drive, pile[0]); err != nil {
				t.Fatal(err)
			}
			pile = pile[1:]
		}
	}
}

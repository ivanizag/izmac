package izmac

import (
	"testing"
)

/*
multiFinderScreenshots is System 6 with 4 MB, switched to MultiFinder with Set
Startup and restarted, TeachText opened beside the Finder and switched to and
from, the Apple menu with both, and the memory each takes
*/
func multiFinderScreenshots(t *testing.T) {
	const page = "multifinder"
	config := testConfig(t)
	config.DiskFiles = []string{testImage(t, testSystemSixDisk)}
	config.RamSizeKb = 4096
	m := buildTestMac(t, config)
	waitForApplication(t, m, "Finder", 60)
	m.RunFrames(600)

	// Set Startup, with MultiFinder chosen
	chooseFromMenu(t, m, 185, 75)
	m.RunFrames(300)
	moveMouseTo(t, m, 258, 106)
	clickMouse(m)
	m.RunFrames(60)
	screenshot(t, m, page, "set-startup")
	moveMouseTo(t, m, 357, 222)
	clickMouse(m)
	m.RunFrames(300)

	// Restart, and the Finder again under MultiFinder
	chooseFromMenu(t, m, 185, 107)
	m.RunFrames(1200)
	waitForApplication(t, m, "Finder", 60)
	m.RunFrames(900)
	screenshot(t, m, page, "restarted")

	// The Read Me in TeachText, its window made smaller so that the Finder
	// shows behind it
	moveMouseTo(t, m, 472, 50)
	clickMouse(m)
	pressCommand(m, "O")
	m.RunFrames(600)
	doubleClickAt(t, m, 237, 105)
	waitForApplication(t, m, "TeachText", 60)
	m.RunFrames(600)
	dragTo(t, m, 500, 330, 300, 190)
	m.RunFrames(300)

	// Switching, recorded: a click on the Finder's window, one on
	// TeachText's, and the Finder's again
	switching := record(m, "")
	switching.capture(150)
	for _, at := range [][2]int16{{400, 230}, {100, 30}, {400, 150}} {
		switching.glide(t, at[0], at[1])
		switching.press(true)
		switching.run(6, 2)
		switching.press(false)
		switching.run(90, 6)
		switching.capture(120)
	}
	switching.save(t, page, "switching", 100)

	// The Apple menu, from TeachText, with both
	moveMouseTo(t, m, 100, 30)
	clickMouse(m)
	m.RunFrames(120)
	openMenu(t, m, 16)
	screenshot(t, m, page, "apple-menu")
	closeMenu(t, m)

	// About the Finder, with the memory each takes
	moveMouseTo(t, m, 400, 230)
	clickMouse(m)
	m.RunFrames(120)
	chooseFromMenu(t, m, 16, 27)
	m.RunFrames(300)
	screenshot(t, m, page, "memory")
}

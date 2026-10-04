package izmac

import (
	"testing"
)

/*
resEditScreenshots is ResEdit 2.1, from its StuffIt archive, beside System 6:
TeachText opened in it, its menus, the Quit of its File menu renamed Goodbye,
saved, and TeachText opened to see it
*/
func resEditScreenshots(t *testing.T) {
	const page = "resedit"
	config := testConfig(t)
	config.RamSizeKb = 4096
	if err := config.AddFiles([]string{testImage(t, testSystemSixDisk), testImage(t, testResEditArchive)}); err != nil {
		t.Fatal(err)
	}
	m := buildTestMac(t, config)
	waitForApplication(t, m, "Finder", 60)
	m.RunFrames(900)
	screenshot(t, m, page, "diskette")

	// ResEdit, and the jack in the box it starts with
	doubleClickAt(t, m, 90, 265)
	waitForApplication(t, m, "ResEdit", 60)
	m.RunFrames(1200)
	screenshot(t, m, page, "resedit")

	// The file to open, TeachText on the hard disk
	moveMouseTo(t, m, 300, 200)
	clickMouse(m)
	m.RunFrames(600)
	moveMouseTo(t, m, 364, 153)
	clickMouse(m)
	m.RunFrames(300)
	moveMouseTo(t, m, 150, 161)
	clickMouse(m)
	m.RunFrames(60)
	screenshot(t, m, page, "open")
	moveMouseTo(t, m, 378, 220)
	clickMouse(m)
	m.RunFrames(1200)
	screenshot(t, m, page, "resources")

	// Its menus, with the Resource menu
	moveMouseTo(t, m, 145, 125)
	clickMouse(m)
	chooseFromMenu(t, m, 152, 43)
	m.RunFrames(900)
	screenshot(t, m, page, "menus")

	// The File menu in the menu editor
	moveMouseTo(t, m, 265, 170)
	clickMouse(m)
	chooseFromMenu(t, m, 152, 43)
	m.RunFrames(900)

	// Quit renamed Goodbye, recorded, and the File menu ResEdit shows to
	// try it pulled down
	moveMouseTo(t, m, 40, 298)
	clickMouse(m)
	m.RunFrames(300)
	pressCommand(m, "A")
	m.RunFrames(30)
	renaming := record(m, "")
	renaming.capture(100)
	for _, key := range []string{"Shift+G", "O", "O", "D", "B", "Y", "E"} {
		typeKeys(m, key)
		renaming.run(12, 6)
	}
	renaming.run(30, 6)
	renaming.glide(t, 360, 10)
	renaming.press(true)
	renaming.run(30, 6)
	renaming.save(t, page, "renaming", 300)
	moveMouseTo(t, m, 300, 300)
	m.SetMouseButton(false)
	m.RunFrames(30)

	// Saved, and ResEdit quit
	pressCommand(m, "S")
	m.RunFrames(600)
	pressCommand(m, "Q")
	waitForApplication(t, m, "Finder", 60)
	m.RunFrames(600)

	// TeachText, with its File menu as it is now
	moveMouseTo(t, m, 472, 50)
	clickMouse(m)
	pressCommand(m, "O")
	m.RunFrames(600)
	doubleClickAt(t, m, 110, 105)
	waitForApplication(t, m, "TeachText", 60)
	m.RunFrames(600)
	openMenu(t, m, 53)
	screenshot(t, m, page, "goodbye")
	closeMenu(t, m)
}

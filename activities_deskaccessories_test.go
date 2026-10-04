package izmac

import (
	"testing"
)

/*
deskAccessoriesScreenshots is System 6 with its desk accessories, each opened
from the Apple menu, the Calculator used over a document of TeachText, and
the Clipboard carrying a number into the document, a picture of the
Scrapbook to the Finder, and text from the host
*/
func deskAccessoriesScreenshots(t *testing.T) {
	const page = "desk-accessories"
	config := testConfig(t)
	config.DiskFiles = []string{testImage(t, testSystemSixDisk)}
	m := buildTestMac(t, config)
	waitForApplication(t, m, "Finder", 60)
	m.RunFrames(600)

	// The Apple menu, with the desk accessories under About the Finder
	moveMouseTo(t, m, 16, 10)
	m.RunFrames(30)
	menu := record(m, "")
	menu.capture(50)
	menu.press(true)
	menu.run(20, 2)
	menu.glide(t, 40, 30)
	menu.glide(t, 40, 150)
	menu.run(20, 2)
	menu.save(t, page, "apple-menu", 200)
	moveMouseTo(t, m, 300, 250)
	m.SetMouseButton(false)
	m.RunFrames(30)

	// The Alarm Clock, opened out with its flag, and closed
	chooseFromMenu(t, m, 16, 59)
	m.RunFrames(120)
	moveMouseTo(t, m, 440, 40)
	clickMouse(m)
	m.RunFrames(120)
	screenshot(t, m, page, "alarm-clock")
	moveMouseTo(t, m, 336, 41)
	clickMouse(m)
	m.RunFrames(60)

	// The Calculator, typed into, and closed
	chooseFromMenu(t, m, 16, 75)
	m.RunFrames(120)
	calculator := record(m, "")
	calculator.capture(100)
	for _, key := range []string{"2", "5", "9", "9", "Shift+8", "3"} {
		typeKeys(m, key)
		calculator.run(12, 6)
	}
	typeKeys(m, "Equal")
	calculator.run(60, 6)
	calculator.save(t, page, "calculator", 300)
	closeBox(t, m, 217, 68)

	// Key Caps, with a word typed and then the option key held down
	chooseFromMenu(t, m, 16, 139)
	m.RunFrames(120)
	typeString(m, "hello")
	m.RunFrames(30)
	codes := KeyCodes()
	keyCaps := record(m, "")
	keyCaps.capture(150)
	m.PutKey(codes["Option"], true)
	keyCaps.run(12, 6)
	keyCaps.save(t, page, "key-caps", 200)
	m.PutKey(codes["Option"], false)
	m.RunFrames(30)
	closeBox(t, m, 74, 50)

	// The Control Panel
	chooseFromMenu(t, m, 16, 107)
	m.RunFrames(900)
	screenshot(t, m, page, "control-panel")
	closeBox(t, m, 108, 42)

	// The Scrapbook, gone through to its last page, whose picture is copied
	chooseFromMenu(t, m, 16, 155)
	m.RunFrames(300)
	scrapbook := record(m, "")
	scrapbook.capture(150)
	for i := 0; i < 4; i++ {
		moveMouseTo(t, m, 428, 267)
		clickMouse(m)
		scrapbook.run(60, 6)
		scrapbook.capture(120)
	}
	scrapbook.save(t, page, "scrapbook", 100)
	pressCommand(m, "C")
	m.RunFrames(60)
	closeBox(t, m, 77, 41)

	// The picture on the Clipboard, as the Finder shows it
	chooseFromMenu(t, m, 90, 155)
	m.RunFrames(300)
	screenshot(t, m, page, "clipboard")
	closeBox(t, m, 14, 237)

	// The Read Me opened in TeachText
	moveMouseTo(t, m, 472, 50)
	clickMouse(m)
	pressCommand(m, "O")
	m.RunFrames(600)
	doubleClickAt(t, m, 237, 105)
	waitForApplication(t, m, "TeachText", 60)
	m.RunFrames(300)

	// The Calculator over it, its answer copied, and pasted into the
	// document on a line of its own
	chooseFromMenu(t, m, 16, 75)
	m.RunFrames(120)
	typeString(m, "2599*3=")
	m.RunFrames(60)
	screenshot(t, m, page, "over-teachtext")
	pressCommand(m, "C")
	m.RunFrames(60)
	moveMouseTo(t, m, 217, 68)
	clickMouse(m)
	m.RunFrames(120)
	moveMouseTo(t, m, 400, 49)
	clickMouse(m)
	pressKey(m, "Return")
	typeString(m, "Three Macintosh Plus at $2,599: $")
	pressCommand(m, "V")
	m.RunFrames(120)
	screenshot(t, m, page, "pasted")

	// Text from the host, back in the Finder, on the Clipboard, TeachText
	// quit without saving
	pressCommand(m, "Q")
	m.RunFrames(120)
	moveMouseTo(t, m, 150, 219)
	clickMouse(m)
	waitForApplication(t, m, "Finder", 60)
	m.RunFrames(300)
	m.startPaste("Copied on the computer izmac runs on.")
	m.RunFrames(pasteFrames)
	chooseFromMenu(t, m, 90, 155)
	m.RunFrames(300)
	screenshot(t, m, page, "from-the-host")
}

// closeBox clicks the box at the top left of a window, which is how a desk
// accessory is closed in the Finder of System 6
func closeBox(t *testing.T, m *Mac, h, v int16) {
	t.Helper()
	moveMouseTo(t, m, h, v)
	clickMouse(m)
	m.RunFrames(60)
}

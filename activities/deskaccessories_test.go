package activities

import (
	"path/filepath"
	"testing"

	"github.com/ivanizag/izmac"
	"github.com/ivanizag/izmac/operator"
)

/*
deskAccessoriesScreenshots is System 6 with its desk accessories, each opened
from the Apple menu, the Calculator used over a document of TeachText, and
the Clipboard carrying a number into the document, a picture of the
Scrapbook to the Finder, and text from the host
*/
func deskAccessoriesScreenshots(t *testing.T) {
	const page = "desk-accessories"
	album := NewAlbum(filepath.Join(activityImages, page))
	config := testConfig(t)
	config.DiskFiles = []string{testImage(t, testSystemSixDisk)}
	m := buildTestMac(t, config)
	must(t, operator.New(m).WaitForApplication("Finder", 60))
	m.RunFrames(600)

	// The Apple menu, with the desk accessories under About the Finder
	must(t, operator.New(m).MoveTo(16, 10))
	m.RunFrames(30)
	menu := Record(operator.New(m), "")
	menu.Capture(50)
	menu.Press(true)
	menu.Run(20, 2)
	must(t, menu.Glide(40, 30))
	must(t, menu.Glide(40, 150))
	menu.Run(20, 2)
	must(t, album.SaveRecording(menu, "apple-menu", 200))
	must(t, operator.New(m).MoveTo(300, 250))
	m.SetMouseButton(false)
	m.RunFrames(30)

	// The Alarm Clock, opened out with its flag, and closed
	must(t, operator.New(m).ChooseFromMenu(16, 59))
	m.RunFrames(120)
	must(t, operator.New(m).MoveTo(440, 40))
	operator.New(m).Click()
	m.RunFrames(120)
	must(t, album.Screenshot(m, "alarm-clock"))
	must(t, operator.New(m).MoveTo(336, 41))
	operator.New(m).Click()
	m.RunFrames(60)

	// The Calculator, typed into, and closed
	must(t, operator.New(m).ChooseFromMenu(16, 75))
	m.RunFrames(120)
	calculator := Record(operator.New(m), "")
	calculator.Capture(100)
	for _, key := range []string{"2", "5", "9", "9", "Shift+8", "3"} {
		must(t, operator.New(m).TypeKeys(key))
		calculator.Run(12, 6)
	}
	must(t, operator.New(m).TypeKeys("Equal"))
	calculator.Run(60, 6)
	must(t, album.SaveRecording(calculator, "calculator", 300))
	closeBox(t, m, 217, 68)

	// Key Caps, with a word typed and then the option key held down
	must(t, operator.New(m).ChooseFromMenu(16, 139))
	m.RunFrames(120)
	must(t, operator.New(m).TypeString("hello"))
	m.RunFrames(30)
	codes := izmac.KeyCodes()
	keyCaps := Record(operator.New(m), "")
	keyCaps.Capture(150)
	m.PutKey(codes["Option"], true)
	keyCaps.Run(12, 6)
	must(t, album.SaveRecording(keyCaps, "key-caps", 200))
	m.PutKey(codes["Option"], false)
	m.RunFrames(30)
	closeBox(t, m, 74, 50)

	// The Control Panel
	must(t, operator.New(m).ChooseFromMenu(16, 107))
	m.RunFrames(900)
	must(t, album.Screenshot(m, "control-panel"))
	closeBox(t, m, 108, 42)

	// The Scrapbook, gone through to its last page, whose picture is copied
	must(t, operator.New(m).ChooseFromMenu(16, 155))
	m.RunFrames(300)
	scrapbook := Record(operator.New(m), "")
	scrapbook.Capture(150)
	for i := 0; i < 4; i++ {
		must(t, operator.New(m).MoveTo(428, 267))
		operator.New(m).Click()
		scrapbook.Run(60, 6)
		scrapbook.Capture(120)
	}
	must(t, album.SaveRecording(scrapbook, "scrapbook", 100))
	must(t, operator.New(m).Command("C"))
	m.RunFrames(60)
	closeBox(t, m, 77, 41)

	// The picture on the Clipboard, as the Finder shows it
	must(t, operator.New(m).ChooseFromMenu(90, 155))
	m.RunFrames(300)
	must(t, album.Screenshot(m, "clipboard"))
	closeBox(t, m, 14, 237)

	// The Read Me opened in TeachText
	must(t, operator.New(m).MoveTo(472, 50))
	operator.New(m).Click()
	must(t, operator.New(m).Command("O"))
	m.RunFrames(600)
	must(t, operator.New(m).DoubleClickAt(237, 105))
	must(t, operator.New(m).WaitForApplication("TeachText", 60))
	m.RunFrames(300)

	// The Calculator over it, its answer copied, and pasted into the
	// document on a line of its own
	must(t, operator.New(m).ChooseFromMenu(16, 75))
	m.RunFrames(120)
	must(t, operator.New(m).TypeString("2599*3="))
	m.RunFrames(60)
	must(t, album.Screenshot(m, "over-teachtext"))
	must(t, operator.New(m).Command("C"))
	m.RunFrames(60)
	must(t, operator.New(m).MoveTo(217, 68))
	operator.New(m).Click()
	m.RunFrames(120)
	must(t, operator.New(m).MoveTo(400, 49))
	operator.New(m).Click()
	must(t, operator.New(m).PressKey("Return"))
	must(t, operator.New(m).TypeString("Three Macintosh Plus at $2,599: $"))
	must(t, operator.New(m).Command("V"))
	m.RunFrames(120)
	must(t, album.Screenshot(m, "pasted"))

	// Text from the host, back in the Finder, on the Clipboard, TeachText
	// quit without saving
	must(t, operator.New(m).Command("Q"))
	m.RunFrames(120)
	must(t, operator.New(m).MoveTo(150, 219))
	operator.New(m).Click()
	must(t, operator.New(m).WaitForApplication("Finder", 60))
	m.RunFrames(300)
	must(t, operator.New(m).Paste("Copied on the computer izmac runs on."))
	must(t, operator.New(m).ChooseFromMenu(90, 155))
	m.RunFrames(300)
	must(t, album.Screenshot(m, "from-the-host"))
}

// closeBox clicks the box at the top left of a window, which is how a desk
// accessory is closed in the Finder of System 6
func closeBox(t *testing.T, m *izmac.Mac, h, v int) {
	t.Helper()
	must(t, operator.New(m).MoveTo(h, v))
	operator.New(m).Click()
	m.RunFrames(60)
}

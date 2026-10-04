package activities

import (
	"path/filepath"
	"testing"

	"github.com/ivanizag/izmac/operator"
)

/*
multiFinderScreenshots is System 6 with 4 MB, switched to MultiFinder with Set
Startup and restarted, TeachText opened beside the Finder and switched to and
from, the Apple menu with both, and the memory each takes
*/
func multiFinderScreenshots(t *testing.T) {
	const page = "multifinder"
	album := NewAlbum(filepath.Join(activityImages, page))
	config := testConfig(t)
	config.DiskFiles = []string{testImage(t, testSystemSixDisk)}
	config.RamSizeKb = 4096
	m := buildTestMac(t, config)
	must(t, operator.New(m).WaitForApplication("Finder", 60))
	m.RunFrames(600)

	// Set Startup, with MultiFinder chosen
	must(t, operator.New(m).ChooseFromMenu(185, 75))
	m.RunFrames(300)
	must(t, operator.New(m).MoveTo(258, 106))
	operator.New(m).Click()
	m.RunFrames(60)
	must(t, album.Screenshot(m, "set-startup"))
	must(t, operator.New(m).MoveTo(357, 222))
	operator.New(m).Click()
	m.RunFrames(300)

	// Restart, and the Finder again under MultiFinder
	must(t, operator.New(m).ChooseFromMenu(185, 107))
	m.RunFrames(1200)
	must(t, operator.New(m).WaitForApplication("Finder", 60))
	m.RunFrames(900)
	must(t, album.Screenshot(m, "restarted"))

	// The Read Me in TeachText, its window made smaller so that the Finder
	// shows behind it
	must(t, operator.New(m).MoveTo(472, 50))
	operator.New(m).Click()
	must(t, operator.New(m).Command("O"))
	m.RunFrames(600)
	must(t, operator.New(m).DoubleClickAt(237, 105))
	must(t, operator.New(m).WaitForApplication("TeachText", 60))
	m.RunFrames(600)
	must(t, operator.New(m).Drag(500, 330, 300, 190))
	m.RunFrames(300)

	// Switching, recorded: a click on the Finder's window, one on
	// TeachText's, and the Finder's again
	switching := Record(operator.New(m), "")
	switching.Capture(150)
	for _, at := range [][2]int{{400, 230}, {100, 30}, {400, 150}} {
		must(t, switching.Glide(at[0], at[1]))
		switching.Press(true)
		switching.Run(6, 2)
		switching.Press(false)
		switching.Run(90, 6)
		switching.Capture(120)
	}
	must(t, album.SaveRecording(switching, "switching", 100))

	// The Apple menu, from TeachText, with both
	must(t, operator.New(m).MoveTo(100, 30))
	operator.New(m).Click()
	m.RunFrames(120)
	must(t, operator.New(m).OpenMenu(16))
	must(t, album.Screenshot(m, "apple-menu"))
	must(t, operator.New(m).CloseMenu())

	// About the Finder, with the memory each takes
	must(t, operator.New(m).MoveTo(400, 230))
	operator.New(m).Click()
	m.RunFrames(120)
	must(t, operator.New(m).ChooseFromMenu(16, 27))
	m.RunFrames(300)
	must(t, album.Screenshot(m, "memory"))
}

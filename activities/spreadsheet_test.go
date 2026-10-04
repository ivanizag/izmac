package activities

import (
	"path/filepath"
	"testing"

	"github.com/ivanizag/izmac/operator"
)

/*
spreadsheetScreenshots is a budget in Multiplan 1.11: the things an office
bought for a Macintosh in 1986, their prices in dollars and a total that is a
formula, recalculated when a line is added, and the formulas shown
*/
func spreadsheetScreenshots(t *testing.T) {
	const page = "spreadsheet"
	album := NewAlbum(filepath.Join(activityImages, page))
	config := testConfig(t)
	config.Diskettes = []string{testImage(t, testMultiplanDiskette)}
	m := buildTestMac(t, config)
	must(t, operator.New(m).WaitForApplication("Finder", 60))
	m.RunFrames(600)

	// The diskette, and Multiplan with a new worksheet
	must(t, operator.New(m).DoubleClickAt(472, 45))
	m.RunFrames(900)
	must(t, album.Screenshot(m, "diskette"))
	must(t, operator.New(m).DoubleClickAt(80, 112))
	must(t, operator.New(m).WaitForApplication("Multiplan", 60))
	m.RunFrames(900)
	must(t, album.Screenshot(m, "worksheet"))

	// What was bought, typed down the first column, and the prices down the
	// second, each cell ended with Return, which goes to the one below
	cell := func(h, v int) {
		must(t, operator.New(m).MoveTo(h, v))
		operator.New(m).Click()
		m.RunFrames(30)
	}
	enter := func(texts ...string) {
		for _, text := range texts {
			must(t, operator.New(m).TypeString(text))
			must(t, operator.New(m).PressKey("Return"))
			m.RunFrames(30)
		}
	}
	cell(60, 88)
	enter("A Macintosh office, 1986")
	cell(60, 112)
	enter("Macintosh Plus", "ImageWriter II", "External drive")
	cell(60, 160)
	enter("Total")
	cell(130, 112)
	enter("2599", "595", "499")
	cell(130, 160)
	enter("=SUM(R3C2:R6C2)")

	// The prices in dollars, the second column chosen by its number, and
	// the first column made wide enough for its names
	cell(132, 76)
	must(t, operator.New(m).ChooseFromMenu(192, 43))
	m.RunFrames(60)
	cell(68, 76)
	must(t, operator.New(m).ChooseFromMenu(192, 283))
	m.RunFrames(300)
	must(t, operator.New(m).TypeString("18"))
	must(t, operator.New(m).PressKey("Return"))
	m.RunFrames(300)
	cell(60, 100)
	must(t, album.Screenshot(m, "budget"))

	// A hard disk added on the empty line, recorded: the total follows
	cell(60, 148)
	adding := Record(operator.New(m), "")
	adding.Capture(100)
	for _, key := range []string{"Shift+H", "Shift+D", "Space", "2", "0", "Tab", "1", "4", "9", "5", "Return"} {
		must(t, operator.New(m).TypeKeys(key))
		adding.Run(12, 6)
	}
	adding.Run(60, 6)
	must(t, album.SaveRecording(adding, "recalculating", 300))

	// The formulas instead of what they come to
	must(t, operator.New(m).ChooseFromMenu(228, 123))
	m.RunFrames(300)
	must(t, album.Screenshot(m, "formulas"))
	must(t, operator.New(m).ChooseFromMenu(228, 139))
	m.RunFrames(300)
}

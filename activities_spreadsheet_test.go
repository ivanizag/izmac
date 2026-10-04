package izmac

import (
	"testing"
)

/*
spreadsheetScreenshots is a budget in Multiplan 1.11: the things an office
bought for a Macintosh in 1986, their prices in dollars and a total that is a
formula, recalculated when a line is added, and the formulas shown
*/
func spreadsheetScreenshots(t *testing.T) {
	const page = "spreadsheet"
	config := testConfig(t)
	config.Diskettes = []string{testImage(t, testMultiplanDiskette)}
	m := buildTestMac(t, config)
	waitForApplication(t, m, "Finder", 60)
	m.RunFrames(600)

	// The diskette, and Multiplan with a new worksheet
	doubleClickAt(t, m, 472, 45)
	m.RunFrames(900)
	screenshot(t, m, page, "diskette")
	doubleClickAt(t, m, 80, 112)
	waitForApplication(t, m, "Multiplan", 60)
	m.RunFrames(900)
	screenshot(t, m, page, "worksheet")

	// What was bought, typed down the first column, and the prices down the
	// second, each cell ended with Return, which goes to the one below
	cell := func(h, v int16) {
		moveMouseTo(t, m, h, v)
		clickMouse(m)
		m.RunFrames(30)
	}
	enter := func(texts ...string) {
		for _, text := range texts {
			typeString(m, text)
			pressKey(m, "Return")
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
	chooseFromMenu(t, m, 192, 43)
	m.RunFrames(60)
	cell(68, 76)
	chooseFromMenu(t, m, 192, 283)
	m.RunFrames(300)
	typeString(m, "18")
	pressKey(m, "Return")
	m.RunFrames(300)
	cell(60, 100)
	screenshot(t, m, page, "budget")

	// A hard disk added on the empty line, recorded: the total follows
	cell(60, 148)
	adding := record(m, "")
	adding.capture(100)
	for _, key := range []string{"Shift+H", "Shift+D", "Space", "2", "0", "Tab", "1", "4", "9", "5", "Return"} {
		typeKeys(m, key)
		adding.run(12, 6)
	}
	adding.run(60, 6)
	adding.save(t, page, "recalculating", 300)

	// The formulas instead of what they come to
	chooseFromMenu(t, m, 228, 123)
	m.RunFrames(300)
	screenshot(t, m, page, "formulas")
	chooseFromMenu(t, m, 228, 139)
	m.RunFrames(300)
}

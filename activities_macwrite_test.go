package izmac

import (
	"path/filepath"
	"testing"
)

/*
macWriteScreenshots is a letter written in MacWrite 4.5: typed, its heading
tried in the fonts of the System and centered, the body set in New York, and
the letter printed on the ImageWriter and saved on the diskette
*/
func macWriteScreenshots(t *testing.T) {
	const page = "macwrite"
	config := testConfig(t)
	config.Diskettes = []string{testImage(t, testMacWriteDiskette)}
	config.Printer = printerImageWriter
	config.PrinterFile = filepath.Join(t.TempDir(), "page")
	m := buildTestMac(t, config)
	waitForApplication(t, m, "Finder", 60)
	m.RunFrames(600)

	// The diskette, and MacWrite with a new document
	doubleClickAt(t, m, 472, 45)
	m.RunFrames(900)
	screenshot(t, m, page, "diskette")
	doubleClickAt(t, m, 350, 150)
	waitForApplication(t, m, "MacWrite", 60)
	m.RunFrames(900)
	screenshot(t, m, page, "untitled")

	// The letter typed
	typeString(m, "A letter from 1986\n\n")
	typeString(m, "Dear reader of the future,\n\n")
	typeString(m, "This letter was written on a Macintosh Plus, in MacWrite, "+
		"and what you see on the screen is what comes out of the printer: "+
		"the fonts, the sizes and the styles, as they are here.\n\n")
	typeString(m, "Yours, from 1986")
	m.RunFrames(120)
	screenshot(t, m, page, "typed")

	// The heading selected, and tried in the fonts of the Font menu, the
	// last one kept, at 24 points and centered
	dragOn(t, m, 8, 97, 300, 97)
	fonts := record(m, "")
	fonts.capture(100)
	for _, item := range []int16{27, 75, 91, 107, 123} {
		chooseFromMenu(t, m, 247, item)
		fonts.run(60, 6)
		fonts.capture(80)
	}
	chooseFromMenu(t, m, 291, 251)
	fonts.run(60, 6)
	pressCommand(m, "M")
	fonts.run(60, 6)
	fonts.save(t, page, "fonts", 200)

	// The body in New York
	dragOn(t, m, 8, 140, 300, 300)
	chooseFromMenu(t, m, 247, 107)
	m.RunFrames(120)
	moveMouseTo(t, m, 400, 320)
	clickMouse(m)
	m.RunFrames(60)
	screenshot(t, m, page, "styled")

	// Print, its dialog, and the page the ImageWriter printed
	chooseFromMenu(t, m, 53, 123)
	m.RunFrames(600)
	moveMouseTo(t, m, 124, 82)
	clickMouse(m)
	m.RunFrames(60)
	screenshot(t, m, page, "print")
	moveMouseTo(t, m, 450, 63)
	clickMouse(m)
	printed := config.PrinterFile + "_001.png"
	for second := 0; !exists(printed); second++ {
		if second == 600 {
			t.Fatalf("MacWrite printed nothing")
		}
		m.RunFrames(60)
	}
	m.RunFrames(600)
	keepPrintedPage(t, printed, page)

	// Saved on the diskette as Letter
	chooseFromMenu(t, m, 53, 91)
	m.RunFrames(600)
	typeString(m, "Letter")
	m.RunFrames(60)
	screenshot(t, m, page, "save-as")
	pressKey(m, "Return")
	m.RunFrames(900)
}

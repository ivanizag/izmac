package activities

import (
	"path/filepath"
	"testing"

	"github.com/ivanizag/izmac"
	"github.com/ivanizag/izmac/operator"
)

/*
macWriteScreenshots is a letter written in MacWrite 4.5: typed, its heading
tried in the fonts of the System and centered, the body set in New York, and
the letter printed on the ImageWriter and saved on the diskette
*/
func macWriteScreenshots(t *testing.T) {
	const page = "macwrite"
	album := NewAlbum(filepath.Join(activityImages, page))
	config := testConfig(t)
	config.Diskettes = []string{testImage(t, testMacWriteDiskette)}
	config.Printer = izmac.PrinterImageWriter
	config.PrinterFile = filepath.Join(t.TempDir(), "page")
	m := buildTestMac(t, config)
	must(t, operator.New(m).WaitForApplication("Finder", 60))
	m.RunFrames(600)

	// The diskette, and MacWrite with a new document
	must(t, operator.New(m).DoubleClickAt(472, 45))
	m.RunFrames(900)
	must(t, album.Screenshot(m, "diskette"))
	must(t, operator.New(m).DoubleClickAt(350, 150))
	must(t, operator.New(m).WaitForApplication("MacWrite", 60))
	m.RunFrames(900)
	must(t, album.Screenshot(m, "untitled"))

	// The letter typed
	must(t, operator.New(m).TypeString("A letter from 1986\n\n"))
	must(t, operator.New(m).TypeString("Dear reader of the future,\n\n"))
	must(t, operator.New(m).TypeString("This letter was written on a Macintosh Plus, in MacWrite, "+
		"and what you see on the screen is what comes out of the printer: "+
		"the fonts, the sizes and the styles, as they are here.\n\n"))
	must(t, operator.New(m).TypeString("Yours, from 1986"))
	m.RunFrames(120)
	must(t, album.Screenshot(m, "typed"))

	// The heading selected, and tried in the fonts of the Font menu, the
	// last one kept, at 24 points and centered
	dragOn(t, m, 8, 97, 300, 97)
	fonts := Record(operator.New(m), "")
	fonts.Capture(100)
	for _, item := range []int{27, 75, 91, 107, 123} {
		must(t, operator.New(m).ChooseFromMenu(247, item))
		fonts.Run(60, 6)
		fonts.Capture(80)
	}
	must(t, operator.New(m).ChooseFromMenu(291, 251))
	fonts.Run(60, 6)
	must(t, operator.New(m).Command("M"))
	fonts.Run(60, 6)
	must(t, album.SaveRecording(fonts, "fonts", 200))

	// The body in New York
	dragOn(t, m, 8, 140, 300, 300)
	must(t, operator.New(m).ChooseFromMenu(247, 107))
	m.RunFrames(120)
	must(t, operator.New(m).MoveTo(400, 320))
	operator.New(m).Click()
	m.RunFrames(60)
	must(t, album.Screenshot(m, "styled"))

	// Print, its dialog, and the page the ImageWriter printed
	must(t, operator.New(m).ChooseFromMenu(53, 123))
	m.RunFrames(600)
	must(t, operator.New(m).MoveTo(124, 82))
	operator.New(m).Click()
	m.RunFrames(60)
	must(t, album.Screenshot(m, "print"))
	must(t, operator.New(m).MoveTo(450, 63))
	operator.New(m).Click()
	printed := config.PrinterFile + "_001.png"
	for second := 0; !exists(printed); second++ {
		if second == 600 {
			t.Fatalf("MacWrite printed nothing")
		}
		m.RunFrames(60)
	}
	m.RunFrames(600)
	must(t, album.KeepPrintedPage(printed, "printed-page"))

	// Saved on the diskette as Letter
	must(t, operator.New(m).ChooseFromMenu(53, 91))
	m.RunFrames(600)
	must(t, operator.New(m).TypeString("Letter"))
	m.RunFrames(60)
	must(t, album.Screenshot(m, "save-as"))
	must(t, operator.New(m).PressKey("Return"))
	m.RunFrames(900)
}

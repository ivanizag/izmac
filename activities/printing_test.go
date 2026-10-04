package activities

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/ivanizag/izmac"
	"github.com/ivanizag/izmac/operator"
)

/*
printingScreenshots is the ImageWriter from MacWrite 4.5: chosen with Choose
Printer, the page set up, and the Sample Memo printed in draft, standard and
high quality, the three pages compared side by side
*/
func printingScreenshots(t *testing.T) {
	const page = "printing"
	album := NewAlbum(filepath.Join(activityImages, page))
	config := testConfig(t)
	config.Diskettes = []string{testImage(t, testMacWriteDiskette)}
	config.Printer = izmac.PrinterImageWriter
	config.PrinterFile = filepath.Join(t.TempDir(), "page")
	m := buildTestMac(t, config)
	must(t, operator.New(m).WaitForApplication("Finder", 60))
	m.RunFrames(600)

	// Choose Printer, with the ImageWriter on the printer port
	must(t, operator.New(m).ChooseFromMenu(16, 91))
	m.RunFrames(900)
	must(t, album.Screenshot(m, "choose-printer"))
	must(t, operator.New(m).MoveTo(205, 207))
	operator.New(m).Click()
	m.RunFrames(300)

	// The Sample Memo, opened in MacWrite
	must(t, operator.New(m).DoubleClickAt(472, 45))
	m.RunFrames(900)
	must(t, operator.New(m).DoubleClickAt(285, 200))
	must(t, operator.New(m).WaitForApplication("MacWrite", 60))
	m.RunFrames(1200)
	must(t, album.Screenshot(m, "memo"))

	// Page Setup
	must(t, operator.New(m).ChooseFromMenu(53, 107))
	m.RunFrames(600)
	must(t, album.Screenshot(m, "page-setup"))
	must(t, operator.New(m).PressKey("Return"))
	m.RunFrames(300)

	// Printed three times, in draft, standard and high quality
	var printed []string
	for _, quality := range []struct {
		name string
		h    int
	}{{"draft", 342}, {"standard", 242}, {"high", 124}} {
		must(t, operator.New(m).ChooseFromMenu(53, 123))
		m.RunFrames(600)
		must(t, operator.New(m).MoveTo(quality.h, 82))
		operator.New(m).Click()
		m.RunFrames(60)
		if quality.name == "draft" {
			must(t, album.Screenshot(m, "print"))
		}
		must(t, operator.New(m).MoveTo(450, 63))
		operator.New(m).Click()
		printed = append(printed, waitForPages(t, m, config.PrinterFile, len(printed))[0])
	}

	// The same corner of each page, side by side
	compareQualities(t, printed, []string{"Draft", "Standard", "High"},
		filepath.Join(activityImages, page, "qualities.png"))
	must(t, album.KeepPrintedPage(printed[2], "printed-page"))
}

/*
waitForPages runs the machine until the printer has finished what it was
given, which is when no new page has come out for twenty seconds, and returns
the pages that came out, given how many there were before
*/
func waitForPages(t *testing.T, m *izmac.Mac, prefix string, before int) []string {
	t.Helper()
	pages := func() []string {
		found, _ := filepath.Glob(prefix + "_*.png")
		return found
	}
	quiet := 0
	for second := 0; quiet < 20 || len(pages()) == before; second++ {
		if second == 900 {
			t.Fatalf("the printer printed nothing")
		}
		count := len(pages())
		m.RunFrames(60)
		if len(pages()) == count {
			quiet++
		} else {
			quiet = 0
		}
	}
	return pages()[before:]
}

// compareQualities puts the top left of each page side by side, with its
// name over it, in one image
func compareQualities(t *testing.T, pages []string, names []string, out string) {
	t.Helper()
	const width, height, gap, title = 520, 300, 16, 24
	crop := image.Rect(150, 260, 150+width, 260+height)
	sheet := image.NewRGBA(image.Rect(0, 0, len(pages)*(width+gap)-gap, height+title))
	draw.Draw(sheet, sheet.Bounds(), image.White, image.Point{}, draw.Src)
	for i, name := range pages {
		f, err := os.Open(name)
		if err != nil {
			t.Fatal(err)
		}
		picture, err := png.Decode(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		left := i * (width + gap)
		draw.Draw(sheet, image.Rect(left, title, left+width, title+height), picture, crop.Min, draw.Src)
		writer := font.Drawer{
			Dst:  sheet,
			Src:  image.NewUniform(color.Black),
			Face: basicfont.Face7x13,
			Dot:  fixed.P(left+4, 16),
		}
		writer.DrawString(strings.ToUpper(names[i][:1]) + names[i][1:])
	}
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, sheet); err != nil {
		t.Fatal(err)
	}
}

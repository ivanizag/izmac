package e2e_tests

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/ivanizag/izmac"
)

/*
Printing from an application, the whole way: the ImageWriter chosen in the
Chooser, a document opened in TeachText and printed with Apple's own driver,
which talks to the printer on the serial port, and a page of the ImageWriter
izmac emulates written on the host. What says it worked is the page: ink on
the line the text of the document is on, and the rest of it as blank as the
document is.
*/
func TestTeachTextPrintsAPage(t *testing.T) {
	t.Parallel()
	config := realConfig(t)
	config.Printer = izmac.PrinterImageWriter
	config.PrinterFile = filepath.Join(t.TempDir(), "page")
	m := buildTestMac(t, config)
	waitForApplication(t, m, "Finder", 60)
	m.RunFrames(300)

	// The ImageWriter in the Chooser, which picks the printer port itself
	openChooser(t, m)
	moveMouseTo(t, m, 100, 88)
	clickMouse(m)
	m.RunFrames(600)
	moveMouseTo(t, m, 71, 47)
	clickMouse(m)
	m.RunFrames(600)

	// Read Me in TeachText, and Print from its File menu, OK
	pressCommand(m, "O")
	m.RunFrames(300)
	doubleClickAt(t, m, 236, 110)
	waitForApplication(t, m, "TeachText", 20)
	m.RunFrames(600)
	moveMouseTo(t, m, 55, 10)
	m.SetMouseButton(true)
	m.RunFrames(30)
	moveMouseTo(t, m, 70, 155)
	m.RunFrames(10)
	m.SetMouseButton(false)
	m.RunFrames(600)
	pressKey(m, "Return")

	page := config.PrinterFile + "_001.png"
	if !waitUntil(m, 120, func() bool { _, err := os.Stat(page); return err == nil }) {
		t.Fatalf("no page was printed")
	}

	f, err := os.Open(page)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	bounds := img.Bounds()
	textLine := image.Rect(bounds.Min.X, bounds.Min.Y, bounds.Max.X, bounds.Min.Y+bounds.Dy()/20)
	if inked := ink(img, textLine); inked < 500 {
		t.Errorf("the line of the text has %v dots of ink, it was not printed", inked)
	}
	middle := image.Rect(bounds.Min.X, bounds.Min.Y+bounds.Dy()/4, bounds.Max.X, bounds.Min.Y+bounds.Dy()*3/4)
	if inked := ink(img, middle); inked != 0 {
		t.Errorf("the middle of the page, blank in the document, has %v dots of ink", inked)
	}
}

// ink counts the dark dots of a part of a page
func ink(img image.Image, area image.Rectangle) int {
	inked := 0
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			if r, g, b, _ := img.At(x, y).RGBA(); r+g+b < 3*0x8000 {
				inked++
			}
		}
	}
	return inked
}

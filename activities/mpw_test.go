package activities

import (
	"bytes"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ivanizag/izmac"
	"github.com/ivanizag/izmac/operator"
)

/*
mpwScreenshots is a Mandelbrot set explorer written in C and built with MPW
3.1: MPW copied to the hard disk from its CD, the program of the page put in
the shared folder with the line ends of the Macintosh, compiled and linked
there from the Worksheet, run, and zoomed into twice
*/
func mpwScreenshots(t *testing.T) {
	const page = "mpw"
	album := NewAlbum(filepath.Join(activityImages, page))
	share := filepath.Join(t.TempDir(), "For the Mac")
	if err := os.Mkdir(share, 0o755); err != nil {
		t.Fatal(err)
	}
	source := strings.ReplaceAll(pageListing(t, page, "c"), "\n", "\r")
	if err := os.WriteFile(filepath.Join(share, "Mandelbrot.c"), []uint8(source), 0o644); err != nil {
		t.Fatal(err)
	}
	predate(t, filepath.Join(share, "Mandelbrot.c"), share)

	config := testConfig(t)
	config.Diskettes = []string{testImage(t, testSystemSixDiskette)}
	config.DiskFiles = []string{hardDisk(t), testImage(t, testMPWArchive)}
	config.RamSizeKb = 4096
	config.Share = share
	config.ShareName = "my-computer"
	config.PrinterPort = ""
	m := buildTestMac(t, config)
	op := operator.New(m)
	must(t, op.WaitForApplication("Finder", 60))
	m.RunFrames(1200)
	must(t, album.Screenshot(m, "desktop"))

	// The CD
	must(t, op.DoubleClickAt(470, 180))
	m.RunFrames(900)
	must(t, album.Screenshot(m, "cd"))

	// MPW 3.1 dragged onto the hard disk, and copied
	must(t, op.Drag(108, 96, 471, 114))
	seconds := 0
	for ; !copyDialogShows(m) && seconds < 30; seconds++ {
		m.RunFrames(60)
	}
	m.RunFrames(1800)
	must(t, album.Screenshot(m, "copying"))
	for ; copyDialogShows(m); seconds++ {
		m.RunFrames(60)
	}
	t.Logf("copied in %v seconds", seconds+30)
	m.RunFrames(600)

	// The CD's window closed, and the folder of the server mounted with
	// the Chooser, as a guest
	must(t, op.ClickAt(30, 34))
	m.RunFrames(300)
	must(t, op.ChooseFromMenu(16, 59))
	m.RunFrames(1800)
	for _, click := range []struct{ h, v, frames int }{
		{100, 85, 1200}, {300, 88, 60}, {330, 177, 900}, {390, 258, 900},
		{200, 111, 60}, {336, 258, 1200}, {71, 47, 1200},
	} {
		must(t, op.ClickAt(click.h, click.v))
		op.Run(click.frames)
	}

	// Macintosh HD and its MPW 3.1 folder
	must(t, op.DoubleClickAt(471, 114))
	m.RunFrames(900)
	must(t, op.DoubleClickAt(56, 128))
	m.RunFrames(900)

	// The MPW Shell, and its Worksheet
	must(t, op.DoubleClickAt(30, 159))
	must(t, op.WaitForApplication("MPW Shell", 120))
	m.RunFrames(2400)
	must(t, album.Screenshot(m, "worksheet"))

	// The commands, each sent with Enter, the first with Command-Return
	must(t, op.Command("A"))
	must(t, op.TypeString(`Directory "For the Mac:"`))
	must(t, op.TypeKeys("Command+Return"))
	m.RunFrames(300)
	must(t, op.TypeString("\nC Mandelbrot.c"))
	must(t, op.TypeKeys("Enter"))
	t.Logf("compiled in %v seconds", written(m, filepath.Join(share, "Mandelbrot.c.o")))
	must(t, op.TypeString("\nLink -o Mandelbrot Mandelbrot.c.o \"{Libraries}\"Interface.o "+
		"\"{CLibraries}\"CRuntime.o \"{CLibraries}\"CInterface.o -t APPL -c '????'"))
	must(t, op.TypeKeys("Enter"))
	// The application is all resource fork, which the host keeps in its
	// AppleDouble file
	t.Logf("linked in %v seconds", written(m, filepath.Join(share, "._Mandelbrot")))
	m.RunFrames(900)
	must(t, album.Screenshot(m, "built"))

	// Mandelbrot run from the Worksheet, and drawn, recorded ten times as
	// fast
	must(t, op.TypeString("\nMandelbrot"))
	must(t, op.MoveTo(420, 300))
	drawing := Record(op, "")
	drawing.Capture(100)
	must(t, op.TypeKeys("Enter"))
	t.Logf("drawn in %v seconds", drawn(m, func() { op.Run(120); drawing.Capture(20) }))
	must(t, album.SaveRecording(drawing, "drawing", 300))

	// A rectangle around the end of the antenna, being dragged, and drawn
	must(t, op.MoveTo(150, 140))
	op.Hold()
	m.RunFrames(10)
	must(t, op.MoveTo(175, 156))
	must(t, op.MoveTo(198, 172))
	m.RunFrames(30)
	must(t, album.Screenshot(m, "selecting"))
	op.Release()
	t.Logf("drawn in %v seconds", drawn(m, func() { op.Run(120) }))
	must(t, op.MoveTo(420, 300))
	m.RunFrames(30)
	must(t, album.Screenshot(m, "antenna"))

	// And one around the copy of the set on it
	must(t, op.Drag(234, 144, 270, 168))
	t.Logf("drawn in %v seconds", drawn(m, func() { op.Run(120) }))
	must(t, op.MoveTo(420, 300))
	m.RunFrames(30)
	must(t, album.Screenshot(m, "copy"))
}

/*
drawn runs the machine, a step at a time, until the picture of Mandelbrot has
stopped changing for ten seconds, and says how many seconds it was changing
*/
func drawn(m *izmac.Mac, step func()) int {
	window := image.Rect(112, 60, 400, 252)
	var last []uint8
	seconds, still := 0, 0
	for still < 10 && seconds < 1200 {
		step()
		seconds += 2
		screen := m.Screenshot().SubImage(window).(*image.RGBA)
		var now []uint8
		for y := window.Min.Y; y < window.Max.Y; y++ {
			i := screen.PixOffset(window.Min.X, y)
			now = append(now, screen.Pix[i:i+window.Dx()*4]...)
		}
		if bytes.Equal(now, last) {
			still += 2
		} else {
			still = 0
		}
		last = now
	}
	return seconds - still
}

/*
written runs the machine until a file of the host has been written: there,
and the same size for three seconds. It says how many seconds that took,
less the three.
*/
func written(m *izmac.Mac, path string) int {
	size, still := int64(-1), 0
	seconds := 0
	for ; still < 3 && seconds < 900; seconds++ {
		m.RunFrames(60)
		info, err := os.Stat(path)
		if err == nil && info.Size() > 0 && info.Size() == size {
			still++
		} else {
			still = 0
		}
		if err == nil {
			size = info.Size()
		}
	}
	return seconds - 3
}

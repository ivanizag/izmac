package activities

import (
	"path/filepath"
	"testing"

	"github.com/ivanizag/izmac/operator"
)

/*
basicScreenshots is a program typed into Microsoft BASIC 2.0 and run: it asks
a name, greets it, draws circles in its window, and waits for a key
*/
func basicScreenshots(t *testing.T) {
	const page = "basic"
	album := NewAlbum(filepath.Join(activityImages, page))
	config := testConfig(t)
	config.Diskettes = []string{testImage(t, testBasicDiskette)}
	m := buildTestMac(t, config)
	must(t, operator.New(m).WaitForApplication("Finder", 60))
	m.RunFrames(600)

	// The diskette, and the BASIC with decimal numbers
	must(t, operator.New(m).DoubleClickAt(472, 45))
	m.RunFrames(900)
	must(t, album.Screenshot(m, "diskette"))
	must(t, operator.New(m).DoubleClickAt(92, 150))
	m.RunFrames(1500)
	must(t, album.Screenshot(m, "windows"))

	// The program, typed in the List window
	must(t, operator.New(m).TypeString("INPUT \"What is your name\"; N$\n"+
		"PRINT \"Hello, \"; N$; \", from 1986\"\n"+
		"FOR R = 10 TO 100 STEP 10\n"+
		"CIRCLE (250, 155), R\n"+
		"NEXT R\n"+
		"WHILE INKEY$ = \"\": WEND\n"))
	m.RunFrames(120)
	must(t, album.Screenshot(m, "program"))

	// The List window closed, and the program run, recorded: the question,
	// the name typed, and the drawing
	must(t, operator.New(m).MoveTo(204, 75))
	operator.New(m).Click()
	m.RunFrames(120)
	must(t, operator.New(m).Command("R"))
	m.RunFrames(300)
	running := Record(operator.New(m), "")
	running.Capture(150)
	for _, key := range []string{"Shift+A", "D", "A", "Return"} {
		must(t, operator.New(m).TypeKeys(key))
		running.Run(12, 6)
	}
	running.Run(600, 6)
	must(t, album.SaveRecording(running, "running", 300))
	must(t, operator.New(m).PressKey("Space"))
	m.RunFrames(300)
}

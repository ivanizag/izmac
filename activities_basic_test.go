package izmac

import (
	"testing"
)

/*
basicScreenshots is a program typed into Microsoft BASIC 2.0 and run: it asks
a name, greets it, draws circles in its window, and waits for a key
*/
func basicScreenshots(t *testing.T) {
	const page = "basic"
	config := testConfig(t)
	config.Diskettes = []string{testImage(t, testBasicDiskette)}
	m := buildTestMac(t, config)
	waitForApplication(t, m, "Finder", 60)
	m.RunFrames(600)

	// The diskette, and the BASIC with decimal numbers
	doubleClickAt(t, m, 472, 45)
	m.RunFrames(900)
	screenshot(t, m, page, "diskette")
	doubleClickAt(t, m, 92, 150)
	m.RunFrames(1500)
	screenshot(t, m, page, "windows")

	// The program, typed in the List window
	typeString(m, "INPUT \"What is your name\"; N$\n"+
		"PRINT \"Hello, \"; N$; \", from 1986\"\n"+
		"FOR R = 10 TO 100 STEP 10\n"+
		"CIRCLE (250, 155), R\n"+
		"NEXT R\n"+
		"WHILE INKEY$ = \"\": WEND\n")
	m.RunFrames(120)
	screenshot(t, m, page, "program")

	// The List window closed, and the program run, recorded: the question,
	// the name typed, and the drawing
	moveMouseTo(t, m, 204, 75)
	clickMouse(m)
	m.RunFrames(120)
	pressCommand(m, "R")
	m.RunFrames(300)
	running := record(m, "")
	running.capture(150)
	for _, key := range []string{"Shift+A", "D", "A", "Return"} {
		typeKeys(m, key)
		running.run(12, 6)
	}
	running.run(600, 6)
	running.save(t, page, "running", 300)
	pressKey(m, "Space")
	m.RunFrames(300)
}

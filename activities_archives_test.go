package izmac

import (
	"os"
	"path/filepath"
	"testing"
)

/*
archivesScreenshots is Bolo as the Tucows archive keeps it, a StuffIt archive
given to izmac beside System 6: unpacked on a volume of its own, opened, the
game started for practice, and the tank driven about
*/
func archivesScreenshots(t *testing.T) {
	const page = "archives"

	// The archive under the name it was downloaded with
	archive := filepath.Join(t.TempDir(), "bolojolopak.sit")
	data, err := os.ReadFile(testBoloArchive)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archive, data, 0o644); err != nil {
		t.Fatal(err)
	}

	config := testConfig(t)
	config.RamSizeKb = 4096
	if err := config.AddFiles([]string{testImage(t, testSystemSixDisk), archive}); err != nil {
		t.Fatal(err)
	}
	m := buildTestMac(t, config)
	waitForApplication(t, m, "Finder", 60)
	m.RunFrames(600)
	screenshot(t, m, page, "desktop")

	// The volume the archive was unpacked on, and the folder in it
	doubleClickAt(t, m, 472, 112)
	m.RunFrames(600)
	doubleClickAt(t, m, 113, 128)
	m.RunFrames(600)
	screenshot(t, m, page, "unpacked")

	// Bolo, and its first dialog
	doubleClickAt(t, m, 380, 150)
	waitForApplication(t, m, "Bolo 0.99.7", 60)
	m.RunFrames(900)
	screenshot(t, m, page, "bolo")

	// Practice, with nobody else on the network, and the options as they
	// are
	moveMouseTo(t, m, 84, 131)
	clickMouse(m)
	moveMouseTo(t, m, 388, 212)
	clickMouse(m)
	m.RunFrames(600)
	moveMouseTo(t, m, 418, 282)
	clickMouse(m)
	m.RunFrames(1200)

	// The tank driven, recorded: Q to go faster, O to turn left, and A to
	// slow down
	codes := KeyCodes()
	drive := record(m, "")
	drive.capture(100)
	for _, step := range []struct {
		key    string
		frames int
	}{{"Q", 120}, {"O", 40}, {"Q", 90}, {"P", 60}, {"Q", 90}, {"A", 120}} {
		m.PutKey(codes[step.key], true)
		drive.run(step.frames, 6)
		m.PutKey(codes[step.key], false)
	}
	drive.run(60, 6)
	drive.save(t, page, "driving", 200)
}

package activities

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ivanizag/izmac"
	"github.com/ivanizag/izmac/operator"
)

/*
archivesScreenshots is Bolo as the Tucows archive keeps it, a StuffIt archive
given to izmac beside System 6: unpacked on a volume of its own, opened, the
game started for practice, and the tank driven about
*/
func archivesScreenshots(t *testing.T) {
	const page = "archives"
	album := NewAlbum(filepath.Join(activityImages, page))

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
	must(t, operator.New(m).WaitForApplication("Finder", 60))
	m.RunFrames(600)
	must(t, album.Screenshot(m, "desktop"))

	// The volume the archive was unpacked on, and the folder in it
	must(t, operator.New(m).DoubleClickAt(472, 112))
	m.RunFrames(600)
	must(t, operator.New(m).DoubleClickAt(113, 128))
	m.RunFrames(600)
	must(t, album.Screenshot(m, "unpacked"))

	// Bolo, and its first dialog
	must(t, operator.New(m).DoubleClickAt(380, 150))
	must(t, operator.New(m).WaitForApplication("Bolo 0.99.7", 60))
	m.RunFrames(900)
	must(t, album.Screenshot(m, "bolo"))

	// Practice, with nobody else on the network, and the options as they
	// are
	must(t, operator.New(m).MoveTo(84, 131))
	operator.New(m).Click()
	must(t, operator.New(m).MoveTo(388, 212))
	operator.New(m).Click()
	m.RunFrames(600)
	must(t, operator.New(m).MoveTo(418, 282))
	operator.New(m).Click()
	m.RunFrames(1200)

	// The tank driven, recorded: Q to go faster, O to turn left, and A to
	// slow down
	codes := izmac.KeyCodes()
	drive := Record(operator.New(m), "")
	drive.Capture(100)
	for _, step := range []struct {
		key    string
		frames int
	}{{"Q", 120}, {"O", 40}, {"Q", 90}, {"P", 60}, {"Q", 90}, {"A", 120}} {
		m.PutKey(codes[step.key], true)
		drive.Run(step.frames, 6)
		m.PutKey(codes[step.key], false)
	}
	drive.Run(60, 6)
	must(t, album.SaveRecording(drive, "driving", 200))
}

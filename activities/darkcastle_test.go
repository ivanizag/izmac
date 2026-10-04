package activities

import (
	"path/filepath"
	"testing"

	"github.com/ivanizag/izmac/operator"
)

/*
darkCastleScreenshots is Dark Castle, whose diskette starts the machine
straight into the game: its title, its menu, and its demonstration
*/
func darkCastleScreenshots(t *testing.T) {
	const page = "dark-castle"
	album := NewAlbum(filepath.Join(activityImages, page))
	config := testConfig(t)
	config.Diskettes = []string{testImage(t, testDarkCastleDisk)}
	m := buildTestMac(t, config)
	m.RunFrames(2400)
	must(t, album.Screenshot(m, "title"))
	must(t, operator.New(m).PressKey("Space"))
	m.RunFrames(900)
	must(t, album.Screenshot(m, "castle"))
	must(t, operator.New(m).MoveTo(252, 311))
	operator.New(m).Click()
	m.RunFrames(600)
	demo := Record(operator.New(m), "")
	demo.Run(1200, 10)
	must(t, album.SaveRecording(demo, "demo", 100))
}

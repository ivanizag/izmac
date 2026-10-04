package activities

import (
	"path/filepath"
	"testing"

	"github.com/ivanizag/izmac/operator"
)

/*
lodeRunnerScreenshots is Lode Runner from its diskette: the diskette, the
first level played by the game itself until a key is pressed, and its Game
menu
*/
func lodeRunnerScreenshots(t *testing.T) {
	const page = "lode-runner"
	album := NewAlbum(filepath.Join(activityImages, page))
	config := testConfig(t)
	config.Diskettes = []string{testImage(t, testLodeRunnerDisk)}
	m := buildTestMac(t, config)
	must(t, operator.New(m).WaitForApplication("Finder", 60))
	m.RunFrames(600)
	must(t, operator.New(m).DoubleClickAt(472, 45))
	m.RunFrames(900)
	must(t, album.Screenshot(m, "diskette"))

	must(t, operator.New(m).DoubleClickAt(158, 165))
	m.RunFrames(1200)
	demo := Record(operator.New(m), "")
	demo.Run(900, 6)
	must(t, album.SaveRecording(demo, "demo", 100))

	must(t, operator.New(m).OpenMenu(147))
	must(t, album.Screenshot(m, "game-menu"))
	must(t, operator.New(m).CloseMenu())
}

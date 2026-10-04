package izmac

import (
	"testing"
)

/*
darkCastleScreenshots is Dark Castle, whose diskette starts the machine
straight into the game: its title, its menu, and its demonstration
*/
func darkCastleScreenshots(t *testing.T) {
	const page = "dark-castle"
	config := testConfig(t)
	config.Diskettes = []string{testImage(t, testDarkCastleDisk)}
	m := buildTestMac(t, config)
	m.RunFrames(2400)
	screenshot(t, m, page, "title")
	pressKey(m, "Space")
	m.RunFrames(900)
	screenshot(t, m, page, "castle")
	moveMouseTo(t, m, 252, 311)
	clickMouse(m)
	m.RunFrames(600)
	demo := record(m, "")
	demo.run(1200, 10)
	demo.save(t, page, "demo", 100)
}

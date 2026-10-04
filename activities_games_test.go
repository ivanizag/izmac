package izmac

import (
	"testing"
)

/*
gamesScreenshots is two games of the Plus playing their own demonstrations:
Lode Runner, from its diskette, and Dark Castle, which starts the machine
straight into the game
*/
func gamesScreenshots(t *testing.T) {
	const page = "games"

	// Lode Runner, opened from its diskette, which plays a level by itself
	// until a key is pressed
	config := testConfig(t)
	config.Diskettes = []string{testImage(t, testLodeRunnerDisk)}
	m := buildTestMac(t, config)
	waitForApplication(t, m, "Finder", 60)
	m.RunFrames(600)
	doubleClickAt(t, m, 472, 45)
	m.RunFrames(900)
	screenshot(t, m, page, "lode-runner-diskette")
	doubleClickAt(t, m, 158, 165)
	m.RunFrames(1200)
	lodeRunner := record(m, "")
	lodeRunner.run(900, 6)
	lodeRunner.save(t, page, "lode-runner", 100)

	// Dark Castle: its title, its menu, and its demonstration
	config = testConfig(t)
	config.Diskettes = []string{testImage(t, testDarkCastleDisk)}
	m = buildTestMac(t, config)
	m.RunFrames(2400)
	screenshot(t, m, page, "dark-castle")
	pressKey(m, "Space")
	m.RunFrames(900)
	screenshot(t, m, page, "dark-castle-menu")
	moveMouseTo(t, m, 252, 311)
	clickMouse(m)
	m.RunFrames(600)
	darkCastle := record(m, "")
	darkCastle.run(1200, 10)
	darkCastle.save(t, page, "dark-castle-demo", 100)
}

package izmac

import (
	"testing"
)

/*
lodeRunnerScreenshots is Lode Runner from its diskette: the diskette, the
first level played by the game itself until a key is pressed, and its Game
menu
*/
func lodeRunnerScreenshots(t *testing.T) {
	const page = "lode-runner"
	config := testConfig(t)
	config.Diskettes = []string{testImage(t, testLodeRunnerDisk)}
	m := buildTestMac(t, config)
	waitForApplication(t, m, "Finder", 60)
	m.RunFrames(600)
	doubleClickAt(t, m, 472, 45)
	m.RunFrames(900)
	screenshot(t, m, page, "diskette")

	doubleClickAt(t, m, 158, 165)
	m.RunFrames(1200)
	demo := record(m, "")
	demo.run(900, 6)
	demo.save(t, page, "demo", 100)

	openMenu(t, m, 147)
	screenshot(t, m, page, "game-menu")
	closeMenu(t, m)
}

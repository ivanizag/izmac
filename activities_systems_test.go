package izmac

import (
	"testing"
)

/*
systemsScreenshots is the same Macintosh Plus started with four Systems in
turn, 2.0, 4.1, 6.0.8 and 7.1.2: the desktop of each with its startup disk
opened, and the About box of its Finder
*/
func systemsScreenshots(t *testing.T) {
	const page = "systems"
	for _, system := range []struct {
		name     string
		image    string
		diskette bool
		ramKb    int
		boot     int
		diskV    int16
	}{
		{"system-2", testPaintDiskette, true, 1024, 600, 45},
		{"system-4", testSystemFourDiskette, true, 1024, 600, 45},
		{"system-6", testSystemSixDisk, false, 1024, 600, 50},
		{"system-7", testSystemSevenDisk, false, 4096, systemSevenBootFrames, 66},
	} {
		config := testConfig(t)
		if system.diskette {
			config.Diskettes = []string{testImage(t, system.image)}
		} else {
			config.DiskFiles = []string{testImage(t, system.image)}
		}
		config.RamSizeKb = system.ramKb
		m := buildTestMac(t, config)
		waitForApplication(t, m, "Finder", 120)
		m.RunFrames(uint64(system.boot))

		// The startup disk opened
		doubleClickAt(t, m, 472, system.diskV)
		m.RunFrames(1200)
		screenshot(t, m, page, system.name)

		// The About box of the Finder, the first item of the Apple menu
		chooseFromMenu(t, m, 16, 27)
		m.RunFrames(600)
		screenshot(t, m, page, system.name+"-about")
	}
}

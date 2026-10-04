package izmac

import (
	"os"
	"path/filepath"
	"testing"
)

/*
fileServerScreenshots is a folder of the host served with -share to a
Macintosh with System 6 on a diskette: the server found in the Chooser, the
folder mounted as a guest, a text file of the host opened in TeachText,
answered and saved, and a file of the diskette copied to the folder
*/
func fileServerScreenshots(t *testing.T) {
	const page = "file-server"
	share := filepath.Join(t.TempDir(), "For the Mac")
	if err := os.Mkdir(share, 0o755); err != nil {
		t.Fatal(err)
	}
	letter := "Dear Macintosh,\nThis letter was written on a computer of today.\n"
	if err := os.WriteFile(filepath.Join(share, "Letter.txt"), []uint8(letter), 0o644); err != nil {
		t.Fatal(err)
	}

	config := testConfig(t)
	config.Diskettes = []string{testImage(t, testSystemSixDiskette)}
	config.RamSizeKb = 4096
	config.Share = share
	config.shareServerName = "my-computer"
	config.PrinterPort = ""
	m := buildTestMac(t, config)
	t.Cleanup(m.fileServer.Stop)
	waitForApplication(t, m, "Finder", 60)
	m.RunFrames(1200)

	// The Chooser, and the server under AppleShare
	chooseFromMenu(t, m, 16, 59)
	m.RunFrames(1800)
	moveMouseTo(t, m, 100, 85)
	clickMouse(m)
	m.RunFrames(1200)
	moveMouseTo(t, m, 300, 88)
	clickMouse(m)
	m.RunFrames(60)
	screenshot(t, m, page, "chooser")

	// As a guest, and the folder as the volume to use
	moveMouseTo(t, m, 330, 177)
	clickMouse(m)
	m.RunFrames(900)
	screenshot(t, m, page, "guest")
	moveMouseTo(t, m, 390, 258)
	clickMouse(m)
	m.RunFrames(900)
	moveMouseTo(t, m, 200, 111)
	clickMouse(m)
	m.RunFrames(60)
	screenshot(t, m, page, "volumes")
	moveMouseTo(t, m, 336, 258)
	clickMouse(m)
	m.RunFrames(1200)

	// The Chooser closed, and the folder on the desktop, opened
	moveMouseTo(t, m, 71, 47)
	clickMouse(m)
	m.RunFrames(1200)
	screenshot(t, m, page, "mounted")
	doubleClickAt(t, m, 472, 104)
	m.RunFrames(1500)
	screenshot(t, m, page, "folder")

	// The letter opened in TeachText, answered and saved
	doubleClickAt(t, m, 120, 130)
	waitForApplication(t, m, "TeachText", 60)
	m.RunFrames(600)
	screenshot(t, m, page, "letter")
	moveMouseTo(t, m, 470, 200)
	clickMouse(m)
	pressKey(m, "Return")
	typeString(m, "Dear computer of today,\nThis answer was written on a Macintosh Plus.")
	m.RunFrames(60)
	pressCommand(m, "S")
	m.RunFrames(900)
	screenshot(t, m, page, "answered")
	pressCommand(m, "Q")
	waitForApplication(t, m, "Finder", 60)
	m.RunFrames(900)

	// The Read Me of the diskette dragged into the folder, recorded
	doubleClickAt(t, m, 472, 50)
	m.RunFrames(1200)
	moveMouseTo(t, m, 174, 106)
	m.RunFrames(30)
	copying := record(m, "")
	copying.capture(50)
	copying.press(true)
	copying.run(20, 2)
	copying.glide(t, 472, 104)
	copying.run(20, 2)
	copying.press(false)
	copying.run(900, 6)
	copying.doubleClick()
	copying.run(900, 6)
	copying.save(t, page, "copying", 300)
}

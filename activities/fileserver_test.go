package activities

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ivanizag/izmac/operator"
)

/*
fileServerScreenshots is a folder of the host served with -share to a
Macintosh with System 6 on a diskette: the server found in the Chooser, the
folder mounted as a guest, a text file of the host opened in TeachText,
answered and saved, and a file of the diskette copied to the folder
*/
func fileServerScreenshots(t *testing.T) {
	const page = "file-server"
	album := NewAlbum(filepath.Join(activityImages, page))
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
	config.ShareName = "my-computer"
	config.PrinterPort = ""
	m := buildTestMac(t, config)
	must(t, operator.New(m).WaitForApplication("Finder", 60))
	m.RunFrames(1200)

	// The Chooser, and the server under AppleShare
	must(t, operator.New(m).ChooseFromMenu(16, 59))
	m.RunFrames(1800)
	must(t, operator.New(m).MoveTo(100, 85))
	operator.New(m).Click()
	m.RunFrames(1200)
	must(t, operator.New(m).MoveTo(300, 88))
	operator.New(m).Click()
	m.RunFrames(60)
	must(t, album.Screenshot(m, "chooser"))

	// As a guest, and the folder as the volume to use
	must(t, operator.New(m).MoveTo(330, 177))
	operator.New(m).Click()
	m.RunFrames(900)
	must(t, album.Screenshot(m, "guest"))
	must(t, operator.New(m).MoveTo(390, 258))
	operator.New(m).Click()
	m.RunFrames(900)
	must(t, operator.New(m).MoveTo(200, 111))
	operator.New(m).Click()
	m.RunFrames(60)
	must(t, album.Screenshot(m, "volumes"))
	must(t, operator.New(m).MoveTo(336, 258))
	operator.New(m).Click()
	m.RunFrames(1200)

	// The Chooser closed, and the folder on the desktop, opened
	must(t, operator.New(m).MoveTo(71, 47))
	operator.New(m).Click()
	m.RunFrames(1200)
	must(t, album.Screenshot(m, "mounted"))
	must(t, operator.New(m).DoubleClickAt(472, 104))
	m.RunFrames(1500)
	must(t, album.Screenshot(m, "folder"))

	// The letter opened in TeachText, answered and saved
	must(t, operator.New(m).DoubleClickAt(120, 130))
	must(t, operator.New(m).WaitForApplication("TeachText", 60))
	m.RunFrames(600)
	must(t, album.Screenshot(m, "letter"))
	must(t, operator.New(m).MoveTo(470, 200))
	operator.New(m).Click()
	must(t, operator.New(m).PressKey("Return"))
	must(t, operator.New(m).TypeString("Dear computer of today,\nThis answer was written on a Macintosh Plus."))
	m.RunFrames(60)
	must(t, operator.New(m).Command("S"))
	m.RunFrames(900)
	must(t, album.Screenshot(m, "answered"))
	must(t, operator.New(m).Command("Q"))
	must(t, operator.New(m).WaitForApplication("Finder", 60))
	m.RunFrames(900)

	// The Read Me of the diskette dragged into the folder, recorded
	must(t, operator.New(m).DoubleClickAt(472, 50))
	m.RunFrames(1200)
	must(t, operator.New(m).MoveTo(174, 106))
	m.RunFrames(30)
	copying := Record(operator.New(m), "")
	copying.Capture(50)
	copying.Press(true)
	copying.Run(20, 2)
	must(t, copying.Glide(472, 104))
	copying.Run(20, 2)
	copying.Press(false)
	copying.Run(900, 6)
	copying.DoubleClick()
	copying.Run(900, 6)
	must(t, album.SaveRecording(copying, "copying", 300))
}

package activities

import (
	"path/filepath"
	"testing"

	"github.com/ivanizag/izmac/operator"
)

/*
resEditScreenshots is ResEdit 2.1, from its StuffIt archive, beside System 6:
TeachText opened in it, its menus, the Quit of its File menu renamed Goodbye,
saved, and TeachText opened to see it
*/
func resEditScreenshots(t *testing.T) {
	const page = "resedit"
	album := NewAlbum(filepath.Join(activityImages, page))
	config := testConfig(t)
	config.RamSizeKb = 4096
	if err := config.AddFiles([]string{testImage(t, testSystemSixDisk), testImage(t, testResEditArchive)}); err != nil {
		t.Fatal(err)
	}
	m := buildTestMac(t, config)
	must(t, operator.New(m).WaitForApplication("Finder", 60))
	m.RunFrames(900)
	must(t, album.Screenshot(m, "diskette"))

	// ResEdit, and the jack in the box it starts with
	must(t, operator.New(m).DoubleClickAt(90, 265))
	must(t, operator.New(m).WaitForApplication("ResEdit", 60))
	m.RunFrames(1200)
	must(t, album.Screenshot(m, "resedit"))

	// The file to open, TeachText on the hard disk
	must(t, operator.New(m).MoveTo(300, 200))
	operator.New(m).Click()
	m.RunFrames(600)
	must(t, operator.New(m).MoveTo(364, 153))
	operator.New(m).Click()
	m.RunFrames(300)
	must(t, operator.New(m).MoveTo(150, 161))
	operator.New(m).Click()
	m.RunFrames(60)
	must(t, album.Screenshot(m, "open"))
	must(t, operator.New(m).MoveTo(378, 220))
	operator.New(m).Click()
	m.RunFrames(1200)
	must(t, album.Screenshot(m, "resources"))

	// Its menus, with the Resource menu
	must(t, operator.New(m).MoveTo(145, 125))
	operator.New(m).Click()
	must(t, operator.New(m).ChooseFromMenu(152, 43))
	m.RunFrames(900)
	must(t, album.Screenshot(m, "menus"))

	// The File menu in the menu editor
	must(t, operator.New(m).MoveTo(265, 170))
	operator.New(m).Click()
	must(t, operator.New(m).ChooseFromMenu(152, 43))
	m.RunFrames(900)

	// Quit renamed Goodbye, recorded, and the File menu ResEdit shows to
	// try it pulled down
	must(t, operator.New(m).MoveTo(40, 298))
	operator.New(m).Click()
	m.RunFrames(300)
	must(t, operator.New(m).Command("A"))
	m.RunFrames(30)
	renaming := Record(operator.New(m), "")
	renaming.Capture(100)
	for _, key := range []string{"Shift+G", "O", "O", "D", "B", "Y", "E"} {
		must(t, operator.New(m).TypeKeys(key))
		renaming.Run(12, 6)
	}
	renaming.Run(30, 6)
	must(t, renaming.Glide(360, 10))
	renaming.Press(true)
	renaming.Run(30, 6)
	must(t, album.SaveRecording(renaming, "renaming", 300))
	must(t, operator.New(m).MoveTo(300, 300))
	m.SetMouseButton(false)
	m.RunFrames(30)

	// Saved, and ResEdit quit
	must(t, operator.New(m).Command("S"))
	m.RunFrames(600)
	must(t, operator.New(m).Command("Q"))
	must(t, operator.New(m).WaitForApplication("Finder", 60))
	m.RunFrames(600)

	// TeachText, with its File menu as it is now
	must(t, operator.New(m).MoveTo(472, 50))
	operator.New(m).Click()
	must(t, operator.New(m).Command("O"))
	m.RunFrames(600)
	must(t, operator.New(m).DoubleClickAt(110, 105))
	must(t, operator.New(m).WaitForApplication("TeachText", 60))
	m.RunFrames(600)
	must(t, operator.New(m).OpenMenu(53))
	must(t, album.Screenshot(m, "goodbye"))
	must(t, operator.New(m).CloseMenu())
}

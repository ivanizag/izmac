package activities

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/ivanizag/izmac"
	"github.com/ivanizag/izmac/hfs"
	"github.com/ivanizag/izmac/operator"
)

/*
thinkPascalScreenshots is a game written in THINK Pascal 4.0 and built into
an application: THINK Pascal installed on a hard disk from its first two
diskettes, the program of the page put in the shared folder and copied to the
hard disk, a project made with it, the game run inside THINK Pascal and
played, and built into an application that plays on its own
*/
// gameListing is the program of the page, as the reader downloads it
const gameListing = "../doc/activities/listings/2048.p"

/*
The page shows the program in blocks, with what each does, and links to it
whole: the blocks, one after the other, are the program
*/
func TestThePageOfTHINKPascalHasTheWholeProgram(t *testing.T) {
	listing, err := os.ReadFile(gameListing)
	if err != nil {
		t.Fatal(err)
	}
	if blocks := pageListing(t, "think-pascal", "pascal"); blocks != string(listing) {
		t.Errorf("the blocks of Pascal of the page are not %v", gameListing)
	}
}

func thinkPascalScreenshots(t *testing.T) {
	const page = "think-pascal"
	album := NewAlbum(filepath.Join(activityImages, page))
	share := filepath.Join(t.TempDir(), "For the Mac")
	if err := os.Mkdir(share, 0o755); err != nil {
		t.Fatal(err)
	}
	listing, err := os.ReadFile(gameListing)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(share, "2048.p"), listing, 0o644); err != nil {
		t.Fatal(err)
	}
	predate(t, filepath.Join(share, "2048.p"), share)

	config := testConfig(t)
	config.Diskettes = []string{
		testImage(t, testSystemSixDiskette),
		testImage(t, testThinkPascalTwoDiskette),
	}
	config.DiskFiles = []string{hardDisk(t)}
	config.RamSizeKb = 4096
	config.Share = share
	config.ShareName = "my-computer"
	config.PrinterPort = ""
	m := buildTestMac(t, config)
	op := operator.New(m)
	must(t, op.WaitForApplication("Finder", 60))
	m.RunFrames(1200)
	must(t, album.Screenshot(m, "desktop"))

	// Interfaces & Libs.sea, and its dialog turned to the hard disk
	must(t, op.DoubleClickAt(162, 90))
	m.RunFrames(1800)
	must(t, op.ClickAt(393, 157))
	m.RunFrames(300)
	must(t, op.ClickAt(393, 157))
	m.RunFrames(300)
	must(t, album.Screenshot(m, "extract"))

	// Extracted, which takes five minutes
	must(t, op.ClickAt(393, 201))
	m.RunFrames(3600)
	must(t, album.Screenshot(m, "extracting"))
	must(t, op.WaitForApplication("Finder", 600))
	m.RunFrames(600)

	// THINK Pascal 2 ejected and THINK Pascal 1 in its place
	must(t, op.Drag(471, 193, 471, 298))
	m.RunFrames(600)
	must(t, op.InsertDiskette(1, testImage(t, testThinkPascalOneDiskette)))
	m.RunFrames(1200)

	// Macintosh HD and its THINK Pascal 4.0 Folder opened
	must(t, op.DoubleClickAt(471, 116))
	m.RunFrames(900)
	must(t, op.DoubleClickAt(119, 128))
	m.RunFrames(900)

	// THINK Pascal 4.0 dragged from the diskette into the folder, recorded
	must(t, op.ClickAt(150, 31))
	m.RunFrames(300)
	must(t, op.MoveTo(199, 138))
	copying := Record(op, "")
	copying.Capture(100)
	copying.Press(true)
	must(t, copying.Glide(145, 232))
	copying.Press(false)
	copying.Run(120, 6)
	seconds := 2
	for ; !copyDialogShows(m) && seconds < 20; seconds++ {
		copying.Run(60, 6)
	}
	for ; copyDialogShows(m); seconds++ {
		for range 6 {
			op.Run(10)
			copying.Capture(3)
		}
	}
	t.Logf("copied in %v seconds", seconds)
	copying.Run(300, 6)
	must(t, album.SaveRecording(copying, "copy", 300))

	// The diskette's window closed
	must(t, op.ClickAt(23, 31))
	m.RunFrames(300)

	// The folder of the server mounted with the Chooser, as a guest
	must(t, op.ChooseFromMenu(16, 59))
	m.RunFrames(1800)
	must(t, op.ClickAt(100, 85))
	m.RunFrames(1200)
	must(t, op.ClickAt(300, 88))
	m.RunFrames(60)
	must(t, op.ClickAt(330, 177))
	m.RunFrames(900)
	must(t, op.ClickAt(390, 258))
	m.RunFrames(900)
	must(t, op.ClickAt(200, 111))
	m.RunFrames(60)
	must(t, op.ClickAt(336, 258))
	m.RunFrames(1200)
	must(t, op.ClickAt(71, 47))
	m.RunFrames(1200)
	must(t, op.DoubleClickAt(471, 233))
	m.RunFrames(1500)
	must(t, album.Screenshot(m, "shared"))

	// 2048.p dragged into the THINK Pascal 4.0 Folder, at the edge of it
	// that shows, and that folder brought to the front
	must(t, op.Drag(159, 206, 50, 245))
	m.RunFrames(900)
	must(t, op.ClickAt(250, 92))
	m.RunFrames(600)
	must(t, op.Drag(62, 258, 270, 245))
	m.RunFrames(300)
	must(t, album.Screenshot(m, "source"))

	// THINK Pascal, and a new project, 2048.π
	must(t, op.DoubleClickAt(151, 245))
	must(t, op.WaitForApplication("THINK Pascal 4.0", 60))
	m.RunFrames(1200)
	must(t, op.ClickAt(377, 207))
	m.RunFrames(600)
	must(t, op.TypeString("2048."))
	must(t, op.TypeKeys("Option+P"))
	m.RunFrames(60)
	must(t, album.Screenshot(m, "new-project"))
	must(t, op.PressKey("Return"))
	m.RunFrames(1200)

	// 2048.p added to the project
	must(t, op.ChooseFromMenu(196, 107))
	m.RunFrames(600)
	must(t, op.ClickAt(377, 182))
	m.RunFrames(300)
	must(t, op.ClickAt(377, 207))
	m.RunFrames(600)
	must(t, album.Screenshot(m, "project"))

	// The program opened in THINK Pascal's editor
	must(t, op.Command("O"))
	m.RunFrames(600)
	must(t, op.PressKey("Return"))
	m.RunFrames(1800)
	must(t, album.Screenshot(m, "editor"))

	// Go: compiled and run, recorded at twice the speed, and played with
	// the arrows
	must(t, op.MoveTo(400, 200))
	running := Record(op, "")
	running.Capture(100)
	must(t, op.Command("G"))
	for range 42 {
		op.Run(30)
		running.Capture(25)
	}
	for _, key := range []string{"Left", "Down", "Left", "Down", "Right", "Down", "Left", "Down", "Left", "Up", "Left", "Down"} {
		must(t, op.TypeKeys(key))
		running.Run(48, 6)
	}
	must(t, album.SaveRecording(running, "playing", 300))

	// The game quit, and built into an application
	must(t, op.Command("Q"))
	m.RunFrames(600)
	must(t, op.ChooseFromMenu(196, 171))
	m.RunFrames(600)
	must(t, op.TypeString("2048"))
	m.RunFrames(60)
	must(t, album.Screenshot(m, "build"))
	must(t, op.PressKey("Return"))
	m.RunFrames(1800)

	// THINK Pascal quit, and the application in the folder
	must(t, op.Command("Q"))
	must(t, op.WaitForApplication("Finder", 60))
	m.RunFrames(1200)
	must(t, op.ClickAt(250, 92))
	m.RunFrames(600)
	must(t, op.ChooseFromMenu(185, 27))
	m.RunFrames(600)
	must(t, album.Screenshot(m, "application"))

	// The game on its own, played, with its Apple menu down
	must(t, op.DoubleClickAt(68, 208))
	m.RunFrames(900)
	for _, key := range []string{"Left", "Down", "Left", "Down", "Right", "Down", "Left", "Down", "Left", "Up", "Left", "Down", "Left", "Down"} {
		must(t, op.TypeKeys(key))
		m.RunFrames(30)
	}
	must(t, op.OpenMenu(16))
	must(t, album.Screenshot(m, "game"))
	must(t, op.CloseMenu())
}

// copyDialogShows says whether the Finder's dialog of a copy is on the
// screen, from its left edge
func copyDialogShows(m *izmac.Mac) bool {
	screen := m.Screenshot()
	return screen.RGBAAt(91, 46).R == 0 && screen.RGBAAt(91, 66).R == 0
}

/*
pageListing is the program of a page: its blocks of code in a language, one
after the other, as the reader puts them together
*/
func pageListing(t *testing.T, page string, language string) string {
	t.Helper()
	text, err := os.ReadFile(filepath.Join(activityImages, "..", page+".md"))
	if err != nil {
		t.Fatal(err)
	}
	blocks := regexp.MustCompile("(?s)```"+language+"\n(.*?)```").FindAllStringSubmatch(string(text), -1)
	if len(blocks) == 0 {
		t.Fatalf("the page %v has no %v", page, language)
	}
	var listing strings.Builder
	for i, block := range blocks {
		if i > 0 {
			listing.WriteString("\n")
		}
		listing.WriteString(block[1])
	}
	return listing.String()
}

/*
hardDisk is an empty hard disk of 20 megabytes, Macintosh HD, as the one the
reader made in the activity of installing System 6 is named. Only its space
is used: the System on the reader's is not, and is left out.
*/
func hardDisk(t *testing.T) string {
	t.Helper()
	root := &hfs.Folder{Modified: activityStart().Add(-time.Hour)}
	data, err := hfs.Build("Macintosh HD", root, 20<<20)
	if err != nil {
		t.Fatal(err)
	}
	disk := filepath.Join(t.TempDir(), "blank.img")
	if err := os.WriteFile(disk, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return disk
}

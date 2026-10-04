package activities

import (
	"image"
	"os"
	"path/filepath"
	"testing"

	"github.com/ivanizag/izmac"
	"github.com/ivanizag/izmac/localtalk"
	"github.com/ivanizag/izmac/operator"
)

/*
The screenshots of the activities in doc/activities, made by taking a machine
through what each page tells its reader to do. Like TestSettleTestImages it is
not a test: it does nothing unless asked for with IZMAC_ACTIVITY_SCREENSHOTS,
and then writes the images of each page in doc/activities/images, so that they
can be made again when the emulator or the instructions change.

	IZMAC_ACTIVITY_SCREENSHOTS=1 go test -run TestActivityScreenshots .
*/

func TestActivityScreenshots(t *testing.T) {
	if os.Getenv("IZMAC_ACTIVITY_SCREENSHOTS") == "" {
		t.Skip("this makes the screenshots of doc/activities")
	}
	t.Run("first-steps", firstStepsScreenshots)
	t.Run("macpaint", macPaintScreenshots)
	t.Run("file-sharing", fileSharingScreenshots)
	t.Run("desk-accessories", deskAccessoriesScreenshots)
	t.Run("multifinder", multiFinderScreenshots)
	t.Run("floppies", floppiesScreenshots)
	t.Run("file-server", fileServerScreenshots)
	t.Run("archives", archivesScreenshots)
	t.Run("macwrite", macWriteScreenshots)
	t.Run("spreadsheet", spreadsheetScreenshots)
	t.Run("basic", basicScreenshots)
	t.Run("hypercard", hyperCardScreenshots)
	t.Run("resedit", resEditScreenshots)
	t.Run("installing", installingScreenshots)
	t.Run("printing", printingScreenshots)
	t.Run("systems", systemsScreenshots)
	t.Run("lode-runner", lodeRunnerScreenshots)
	t.Run("dark-castle", darkCastleScreenshots)
}

/*
firstStepsScreenshots is the 1984 experience: izmac started with nothing, which
is the MacPaint diskette with System 2.0, and the Finder taken through its
desktop, its menus, a desk accessory, Get Info, the Trash and Shut Down
*/
func firstStepsScreenshots(t *testing.T) {
	const page = "first-steps"
	album := NewAlbum(filepath.Join(activityImages, page))
	config := testConfig(t)
	config.Diskettes = []string{testImage(t, testPaintDiskette)}
	m := buildTestMac(t, config)

	// Switched on, recorded from the first frame to the desktop
	// Switched on, and recorded from the disk with the question mark that
	// asks for a disk after the memory test, to the desktop
	waitForAQuestionMark(t, m)
	starting := Record(operator.New(m), "")
	starting.Capture(10)
	for frame := 0; m.CurrentApplication() != "Finder"; frame += 6 {
		if frame > 60*60 {
			t.Fatalf("the Finder did not start")
		}
		starting.Run(6, 6)
	}
	starting.Run(600, 6)
	must(t, album.SaveRecording(starting, "starting", 300))

	// The diskette opened, recorded: the pointer taken to it, the double
	// click, and the window coming out of the icon
	must(t, operator.New(m).MoveTo(472, 45))
	m.RunFrames(30)
	opening := Record(operator.New(m), "")
	opening.Capture(100)
	opening.DoubleClick()
	opening.Run(600, 6)
	must(t, album.SaveRecording(opening, "open-disk", 300))
	must(t, album.Screenshot(m, "disk-window"))

	// The Apple menu, recorded: pulled down, gone over to the bottom and
	// back up to the Puzzle, and let go of outside afterwards, which chooses
	// nothing
	must(t, operator.New(m).MoveTo(16, 10))
	m.RunFrames(30)
	menu := Record(operator.New(m), "")
	menu.Capture(50)
	menu.Press(true)
	menu.Run(20, 2)
	must(t, menu.Glide(40, 30))
	must(t, menu.Glide(40, 170))
	menu.Run(20, 2)
	must(t, menu.Glide(40, 155))
	menu.Run(10, 2)
	must(t, album.SaveRecording(menu, "apple-menu", 200))
	must(t, operator.New(m).MoveTo(300, 250))
	m.SetMouseButton(false)
	m.RunFrames(30)

	// About the Finder
	must(t, operator.New(m).ChooseFromMenu(16, 27))
	m.RunFrames(300)
	must(t, album.Screenshot(m, "about-the-finder"))
	operator.New(m).Click()
	m.RunFrames(120)

	// The Puzzle, a desk accessory, which shuffles itself each time, and
	// its close box
	must(t, operator.New(m).ChooseFromMenu(16, 155))
	m.RunFrames(300)
	must(t, album.Screenshot(m, "puzzle"))
	must(t, operator.New(m).MoveTo(113, 89))
	operator.New(m).Click()
	m.RunFrames(120)

	// The window by name, and back to icons
	must(t, operator.New(m).OpenMenu(135))
	must(t, album.Screenshot(m, "view-menu"))
	must(t, operator.New(m).CloseMenu())
	must(t, operator.New(m).ChooseFromMenu(135, 59))
	m.RunFrames(300)
	must(t, album.Screenshot(m, "by-name"))
	must(t, operator.New(m).ChooseFromMenu(135, 27))
	m.RunFrames(300)

	// Get Info on MacPaint
	must(t, operator.New(m).MoveTo(94, 140))
	operator.New(m).Click()
	m.RunFrames(30)
	must(t, operator.New(m).Command("I"))
	m.RunFrames(300)
	must(t, album.Screenshot(m, "get-info"))
	must(t, operator.New(m).MoveTo(24, 40))
	operator.New(m).Click()
	m.RunFrames(120)

	// A new folder, dragged to the Trash, and the Trash emptied
	must(t, operator.New(m).Command("N"))
	m.RunFrames(300)
	must(t, album.Screenshot(m, "new-folder"))
	must(t, operator.New(m).MoveTo(287, 140))
	m.RunFrames(30)
	trash := Record(operator.New(m), "")
	trash.Capture(50)
	trash.Press(true)
	trash.Run(20, 2)
	must(t, trash.Glide(472, 318))
	trash.Run(30, 2)
	trash.Press(false)
	trash.Run(300, 6)
	must(t, album.SaveRecording(trash, "to-the-trash", 300))
	must(t, operator.New(m).OpenMenu(185))
	must(t, album.Screenshot(m, "special-menu"))
	must(t, operator.New(m).CloseMenu())
	must(t, operator.New(m).ChooseFromMenu(185, 43))
	m.RunFrames(300)

	// Shut Down, the last item of Special, and the disk with the question
	// mark that blinks while the machine waits for one
	must(t, operator.New(m).ChooseFromMenu(185, 123))
	m.RunFrames(900)
	waitForAQuestionMark(t, m)
	shutDown := Record(operator.New(m), "")
	shutDown.Capture(5)
	shutDown.Run(240, 3)
	must(t, album.SaveRecording(shutDown, "shut-down", 0))
}

// dragOn drags with the button held from one place to another, which is how
// MacPaint draws
func dragOn(t *testing.T, m *izmac.Mac, fromH, fromV, toH, toV int) {
	t.Helper()
	must(t, operator.New(m).MoveTo(fromH, fromV))
	m.SetMouseButton(true)
	m.RunFrames(10)
	must(t, operator.New(m).MoveTo((fromH+toH)/2, (fromV+toV)/2))
	must(t, operator.New(m).MoveTo(toH, toV))
	m.RunFrames(10)
	m.SetMouseButton(false)
	m.RunFrames(30)
}

// pick clicks a tool or a pattern of MacPaint
func pick(t *testing.T, m *izmac.Mac, h, v int) {
	t.Helper()
	must(t, operator.New(m).MoveTo(h, v))
	operator.New(m).Click()
	m.RunFrames(20)
}

/*
macPaintScreenshots is a drawing made in MacPaint, from the same diskette:
shapes with patterns, text, FatBits, the page, and the drawing printed on the
ImageWriter, whose page is kept as it came out
*/
func macPaintScreenshots(t *testing.T) {
	const page = "macpaint"
	album := NewAlbum(filepath.Join(activityImages, page))
	config := testConfig(t)
	config.Diskettes = []string{testImage(t, testPaintDiskette)}
	config.Printer = izmac.PrinterImageWriter
	config.PrinterFile = filepath.Join(t.TempDir(), "page")
	m := buildTestMac(t, config)
	must(t, operator.New(m).WaitForApplication("Finder", 60))
	m.RunFrames(600)
	must(t, operator.New(m).DoubleClickAt(472, 45))
	m.RunFrames(600)
	must(t, operator.New(m).DoubleClickAt(94, 140))
	must(t, operator.New(m).WaitForApplication("MacPaint", 60))
	m.RunFrames(900)
	must(t, album.Screenshot(m, "empty"))

	// A rectangle filled with bricks, recorded as it is dragged out, and an
	// empty oval
	pick(t, m, 350, 305)
	pick(t, m, 54, 150)
	must(t, operator.New(m).MoveTo(110, 70))
	m.RunFrames(30)
	rectangle := Record(operator.New(m), "")
	rectangle.Capture(50)
	rectangle.Press(true)
	rectangle.Run(10, 2)
	must(t, rectangle.Glide(160, 160))
	must(t, rectangle.Glide(240, 100))
	must(t, rectangle.Glide(220, 150))
	rectangle.Run(20, 2)
	rectangle.Press(false)
	rectangle.Run(30, 3)
	must(t, album.SaveRecording(rectangle, "rectangle", 200))
	pick(t, m, 29, 194)
	dragOn(t, m, 250, 70, 380, 160)

	// The oval filled with grey by the paint bucket, recorded: the bucket
	// clicked inside it, and the pattern poured in
	pick(t, m, 150, 322)
	pick(t, m, 29, 84)
	must(t, operator.New(m).MoveTo(315, 120))
	m.RunFrames(30)
	fill := Record(operator.New(m), "")
	fill.Capture(100)
	m.SetMouseButton(true)
	fill.Run(6, 2)
	m.SetMouseButton(false)
	fill.Run(60, 3)
	must(t, album.SaveRecording(fill, "fill", 200))

	// An empty rounded rectangle around both
	pick(t, m, 29, 172)
	dragOn(t, m, 95, 60, 400, 175)
	must(t, album.Screenshot(m, "shapes"))

	// Text, and a line with the pencil under it
	pick(t, m, 54, 62)
	must(t, operator.New(m).MoveTo(140, 210))
	operator.New(m).Click()
	must(t, operator.New(m).TypeKeys("Shift+H", "E", "L", "L", "O", "Space", "F", "R", "O", "M", "Space",
		"1", "9", "8", "4", "Shift+1"))
	pick(t, m, 54, 106)
	dragOn(t, m, 140, 230, 340, 232)

	// The spray can, in black, recorded while it is held down and moved
	// about
	pick(t, m, 127, 306)
	pick(t, m, 54, 84)
	must(t, operator.New(m).MoveTo(400, 215))
	m.RunFrames(30)
	spray := Record(operator.New(m), "")
	spray.Capture(50)
	spray.Press(true)
	spray.Run(30, 2)
	must(t, spray.Glide(440, 230))
	spray.Run(20, 2)
	must(t, spray.Glide(410, 250))
	spray.Run(20, 2)
	must(t, spray.Glide(380, 235))
	spray.Run(20, 2)
	spray.Press(false)
	spray.Run(20, 2)
	must(t, album.SaveRecording(spray, "spray-can", 200))
	must(t, album.Screenshot(m, "drawing"))

	// FatBits, the drawing a dot at a time, entered with the command key and
	// a click of the pencil on the edge of the oval, and left the same way
	codes := izmac.KeyCodes()
	m.PutKey(codes["Command"], true)
	m.RunFrames(6)
	must(t, operator.New(m).MoveTo(250, 115))
	operator.New(m).Click()
	m.PutKey(codes["Command"], false)
	m.RunFrames(300)
	must(t, album.Screenshot(m, "fatbits"))
	must(t, operator.New(m).ChooseFromMenu(140, 43))
	m.RunFrames(300)

	// The whole page
	must(t, operator.New(m).ChooseFromMenu(140, 59))
	m.RunFrames(300)
	must(t, album.Screenshot(m, "show-page"))
	must(t, operator.New(m).MoveTo(440, 229))
	operator.New(m).Click()
	m.RunFrames(300)

	// Print Final, and the page MacPaint draws as it sends it to the printer,
	// kept as it was last seen whole, before MacPaint went back to the
	// drawing; the grey around the page is how its window is told from the
	// drawing's
	must(t, operator.New(m).ChooseFromMenu(55, 139))
	printed := config.PrinterFile + "_001.png"
	var drawn *image.RGBA
	for second := 0; !exists(printed); second++ {
		if second == 600 {
			t.Fatalf("MacPaint printed nothing")
		}
		m.RunFrames(30)
		screen := m.GetImage()
		if showsThePage(screen) {
			drawn = image.NewRGBA(screen.Bounds())
			copy(drawn.Pix, screen.Pix)
		}
	}
	if drawn == nil {
		t.Fatalf("MacPaint printed without showing the page")
	}
	must(t, album.Write(drawn, "printing", ""))
	m.RunFrames(600)
	must(t, album.KeepPrintedPage(printed, "printed-page"))
}

/*
fileSharingScreenshots is two machines with System 7 on one LocalTalk, as two
izmacs with -appletalk host are: the first shares a folder with its File
Sharing and lets guests in, and the second mounts it from the Chooser and
copies a file into it, which the first then shows. The first runs on a
goroutine of its own while the second is driven.
*/
func fileSharingScreenshots(t *testing.T) {
	const page, ada, grace = "file-sharing", "Ada's Mac", "Grace's Mac"
	album := NewAlbum(filepath.Join(activityImages, page))
	network := localtalk.NewNetwork()
	machine := func() *izmac.Mac {
		config := testConfig(t)
		config.DiskFiles = []string{testImage(t, testSystemSevenDisk)}
		config.RamSizeKb = 4096
		config.AppleTalk = izmac.AppleTalkLocal
		config.PrinterPort = ""
		config.LocalTalkNetwork = network
		m := buildTestMac(t, config)
		must(t, operator.New(m).WaitForApplication("Finder", 120))
		m.RunFrames(600)
		return m
	}

	// The first: Sharing Setup, from the Control Panels of the System Folder
	server := machine()

	// Its disk opened at once, which is when the Finder puts the icons in
	// the order of their names, and renamed so that the two are told
	// apart: its name clicked, and a new one typed
	must(t, operator.New(server).Command("O"))
	server.RunFrames(600)
	must(t, operator.New(server).MoveTo(472, 66))
	operator.New(server).Click()
	server.RunFrames(120)
	must(t, operator.New(server).TypeKeys("Shift+A", "D", "A", "Quote", "S", "Space", "Shift+D", "I", "S", "K"))
	must(t, operator.New(server).PressKey("Return"))
	server.RunFrames(300)
	must(t, album.ScreenshotOf(server, "disk-window", ada))
	for _, at := range [][2]int{{160, 92}, {358, 92}} {
		must(t, operator.New(server).DoubleClickAt(at[0], at[1]))
		server.RunFrames(900)
	}
	must(t, album.ScreenshotOf(server, "control-panels", ada))
	must(t, operator.New(server).DoubleClickAt(232, 92))
	server.RunFrames(900)
	must(t, album.ScreenshotOf(server, "sharing-setup", ada))
	must(t, operator.New(server).MoveTo(300, 85))
	operator.New(server).Click()
	must(t, operator.New(server).TypeKeys("Shift+A", "D", "A"))
	must(t, operator.New(server).PressKey("Tab"))
	must(t, operator.New(server).TypeString("secret"))
	must(t, operator.New(server).PressKey("Tab"))
	must(t, operator.New(server).TypeKeys("Shift+A", "D", "A", "Quote", "S", "Space", "Shift+M", "A", "C"))
	// Start, recorded at five times the speed, which a Plus takes a minute
	// over
	must(t, operator.New(server).MoveTo(148, 197))
	server.RunFrames(30)
	starting := Record(operator.New(server), ada)
	starting.Capture(50)
	starting.Press(true)
	starting.Run(8, 2)
	starting.Press(false)
	for frame := 0; frame < 3600; frame += 30 {
		server.RunFrames(30)
		starting.Capture(10)
	}
	must(t, album.SaveRecording(starting, "sharing-on", 300))
	must(t, operator.New(server).Command("W"))
	server.RunFrames(300)

	// Control Panels and System Folder closed, back to the disk
	for i := 0; i < 2; i++ {
		must(t, operator.New(server).Command("W"))
		server.RunFrames(300)
	}

	// The Shared folder, shared: File, Sharing..., its box ticked, and the
	// window closed, saving
	must(t, operator.New(server).MoveTo(103, 92))
	operator.New(server).Click()
	server.RunFrames(60)
	must(t, operator.New(server).ChooseFromMenu(55, 123))
	server.RunFrames(900)
	must(t, operator.New(server).MoveTo(26, 106))
	operator.New(server).Click()
	server.RunFrames(120)
	must(t, album.ScreenshotOf(server, "share-folder", ada))
	must(t, operator.New(server).Command("W"))
	server.RunFrames(600)
	must(t, operator.New(server).PressKey("Return"))
	server.RunFrames(600)

	// The Shared folder opened on the first, to see what comes into it
	must(t, operator.New(server).DoubleClickAt(103, 92))
	server.RunFrames(900)

	// From now on the first runs by itself
	stop, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		for {
			select {
			case <-stop:
				return
			default:
				server.RunFrames(5)
			}
		}
	}()
	defer func() {
		select {
		case <-stop:
		default:
			close(stop)
			<-stopped
		}
	}()

	// The second: the Chooser, AppleShare, the first as a guest, the
	// folder
	client := machine()
	client.RunFrames(systemSevenBootFrames)
	must(t, operator.New(client).OpenMenu(16))
	must(t, operator.New(client).MoveTo(16, 78))
	must(t, operator.New(client).MoveTo(60, 78))
	client.RunFrames(10)
	client.SetMouseButton(false)
	client.RunFrames(1800)
	must(t, operator.New(client).MoveTo(84, 75))
	operator.New(client).Click()
	client.RunFrames(1200)
	must(t, operator.New(client).MoveTo(330, 78))
	operator.New(client).Click()
	client.RunFrames(60)
	must(t, album.ScreenshotOf(client, "chooser", grace))
	must(t, operator.New(client).MoveTo(364, 266))
	operator.New(client).Click()
	client.RunFrames(900)

	// As the registered user the first one has, its owner
	must(t, operator.New(client).TypeKeys("Shift+A", "D", "A"))
	must(t, operator.New(client).PressKey("Tab"))
	must(t, operator.New(client).TypeString("secret"))
	must(t, album.ScreenshotOf(client, "connect-as", grace))
	must(t, operator.New(client).MoveTo(390, 258))
	operator.New(client).Click()
	client.RunFrames(900)
	must(t, album.ScreenshotOf(client, "select-items", grace))
	must(t, operator.New(client).MoveTo(336, 258))
	operator.New(client).Click()
	operator.New(client).WaitUntil(60, func() bool { return len(client.MountedVolumes()) == 2 })
	client.RunFrames(600)
	must(t, operator.New(client).MoveTo(34, 32))
	operator.New(client).Click()
	client.RunFrames(1200)
	must(t, album.ScreenshotOf(client, "mounted", grace))

	// A folder made on the desktop of this one, and dragged into the shared
	// folder of the other
	must(t, operator.New(client).Command("N"))
	client.RunFrames(600)
	must(t, operator.New(client).TypeKeys("Shift+F", "R", "O", "M", "Space", "Shift+G", "R", "A", "C", "E"))
	must(t, operator.New(client).PressKey("Return"))
	client.RunFrames(300)
	must(t, operator.New(client).DoubleClickAt(472, 95))
	client.RunFrames(1500)
	must(t, album.ScreenshotOf(client, "remote-disk", grace))

	// The drag recorded, from the pointer on the folder to Shared opened
	// with the folder in it
	must(t, operator.New(client).MoveTo(472, 150))
	client.RunFrames(30)
	copying := Record(operator.New(client), grace)
	copying.Capture(50)
	copying.Press(true)
	copying.Run(20, 2)
	must(t, copying.Glide(103, 92))
	copying.Run(20, 2)
	copying.Press(false)
	copying.Run(1800, 6)
	copying.DoubleClick()
	copying.Run(1200, 6)
	must(t, album.SaveRecording(copying, "copying", 300))

	// And the first, stopped to be looked at, has it in its shared folder
	close(stop)
	<-stopped
	server.RunFrames(1200)
	must(t, album.ScreenshotOf(server, "arrived", ada))
}

/*
showsAQuestionMark says whether the middle of the screen has the disk with the
question mark the Macintosh asks for a disk with, by how much of it is black:
the disk on the grey is a third of it, the question mark a few dots more, and
the grey alone, the black of the screen switched on and the white of the
screen cleared are something else
*/
func showsAQuestionMark(screen *image.RGBA) bool {
	black := 0
	for y := 155; y < 190; y++ {
		for x := 240; x < 272; x++ {
			if screen.RGBAAt(x, y).R == 0 {
				black++
			}
		}
	}
	return black > 370 && black < 420
}

// waitForAQuestionMark runs the machine until it shows the disk with the
// question mark
func waitForAQuestionMark(t *testing.T, m *izmac.Mac) {
	t.Helper()
	for frame := 0; !showsAQuestionMark(m.GetImage()); frame++ {
		if frame > 60*60 {
			t.Fatalf("the Macintosh did not ask for a disk")
		}
		m.RunFrames(1)
	}
}

/*
showsThePage says whether MacPaint has the page it is printing on the screen:
its window has a light grey, a dot in four black, on the left of the page,
where the drawing is white
*/
func showsThePage(screen *image.RGBA) bool {
	black := 0
	for y := 250; y < 270; y++ {
		for x := 100; x < 120; x++ {
			if screen.RGBAAt(x, y).R == 0 {
				black++
			}
		}
	}
	return black > 50 && black < 300
}

package izmac

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/ivanizag/izmac/localtalk"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

/*
The screenshots of the activities in doc/activities, made by taking a machine
through what each page tells its reader to do. Like TestSettleTestImages it is
not a test: it does nothing unless asked for with IZMAC_ACTIVITY_SCREENSHOTS,
and then writes the images of each page in doc/activities/images, so that they
can be made again when the emulator or the instructions change.

	IZMAC_ACTIVITY_SCREENSHOTS=1 go test -run TestActivityScreenshots .
*/

const activityImages = "doc/activities/images"

// screenshot writes the screen of a machine as one of the images of an
// activity, in its frame
func screenshot(t *testing.T, m *Mac, activity string, name string) {
	t.Helper()
	screenshotOf(t, m, activity, name, "")
}

/*
The frame around a screenshot: black, as the glass of the screen of the
Macintosh is around what it shows, so that the corners the ROM rounds off in
black read as round, and with rounded corners of its own. A label, when there
is one, goes in its bottom edge in grey, the way a name is on the front of a
monitor.
*/
const (
	frameWidth  = 14
	frameLabel  = 14
	frameRadius = 12
)

func framed(screen image.Image, label string) *image.RGBA {
	b := screen.Bounds()
	bottom := frameWidth
	if label != "" {
		bottom += frameLabel
	}
	width, height := b.Dx()+2*frameWidth, b.Dy()+frameWidth+bottom
	out := image.NewRGBA(image.Rect(0, 0, width, height))

	// Black, but for what the rounding leaves out of each corner
	corner := func(x int, last int) int {
		return max(frameRadius-x, x-(last-frameRadius), 0)
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			dx, dy := corner(x, width-1), corner(y, height-1)
			if dx*dx+dy*dy <= frameRadius*frameRadius {
				out.Set(x, y, color.Black)
			}
		}
	}
	draw.Draw(out, image.Rect(frameWidth, frameWidth, frameWidth+b.Dx(), frameWidth+b.Dy()),
		screen, b.Min, draw.Src)

	if label != "" {
		face := basicfont.Face7x13
		writer := font.Drawer{
			Dst:  out,
			Src:  image.NewUniform(color.Gray{Y: 0xb0}),
			Face: face,
		}
		textWidth := writer.MeasureString(label).Round()
		writer.Dot = fixed.P((width-textWidth)/2, frameWidth+b.Dy()+frameLabel-1)
		writer.DrawString(label)
	}
	return out
}

// screenshotOf writes the screen of a machine with a label in its frame,
// which says whose it is when there are two
func screenshotOf(t *testing.T, m *Mac, activity string, name string, label string) {
	t.Helper()
	folder := filepath.Join(activityImages, activity)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(folder, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, framed(m.GetImage(), label)); err != nil {
		t.Fatal(err)
	}
}

// openMenu holds a menu of the menu bar open, at the place of its title
func openMenu(t *testing.T, m *Mac, titleH int16) {
	t.Helper()
	moveMouseTo(t, m, titleH, 10)
	m.SetMouseButton(true)
	m.RunFrames(30)
}

// closeMenu lets go of a menu without choosing anything in it, away from it
func closeMenu(t *testing.T, m *Mac) {
	t.Helper()
	moveMouseTo(t, m, 300, 300)
	m.RunFrames(5)
	m.SetMouseButton(false)
	m.RunFrames(30)
}

/*
chooseFromMenu opens a menu and chooses the item at a height in it, going
down the menu first and across after: across the menu bar is into the next
menu
*/
func chooseFromMenu(t *testing.T, m *Mac, titleH int16, itemV int16) {
	t.Helper()
	openMenu(t, m, titleH)
	moveMouseTo(t, m, titleH, itemV)
	moveMouseTo(t, m, titleH+30, itemV)
	m.RunFrames(10)
	m.SetMouseButton(false)
}

func TestActivityScreenshots(t *testing.T) {
	if os.Getenv("IZMAC_ACTIVITY_SCREENSHOTS") == "" {
		t.Skip("this makes the screenshots of doc/activities")
	}
	t.Run("first-steps", firstStepsScreenshots)
	t.Run("macpaint", macPaintScreenshots)
	t.Run("file-sharing", fileSharingScreenshots)
}

/*
firstStepsScreenshots is the 1984 experience: izmac started with nothing, which
is the MacPaint diskette with System 2.0, and the Finder taken through its
desktop, its menus, a desk accessory, Get Info, the Trash and Shut Down
*/
func firstStepsScreenshots(t *testing.T) {
	const page = "first-steps"
	config := testConfig(t)
	config.Diskettes = []string{testImage(t, testPaintDiskette)}
	m := buildTestMac(t, config)
	waitForApplication(t, m, "Finder", 60)
	m.RunFrames(600)
	screenshot(t, m, page, "desktop")

	// The diskette opened
	doubleClickAt(t, m, 472, 45)
	m.RunFrames(600)
	screenshot(t, m, page, "disk-window")

	// The Apple menu, and About the Finder
	openMenu(t, m, 16)
	screenshot(t, m, page, "apple-menu")
	closeMenu(t, m)
	chooseFromMenu(t, m, 16, 27)
	m.RunFrames(300)
	screenshot(t, m, page, "about-the-finder")
	clickMouse(m)
	m.RunFrames(120)

	// The Puzzle, a desk accessory, which shuffles itself each time, and
	// its close box
	chooseFromMenu(t, m, 16, 155)
	m.RunFrames(300)
	screenshot(t, m, page, "puzzle")
	moveMouseTo(t, m, 113, 89)
	clickMouse(m)
	m.RunFrames(120)

	// The window by name, and back to icons
	openMenu(t, m, 135)
	screenshot(t, m, page, "view-menu")
	closeMenu(t, m)
	chooseFromMenu(t, m, 135, 59)
	m.RunFrames(300)
	screenshot(t, m, page, "by-name")
	chooseFromMenu(t, m, 135, 27)
	m.RunFrames(300)

	// Get Info on MacPaint
	moveMouseTo(t, m, 94, 140)
	clickMouse(m)
	m.RunFrames(30)
	pressCommand(m, "I")
	m.RunFrames(300)
	screenshot(t, m, page, "get-info")
	moveMouseTo(t, m, 24, 40)
	clickMouse(m)
	m.RunFrames(120)

	// A new folder, dragged to the Trash, and the Trash emptied
	pressCommand(m, "N")
	m.RunFrames(300)
	screenshot(t, m, page, "new-folder")
	dragTo(t, m, 287, 140, 472, 318)
	m.RunFrames(300)
	screenshot(t, m, page, "in-the-trash")
	openMenu(t, m, 185)
	screenshot(t, m, page, "special-menu")
	closeMenu(t, m)
	chooseFromMenu(t, m, 185, 43)
	m.RunFrames(300)

	// Shut Down, the last item of Special
	chooseFromMenu(t, m, 185, 123)
	m.RunFrames(900)
	screenshot(t, m, page, "shut-down")
}

// dragOn drags with the button held from one place to another, which is how
// MacPaint draws
func dragOn(t *testing.T, m *Mac, fromH, fromV, toH, toV int16) {
	t.Helper()
	moveMouseTo(t, m, fromH, fromV)
	m.SetMouseButton(true)
	m.RunFrames(10)
	moveMouseTo(t, m, (fromH+toH)/2, (fromV+toV)/2)
	moveMouseTo(t, m, toH, toV)
	m.RunFrames(10)
	m.SetMouseButton(false)
	m.RunFrames(30)
}

// pick clicks a tool or a pattern of MacPaint
func pick(t *testing.T, m *Mac, h, v int16) {
	t.Helper()
	moveMouseTo(t, m, h, v)
	clickMouse(m)
	m.RunFrames(20)
}

/*
macPaintScreenshots is a drawing made in MacPaint, from the same diskette:
shapes with patterns, text, FatBits, the page, and the drawing printed on the
ImageWriter, whose page is kept as it came out
*/
func macPaintScreenshots(t *testing.T) {
	const page = "macpaint"
	config := testConfig(t)
	config.Diskettes = []string{testImage(t, testPaintDiskette)}
	config.Printer = printerImageWriter
	config.PrinterFile = filepath.Join(t.TempDir(), "page")
	m := buildTestMac(t, config)
	waitForApplication(t, m, "Finder", 60)
	m.RunFrames(600)
	doubleClickAt(t, m, 472, 45)
	m.RunFrames(600)
	doubleClickAt(t, m, 94, 140)
	waitForApplication(t, m, "MacPaint", 60)
	m.RunFrames(900)
	screenshot(t, m, page, "empty")

	// A rectangle filled with bricks, an oval filled with grey, and an
	// empty rounded rectangle around them
	pick(t, m, 350, 305)
	pick(t, m, 54, 150)
	dragOn(t, m, 110, 70, 220, 150)
	pick(t, m, 150, 322)
	pick(t, m, 54, 194)
	dragOn(t, m, 250, 70, 380, 160)
	pick(t, m, 29, 172)
	dragOn(t, m, 95, 60, 400, 175)
	screenshot(t, m, page, "shapes")

	// Text, and a line with the pencil under it
	pick(t, m, 54, 62)
	moveMouseTo(t, m, 140, 210)
	clickMouse(m)
	typeKeys(m, "Shift+H", "E", "L", "L", "O", "Space", "F", "R", "O", "M", "Space",
		"1", "9", "8", "4", "Shift+1")
	pick(t, m, 54, 106)
	dragOn(t, m, 140, 230, 340, 232)
	screenshot(t, m, page, "drawing")

	// FatBits, the drawing a dot at a time, entered with the command key and
	// a click of the pencil on the edge of the oval, and left the same way
	codes := KeyCodes()
	m.PutKey(codes["Command"], true)
	m.RunFrames(6)
	moveMouseTo(t, m, 250, 115)
	clickMouse(m)
	m.PutKey(codes["Command"], false)
	m.RunFrames(300)
	screenshot(t, m, page, "fatbits")
	chooseFromMenu(t, m, 140, 43)
	m.RunFrames(300)

	// The whole page
	chooseFromMenu(t, m, 140, 59)
	m.RunFrames(300)
	screenshot(t, m, page, "show-page")
	moveMouseTo(t, m, 440, 229)
	clickMouse(m)
	m.RunFrames(300)

	// Print Final, and the page the ImageWriter printed
	chooseFromMenu(t, m, 55, 139)
	m.RunFrames(300)
	screenshot(t, m, page, "printing")
	printed := config.PrinterFile + "_001.png"
	if !waitUntil(m, 300, func() bool { return exists(printed) }) {
		t.Fatalf("MacPaint printed nothing")
	}
	m.RunFrames(600)
	data, err := os.ReadFile(printed)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(activityImages, page, "printed-page.png"), data, 0o644); err != nil {
		t.Fatal(err)
	}
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
	network := localtalk.NewNetwork()
	machine := func() *Mac {
		config := testConfig(t)
		config.DiskFiles = []string{testImage(t, testSystemSevenDisk)}
		config.RamSizeKb = 4096
		config.AppleTalk = appleTalkLocal
		config.PrinterPort = ""
		config.localTalkNetwork = network
		m := buildTestMac(t, config)
		waitForApplication(t, m, "Finder", 120)
		m.RunFrames(600)
		return m
	}

	// The first: Sharing Setup, from the Control Panels of the System Folder
	server := machine()

	// Its disk opened at once, which is when the Finder puts the icons in
	// the order of their names, and renamed so that the two are told
	// apart: its name clicked, and a new one typed
	pressCommand(server, "O")
	server.RunFrames(600)
	moveMouseTo(t, server, 472, 66)
	clickMouse(server)
	server.RunFrames(120)
	typeKeys(server, "Shift+A", "D", "A", "Quote", "S", "Space", "Shift+D", "I", "S", "K")
	pressKey(server, "Return")
	server.RunFrames(300)
	screenshotOf(t, server, page, "disk-window", ada)
	for _, at := range [][2]int16{{160, 92}, {358, 92}} {
		doubleClickAt(t, server, at[0], at[1])
		server.RunFrames(900)
	}
	screenshotOf(t, server, page, "control-panels", ada)
	doubleClickAt(t, server, 232, 92)
	server.RunFrames(900)
	screenshotOf(t, server, page, "sharing-setup", ada)
	moveMouseTo(t, server, 300, 85)
	clickMouse(server)
	typeKeys(server, "Shift+A", "D", "A")
	pressKey(server, "Tab")
	typeText(server, "secret")
	pressKey(server, "Tab")
	typeKeys(server, "Shift+A", "D", "A", "Quote", "S", "Space", "Shift+M", "A", "C")
	moveMouseTo(t, server, 148, 197)
	clickMouse(server)
	server.RunFrames(3600)
	screenshotOf(t, server, page, "sharing-on", ada)
	pressCommand(server, "W")
	server.RunFrames(300)

	// Control Panels and System Folder closed, back to the disk
	for i := 0; i < 2; i++ {
		pressCommand(server, "W")
		server.RunFrames(300)
	}

	// The Shared folder, shared: File, Sharing..., its box ticked, and the
	// window closed, saving
	moveMouseTo(t, server, 103, 92)
	clickMouse(server)
	server.RunFrames(60)
	chooseFromMenu(t, server, 55, 123)
	server.RunFrames(900)
	moveMouseTo(t, server, 26, 106)
	clickMouse(server)
	server.RunFrames(120)
	screenshotOf(t, server, page, "share-folder", ada)
	pressCommand(server, "W")
	server.RunFrames(600)
	pressKey(server, "Return")
	server.RunFrames(600)

	// The Shared folder opened on the first, to see what comes into it
	doubleClickAt(t, server, 103, 92)
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
	openMenu(t, client, 16)
	moveMouseTo(t, client, 16, 78)
	moveMouseTo(t, client, 60, 78)
	client.RunFrames(10)
	client.SetMouseButton(false)
	client.RunFrames(1800)
	moveMouseTo(t, client, 84, 75)
	clickMouse(client)
	client.RunFrames(1200)
	moveMouseTo(t, client, 330, 78)
	clickMouse(client)
	client.RunFrames(60)
	screenshotOf(t, client, page, "chooser", grace)
	moveMouseTo(t, client, 364, 266)
	clickMouse(client)
	client.RunFrames(900)

	// As the registered user the first one has, its owner
	typeKeys(client, "Shift+A", "D", "A")
	pressKey(client, "Tab")
	typeText(client, "secret")
	screenshotOf(t, client, page, "connect-as", grace)
	moveMouseTo(t, client, 390, 258)
	clickMouse(client)
	client.RunFrames(900)
	screenshotOf(t, client, page, "select-items", grace)
	moveMouseTo(t, client, 336, 258)
	clickMouse(client)
	waitUntil(client, 60, func() bool { return len(mountedVolumes(client)) == 2 })
	client.RunFrames(600)
	moveMouseTo(t, client, 34, 32)
	clickMouse(client)
	client.RunFrames(1200)
	screenshotOf(t, client, page, "mounted", grace)

	// A folder made on the desktop of this one, and dragged into the shared
	// folder of the other
	pressCommand(client, "N")
	client.RunFrames(600)
	typeKeys(client, "Shift+F", "R", "O", "M", "Space", "Shift+G", "R", "A", "C", "E")
	pressKey(client, "Return")
	client.RunFrames(300)
	doubleClickAt(t, client, 472, 95)
	client.RunFrames(1500)
	screenshotOf(t, client, page, "remote-disk", grace)
	dragTo(t, client, 472, 150, 103, 92)
	client.RunFrames(1800)
	screenshotOf(t, client, page, "copied", grace)

	// And the first, stopped to be looked at, has it in its shared folder
	close(stop)
	<-stopped
	server.RunFrames(1200)
	screenshotOf(t, server, page, "arrived", ada)
}

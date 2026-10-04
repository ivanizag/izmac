package izmac

import (
	"image"
	"image/color"
	"image/draw"
	"image/gif"
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
	writeScreenshot(t, m.GetImage(), activity, name, label)
}

// writeScreenshot writes a screen, taken before, as one of the images of an
// activity
func writeScreenshot(t *testing.T, screen image.Image, activity string, name string, label string) {
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
	if err := png.Encode(f, framed(screen, label)); err != nil {
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

	// Switched on, recorded from the first frame to the desktop
	starting := record(m, "")
	for frame := 0; currentApplication(m) != "Finder"; frame += 6 {
		if frame > 60*60 {
			t.Fatalf("the Finder did not start")
		}
		starting.run(6, 6)
	}
	starting.run(600, 6)
	starting.save(t, page, "starting", 300)

	// The diskette opened, recorded: the pointer taken to it, the double
	// click, and the window coming out of the icon
	opening := record(m, "")
	opening.capture(100)
	opening.glide(t, 472, 45)
	opening.doubleClick()
	opening.run(600, 6)
	opening.save(t, page, "open-disk", 300)
	screenshot(t, m, page, "disk-window")

	// The Apple menu, recorded: pulled down, gone over to the bottom and back
	// up, and let go of outside, which chooses nothing
	menu := record(m, "")
	menu.capture(50)
	menu.glide(t, 16, 10)
	menu.press(true)
	menu.run(20, 2)
	menu.glide(t, 40, 30)
	menu.glide(t, 40, 170)
	menu.run(20, 2)
	menu.glide(t, 40, 60)
	menu.run(20, 2)
	menu.glide(t, 300, 250)
	menu.press(false)
	menu.run(30, 3)
	menu.save(t, page, "apple-menu", 200)

	// About the Finder
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
	trash := record(m, "")
	trash.capture(50)
	trash.glide(t, 287, 140)
	trash.press(true)
	trash.run(20, 2)
	trash.glide(t, 472, 318)
	trash.run(30, 2)
	trash.press(false)
	trash.run(300, 6)
	trash.save(t, page, "to-the-trash", 300)
	openMenu(t, m, 185)
	screenshot(t, m, page, "special-menu")
	closeMenu(t, m)
	chooseFromMenu(t, m, 185, 43)
	m.RunFrames(300)

	// Shut Down, the last item of Special, and the disk with the question
	// mark that blinks while the machine waits for one
	chooseFromMenu(t, m, 185, 123)
	m.RunFrames(900)
	shutDown := record(m, "")
	shutDown.capture(5)
	shutDown.run(240, 3)
	shutDown.save(t, page, "shut-down", 0)
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

	// A rectangle filled with bricks, recorded as it is dragged out, and an
	// empty oval
	pick(t, m, 350, 305)
	pick(t, m, 54, 150)
	rectangle := record(m, "")
	rectangle.capture(50)
	rectangle.glide(t, 110, 70)
	rectangle.press(true)
	rectangle.run(10, 2)
	rectangle.glide(t, 160, 160)
	rectangle.glide(t, 240, 100)
	rectangle.glide(t, 220, 150)
	rectangle.run(20, 2)
	rectangle.press(false)
	rectangle.run(30, 3)
	rectangle.save(t, page, "rectangle", 200)
	pick(t, m, 29, 194)
	dragOn(t, m, 250, 70, 380, 160)

	// The oval filled with grey by the paint bucket, recorded: the bucket
	// clicked inside it, and the pattern poured in
	pick(t, m, 150, 322)
	pick(t, m, 29, 84)
	moveMouseTo(t, m, 315, 120)
	m.RunFrames(30)
	fill := record(m, "")
	fill.capture(100)
	m.SetMouseButton(true)
	fill.run(6, 2)
	m.SetMouseButton(false)
	fill.run(60, 3)
	fill.save(t, page, "fill", 200)

	// An empty rounded rectangle around both
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

	// The spray can, in black, recorded while it is held down and moved
	// about
	pick(t, m, 127, 306)
	pick(t, m, 54, 84)
	spray := record(m, "")
	spray.capture(50)
	spray.glide(t, 400, 215)
	spray.press(true)
	spray.run(30, 2)
	spray.glide(t, 440, 230)
	spray.run(20, 2)
	spray.glide(t, 410, 250)
	spray.run(20, 2)
	spray.glide(t, 380, 235)
	spray.run(20, 2)
	spray.press(false)
	spray.run(20, 2)
	spray.save(t, page, "spray-can", 200)
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

	// Print Final, and the page MacPaint draws as it sends it to the printer,
	// kept as it was last seen whole, before MacPaint went back to the
	// drawing; the grey around the page is how its window is told from the
	// drawing's
	chooseFromMenu(t, m, 55, 139)
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
	writeScreenshot(t, drawn, page, "printing", "")
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
	// Start, recorded at five times the speed, which a Plus takes a minute
	// over
	starting := record(server, ada)
	starting.capture(50)
	starting.glide(t, 148, 197)
	starting.press(true)
	starting.run(8, 2)
	starting.press(false)
	for frame := 0; frame < 3600; frame += 30 {
		server.RunFrames(30)
		starting.capture(10)
	}
	starting.save(t, page, "sharing-on", 300)
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
	copying := record(client, grace)
	copying.capture(50)
	copying.glide(t, 472, 150)
	copying.press(true)
	copying.run(20, 2)
	copying.glide(t, 103, 92)
	copying.run(20, 2)
	copying.press(false)
	copying.run(1800, 6)
	copying.save(t, page, "copying", 300)

	// And the first, stopped to be looked at, has it in its shared folder
	close(stop)
	<-stopped
	server.RunFrames(1200)
	screenshotOf(t, server, page, "arrived", ada)
}

/*
A recording is an animated screenshot, for what is worth watching move: a GIF
of the screen in its frame, a picture taken every few frames of the machine.
The screen has two colours and changes in little places at a time, so each
picture after the first is only the part of it that changed, and a picture
that changed nothing makes the one before last longer, up to a few seconds.
*/
type recording struct {
	m      *Mac
	label  string
	frames []*image.Paletted
	delays []int
	last   *image.Paletted
}

// recordingPalette is what a framed screenshot is made of: the corners left
// out of the frame, black, white, and the grey of a label
// longestStill is the longest a picture stays, in hundredths of a second: a
// wait on the machine with nothing changing is shortened to that
const longestStill = 300

func recordingPalette() color.Palette {
	return color.Palette{
		color.Transparent, color.Black, color.White, color.Gray{Y: 0xb0},
	}
}

func record(m *Mac, label string) *recording {
	return &recording{m: m, label: label}
}

// capture takes a picture of the screen, shown for a time in hundredths of a
// second
func (r *recording) capture(delay int) {
	whole := framed(r.m.GetImage(), r.label)
	picture := image.NewPaletted(whole.Bounds(), recordingPalette())
	for i := 0; i < len(whole.Pix); i += 4 {
		var index uint8
		switch {
		case whole.Pix[i+3] == 0:
			index = 0
		case whole.Pix[i] == 0:
			index = 1
		case whole.Pix[i] == 0xff:
			index = 2
		default:
			index = 3
		}
		picture.Pix[i/4] = index
	}

	if r.last == nil {
		r.frames, r.delays, r.last = append(r.frames, picture), append(r.delays, delay), picture
		return
	}

	// Only the part that changed, if anything did
	changed := image.Rectangle{}
	bounds := picture.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if picture.ColorIndexAt(x, y) != r.last.ColorIndexAt(x, y) {
				changed = changed.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	r.last = picture
	if changed.Empty() {
		r.delays[len(r.delays)-1] = min(r.delays[len(r.delays)-1]+delay, longestStill)
		return
	}
	part := image.NewPaletted(changed, recordingPalette())
	draw.Draw(part, changed, picture, changed.Min, draw.Src)
	r.frames, r.delays = append(r.frames, part), append(r.delays, delay)
}

// run runs the machine for some frames, taking a picture every few of them
func (r *recording) run(frames int, every int) {
	for done := 0; done < frames; done += every {
		r.m.RunFrames(uint64(every))
		r.capture(every * 100 / 60)
	}
}

/*
glide takes the pointer to a place a little at a time, as a hand would, a
picture at each step
*/
func (r *recording) glide(t *testing.T, toH, toV int16) {
	t.Helper()
	const stepPixels = 12
	fromH, fromV := pointerAt(r.m)
	distance := max(abs(int(toH-fromH)), abs(int(toV-fromV)))
	steps := distance/stepPixels + 1
	for step := 1; step <= steps; step++ {
		moveMouseTo(t, r.m, fromH+int16(int(toH-fromH)*step/steps), fromV+int16(int(toV-fromV)*step/steps))
		r.capture(3)
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// press presses or lets go of the button
func (r *recording) press(down bool) {
	r.m.SetMouseButton(down)
	r.run(2, 2)
}

// doubleClick clicks twice in a row where the pointer is
func (r *recording) doubleClick() {
	for i := 0; i < 2; i++ {
		r.press(true)
		r.run(6, 2)
		r.press(false)
		r.run(6, 2)
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

// save writes the recording as one of the images of an activity, the last
// picture held a while before it starts again
func (r *recording) save(t *testing.T, activity string, name string, hold int) {
	t.Helper()
	if len(r.delays) == 0 {
		t.Fatalf("the recording %v has nothing in it", name)
	}
	r.delays[len(r.delays)-1] += hold
	folder := filepath.Join(activityImages, activity)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(folder, name+".gif"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := gif.EncodeAll(f, &gif.GIF{Image: r.frames, Delay: r.delays}); err != nil {
		t.Fatal(err)
	}
}

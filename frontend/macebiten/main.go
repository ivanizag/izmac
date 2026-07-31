package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/ivanizag/izmac"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/pkg/profile"
)

const windowScale = 2

// game drives the ebiten main loop. The emulation runs on its own goroutine,
// this one only reads the frame buffer and pushes the input.
type game struct {
	m         *izmac.Mac
	image     *ebiten.Image
	keyboard  *ebitenKeyboard
	mouse     *ebitenMouse
	clipboard *ebitenClipboard
	menu      *menu
	title     string

	// The size of the screen of the machine, taken from the image it hands
	// over rather than asked for separately. Layout is called on every
	// frame, so it is kept rather than looked up again.
	width  int
	height int

	updates uint64
	paused  bool
}

func (g *game) Update() error {
	// The menu takes the keys and the pointer while it is up, so that the
	// machine does not also get them
	if !g.menu.update() {
		g.keyboard.update()
		g.mouse.update()
	}

	// A diskette image dropped on the window goes in a drive
	if filename, dropped := pathOfDropped(ebiten.DroppedFiles()); dropped {
		message := insertDropped(g.m, filename)
		g.menu.say(message)
	}

	g.clipboard.update()

	if g.paused != g.m.IsPaused() {
		g.paused = g.m.IsPaused()
		ebiten.SetWindowTitle(g.windowTitle())
	}

	if !g.m.IsPaused() {
		img := g.m.GetImage()
		g.image.WritePixels(img.Pix)
	}

	if g.updates%60 == 0 {
		ebiten.SetWindowTitle(g.windowTitle())
	}
	g.updates++

	return nil
}

func (g *game) Draw(dst *ebiten.Image) {
	dst.DrawImage(g.image, nil)
	g.menu.draw(dst)
}

func (g *game) Layout(outsideWidth int, outsideHeight int) (int, int) {
	return g.width, g.height
}

func (g *game) windowTitle() string {
	if g.m.IsPaused() {
		return g.title + " - PAUSED"
	}
	speed := fmt.Sprintf("%0.2f MHz", g.m.GetCurrentFreqMHz())
	if g.m.IsFullSpeed() {
		speed += ", full speed"
	}
	return fmt.Sprintf("%v - %v - %v", g.title, speed, g.mouse.hint())
}

func main() {
	config := izmac.NewConfiguration()
	err := config.ParseFlags(os.Args[0], os.Args[1:], os.Stderr)
	if izmac.IsHelpRequested(err) {
		os.Exit(0)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	m, err := izmac.NewMac(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	for _, line := range m.Summary() {
		fmt.Println(line)
	}

	if m.IsProfiling() {
		defer profile.Start().Stop()
	}

	fmt.Print(keyHelp())

	err = ebitenRun(m)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// keyHelp is what the frontend says as it starts. The keys that stand in for
// the two the Macintosh keyboard has and a modern one does not are named
// after the host, since they are not called the same thing on all of them.
func keyHelp() string {
	return `
     F10: Open the menu
     F11: Force the clipboard of the host into the machine
      F5: Full speed on and off
 Ctrl-F5: Report the speed reached
      F4: Show or hide the processor trace
 Ctrl-F2: Reset
   Pause: Stop the machine and let it go again

Click on the window to use the mouse, right click to get the pointer back.
` + modifierHelp()
}

// modifierHelp names the keys of the host that the command and option keys of
// the Macintosh are reached with. Two of the host's stand for the command key
// because the host keeps some of its own combinations.
func modifierHelp() string {
	switch runtime.GOOS {
	case "darwin":
		return "The command and option keys are the command key of the Macintosh," +
			" and control is its option key.\n"
	case "windows":
		return "The windows and alt keys are the command key of the Macintosh," +
			" and control is its option key.\n"
	default:
		return "The super and alt keys are the command key of the Macintosh," +
			" and control is its option key.\n"
	}
}

func ebitenRun(m *izmac.Mac) error {
	sound, err := newEbitenAudio(m)
	if err != nil {
		return err
	}

	mouse := newEbitenMouse(m)
	keyboard := newEbitenKeyboard(m)
	clip := newEbitenClipboard(m)
	menu, err := newMenu(m, mouse, keyboard, clip)
	if err != nil {
		return err
	}

	size := m.GetImage().Bounds().Size()

	g := &game{
		m:         m,
		image:     ebiten.NewImage(size.X, size.Y),
		keyboard:  keyboard,
		mouse:     mouse,
		clipboard: clip,
		menu:      menu,
		title:     "iz" + m.Name,
		width:     size.X,
		height:    size.Y,
	}

	ebiten.SetWindowSize(size.X*windowScale, size.Y*windowScale)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowTitle(g.title)

	sound.start()

	go m.Run()
	defer m.SendCommand(izmac.CommandKill)

	return ebiten.RunGame(g)
}

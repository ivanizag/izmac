package main

import (
	"fmt"
	"os"
	"time"

	"github.com/ivanizag/izmac"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/pkg/profile"
)

const windowScale = 2

// game drives the ebiten main loop. The emulation runs on its own goroutine,
// this one only reads the frame buffer and pushes the input.
type game struct {
	m        *izmac.Mac
	image    *ebiten.Image
	keyboard *ebitenKeyboard
	mouse    *ebitenMouse
	menu     *menu
	title    string

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

	fmt.Print(keyHelp)

	err = ebitenRun(m)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

const keyHelp = `
     F10: Open the menu
      F5: Full speed on and off
 Ctrl-F5: Report the speed reached
      F4: Show or hide the processor trace
 Ctrl-F2: Reset
   Pause: Stop the machine and let it go again

Click on the window to use the mouse, right click to get the pointer back.
The option key is the Macintosh command key, and control is option.
`

func ebitenRun(m *izmac.Mac) error {
	sound, err := newEbitenAudio(m)
	if err != nil {
		return err
	}

	mouse := newEbitenMouse(m)
	keyboard := newEbitenKeyboard(m)
	menu, err := newMenu(m, mouse, keyboard)
	if err != nil {
		return err
	}

	size := m.GetImage().Bounds().Size()

	g := &game{
		m:        m,
		image:    ebiten.NewImage(size.X, size.Y),
		keyboard: keyboard,
		mouse:    mouse,
		menu:     menu,
		title:    "iz" + m.Name,
		width:    size.X,
		height:   size.Y,
	}

	ebiten.SetWindowSize(size.X*windowScale, size.Y*windowScale)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowTitle(g.title)

	sound.start()

	go m.Run()
	defer stopMachine(m)

	return ebiten.RunGame(g)
}

/*
shutdownWait is how long the window waits for the emulation to stop once it
has been asked to. The run loop looks at the command channel every scan line
of instructions and throttles for at most a tenth of a second between, so this
is far longer than it can honestly take.
*/
const shutdownWait = 2 * time.Second

/*
stopMachine asks the emulation to stop and waits for it to have stopped.

Waiting is the point. Stopping is where a diskette the machine has changed is
written back to the host, and the emulation runs on its own goroutine: asking
and walking away would let the process end first and lose the diskette.
*/
func stopMachine(m *izmac.Mac) {
	m.SendCommand(izmac.CommandKill)

	if !m.WaitUntilStopped(shutdownWait) {
		fmt.Fprintln(os.Stderr, "Warning: the emulation did not stop when asked, "+
			"anything a diskette had not yet been given back may be lost")
	}
}

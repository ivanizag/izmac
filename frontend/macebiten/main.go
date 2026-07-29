package main

import (
	"fmt"
	"os"

	"github.com/ivanizag/izmac"
	"github.com/ivanizag/izmac/screen"

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

	if g.paused != g.m.IsPaused() {
		g.paused = g.m.IsPaused()
		ebiten.SetWindowTitle(g.windowTitle())
	}

	if !g.m.IsPaused() {
		img := screen.Snapshot(g.m.GetVideoSource())
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
	return screen.Width, screen.Height
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
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	m, err := izmac.NewMac(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(m.RomDescription())
	if warning := m.RomWarning(); warning != "" {
		fmt.Printf("Warning: %v\n", warning)
	}
	for _, disk := range m.GetDisks() {
		fmt.Printf("SCSI %v: %v, %v blocks\n", disk.Id, disk.Name, disk.Blocks)
	}
	for _, warning := range m.MediaWarnings() {
		fmt.Printf("Warning: %v\n", warning)
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

Click on the window to use the mouse, escape to get the pointer back.
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

	g := &game{
		m:        m,
		image:    ebiten.NewImage(screen.Width, screen.Height),
		keyboard: keyboard,
		mouse:    mouse,
		menu:     menu,
		title:    "iz" + m.Name,
	}

	ebiten.SetWindowSize(screen.Width*windowScale, screen.Height*windowScale)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowTitle(g.title)

	sound.start()

	go m.Run()
	defer m.SendCommand(izmac.CommandKill)

	return ebiten.RunGame(g)
}

package main

import (
	"bytes"
	"fmt"
	"image/color"
	"image/png"
	"os"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/examples/resources/fonts"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/ivanizag/izmac"
	"github.com/ivanizag/izmac/screen"
)

/*
A menu drawn over the screen of the machine, for the things that are about the
emulator rather than about the Macintosh.

It is drawn rather than being a menu of the host because ebiten has no menus
of its own, and it is on F10 because that is a key the Macintosh keyboard does
not have and so cannot want. Opening it lets go of the mouse, since the
pointer is otherwise captured by the machine and could not reach it.
*/
type menu struct {
	m        *izmac.Mac
	mouse    *ebitenMouse
	keyboard *ebitenKeyboard

	open     bool
	selected int
	items    []menuItem

	face *text.GoTextFace

	// message is shown for a moment after an item does something
	message   string
	messageAt time.Time
}

// menuItem is a line of the menu. The label is worked out when it is drawn so
// that it can say what the item will do rather than what it did.
type menuItem struct {
	label  func(m *izmac.Mac) string
	action func(mn *menu)
}

const (
	menuKey = ebiten.KeyF10

	menuLineHeight = 16
	menuPadding    = 8
	menuWidth      = 176
	menuTextSize   = 11

	// messageLinger is how long a line stays up after an item is chosen
	messageLinger = 2 * time.Second
)

func newMenu(m *izmac.Mac, mouse *ebitenMouse, keyboard *ebitenKeyboard) (*menu, error) {
	source, err := text.NewGoTextFaceSource(bytes.NewReader(fonts.MPlus1pRegular_ttf))
	if err != nil {
		return nil, err
	}

	return &menu{
		m:        m,
		mouse:    mouse,
		keyboard: keyboard,
		face:     &text.GoTextFace{Source: source, Size: menuTextSize},
		items:    menuItems(),
	}, nil
}

func menuItems() []menuItem {
	return []menuItem{
		{
			label: func(m *izmac.Mac) string {
				if m.IsFullSpeed() {
					return "Normal speed"
				}
				return "Full speed"
			},
			action: func(mn *menu) {
				mn.m.SendCommand(izmac.CommandToggleSpeed)
				mn.open = false
			},
		},
		{
			label: func(m *izmac.Mac) string { return "Save a screenshot" },
			action: func(mn *menu) {
				mn.say(mn.saveScreenshot())
				mn.open = false
			},
		},
		{
			label: func(m *izmac.Mac) string { return "Reset" },
			action: func(mn *menu) {
				mn.m.SendCommand(izmac.CommandReset)
				mn.open = false
			},
		},
		{
			label:  func(m *izmac.Mac) string { return "Close this menu" },
			action: func(mn *menu) { mn.open = false },
		},
	}
}

// update takes the keys and the clicks the menu wants. It answers whether it
// has them, so that the machine does not also get them.
func (mn *menu) update() bool {
	if inpututil.IsKeyJustPressed(menuKey) {
		mn.toggle()
		return true
	}
	if !mn.open {
		return false
	}

	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyArrowDown):
		mn.selected = (mn.selected + 1) % len(mn.items)
	case inpututil.IsKeyJustPressed(ebiten.KeyArrowUp):
		mn.selected = (mn.selected + len(mn.items) - 1) % len(mn.items)
	case inpututil.IsKeyJustPressed(ebiten.KeyEnter),
		inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter),
		inpututil.IsKeyJustPressed(ebiten.KeySpace):
		mn.items[mn.selected].action(mn)
	case inpututil.IsKeyJustPressed(ebiten.KeyEscape):
		mn.open = false
	}

	// The pointer picks a line, and a click takes it
	if _, y := ebiten.CursorPosition(); y >= menuPadding {
		if line := (y - menuPadding) / menuLineHeight; line < len(mn.items) {
			mn.selected = line
			if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
				mn.items[mn.selected].action(mn)
			}
		}
	}

	return true
}

func (mn *menu) toggle() {
	mn.open = !mn.open
	if mn.open {
		// The machine has the pointer otherwise, and the menu needs it.
		// Anything held on the keyboard is let go of too, since its
		// release would not be seen while the menu has the keys.
		mn.mouse.release()
		mn.keyboard.releaseAll()
	}
}

func (mn *menu) say(message string) {
	mn.message = message
	mn.messageAt = time.Now()
}

// draw puts the menu, or whatever it last had to say, over the screen
func (mn *menu) draw(dst *ebiten.Image) {
	if !mn.open {
		if mn.message != "" && time.Since(mn.messageAt) < messageLinger {
			mn.drawLine(dst, mn.message, menuPadding, menuPadding, false)
		}
		return
	}

	height := len(mn.items)*menuLineHeight + menuPadding*2
	panel := ebiten.NewImage(menuWidth, height)
	panel.Fill(color.RGBA{0, 0, 0, 255})
	dst.DrawImage(panel, &ebiten.DrawImageOptions{})

	for i, item := range mn.items {
		y := menuPadding + i*menuLineHeight
		mn.drawLine(dst, item.label(mn.m), menuPadding, y, i == mn.selected)
	}
}

func (mn *menu) drawLine(dst *ebiten.Image, line string, x int, y int, selected bool) {
	if selected {
		line = "> " + line
	} else {
		line = "  " + line
	}

	op := &text.DrawOptions{}
	op.GeoM.Translate(float64(x), float64(y))
	op.ColorScale.ScaleWithColor(color.RGBA{255, 255, 255, 255})
	text.Draw(dst, line, mn.face, op)
}

/*
saveScreenshot writes the screen as it is now, named for the moment it was
taken so that one does not overwrite the last.
*/
func (mn *menu) saveScreenshot() string {
	name := fmt.Sprintf("izmac-%v.png", time.Now().Format("20060102-150405"))

	file, err := os.Create(name)
	if err != nil {
		return fmt.Sprintf("Could not save: %v", err)
	}
	defer file.Close()

	err = png.Encode(file, screen.Snapshot(mn.m.GetVideoSource()))
	if err != nil {
		return fmt.Sprintf("Could not save: %v", err)
	}

	return "Saved " + name
}

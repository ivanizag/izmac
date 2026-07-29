package main

import (
	"github.com/ivanizag/izmac"

	"github.com/hajimehoshi/ebiten/v2"
)

/*
The mouse of the host handed to the machine as movement rather than as a
position. The Macintosh mouse is a relative device: the ROM counts quadrature
transitions and keeps the pointer itself, so there is nothing to tell it
where the pointer ought to be, only how far it has travelled.

The pointer is captured so that the host's own one does not run off the
window and so that the movement keeps coming when it reaches the edge of the
screen. Escape gives it back.
*/
type ebitenMouse struct {
	m *izmac.Mac

	captured bool
	lastX    int
	lastY    int
}

func newEbitenMouse(m *izmac.Mac) *ebitenMouse {
	return &ebitenMouse{m: m}
}

func (mo *ebitenMouse) update() {
	if !mo.captured {
		// Clicking on the window takes the pointer
		if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
			mo.capture()
		}
		return
	}

	if ebiten.IsKeyPressed(ebiten.KeyEscape) {
		mo.release()
		return
	}

	x, y := ebiten.CursorPosition()
	dx, dy := x-mo.lastX, y-mo.lastY
	mo.lastX, mo.lastY = x, y

	if dx != 0 || dy != 0 {
		mo.m.MoveMouse(dx, dy)
	}

	mo.m.SetMouseButton(ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft))
}

func (mo *ebitenMouse) capture() {
	ebiten.SetCursorMode(ebiten.CursorModeCaptured)
	mo.lastX, mo.lastY = ebiten.CursorPosition()
	mo.captured = true
}

func (mo *ebitenMouse) release() {
	ebiten.SetCursorMode(ebiten.CursorModeVisible)
	mo.m.SetMouseButton(false)
	mo.captured = false
}

// hint is what to put on the title bar to say how to get the pointer back,
// or how to give it over
func (mo *ebitenMouse) hint() string {
	if mo.captured {
		return "esc releases the mouse"
	}
	return "click to use the mouse"
}

package main

import (
	"github.com/ivanizag/izmac"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

/*
The mouse of the host handed to the machine, one of the two ways it can be.

The machine takes it either way: SetMousePosition for where the host has its
pointer, MoveMouse for how far it has moved. Which of the two is wanted is the
machine's to say, IsAbsoluteMouse, since the command line and the menu both
change it.

What is left for here is the window. A position wants the host pointer hidden
while it is over the screen, since the machine draws one under it, and a
movement wants it captured, so that it does not run off the window and keeps
coming once it reaches the edge of the screen. Escape and the secondary button
give it back: the Macintosh has one button and no escape key, so neither is
anything the machine wants.
*/
type ebitenMouse struct {
	m *izmac.Mac

	// The screen of the machine, taken from the image it hands over as the
	// window itself is, to tell a pointer over it from one somewhere else
	width  int
	height int

	// captured is the host pointer held by the window, and lastX and lastY
	// where it was when it was last looked at
	captured bool
	lastX    int
	lastY    int

	// hidden is the host pointer taken out of sight under the machine's
	hidden bool

	// down is the button as the machine was last told of it, so that a drag
	// that leaves the window is released rather than left held
	down bool
}

func newEbitenMouse(m *izmac.Mac) *ebitenMouse {
	size := m.GetImage().Bounds().Size()

	return &ebitenMouse{
		m:      m,
		width:  size.X,
		height: size.Y,
	}
}

func (mo *ebitenMouse) update() {
	if mo.m.IsAbsoluteMouse() {
		mo.updatePlaced()
		return
	}
	mo.updatePushed()
}

// updatePlaced tells the machine where the host has its pointer, and hides it
// while it is over the screen since the machine draws one under it
func (mo *ebitenMouse) updatePlaced() {
	if mo.captured {
		// Left over from the other way round, the pointer goes back
		mo.release()
	}

	x, y := ebiten.CursorPosition()
	over := mo.overScreen(x, y)
	mo.hideCursor(over)

	pressed := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	if !over && !mo.down {
		// A press that started somewhere else on the host is not the
		// machine's to hear about
		pressed = false
	}

	// A drag that has left the screen goes on being reported, and the machine
	// holds it against the edge it left by
	if over || pressed {
		mo.m.SetMousePosition(x, y)
	}

	mo.down = pressed
	mo.m.SetMouseButton(pressed)
}

// updatePushed hands the machine the movement of the host's pointer, which it
// has to hold on to the pointer to keep receiving
func (mo *ebitenMouse) updatePushed() {
	if !mo.captured {
		// Nothing of the machine's is being drawn over it now
		mo.hideCursor(false)

		// Clicking on the window takes the pointer
		if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
			mo.capture()
		}
		return
	}

	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) ||
		ebiten.IsKeyPressed(ebiten.KeyEscape) {
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

// overScreen tells whether the host pointer is over the screen of the machine.
// A window without the focus is never it: ebiten goes on reporting a position
// over a window that is behind another one, and a pointer being used on
// something else is not the machine's to follow or to hide.
func (mo *ebitenMouse) overScreen(x int, y int) bool {
	return ebiten.IsFocused() &&
		x >= 0 && x < mo.width && y >= 0 && y < mo.height
}

// hideCursor takes the host pointer out of sight and gives it back, and only
// says so to ebiten as it changes
func (mo *ebitenMouse) hideCursor(hide bool) {
	if hide == mo.hidden {
		return
	}

	mode := ebiten.CursorModeVisible
	if hide {
		mode = ebiten.CursorModeHidden
	}

	ebiten.SetCursorMode(mode)
	mo.hidden = hide
}

func (mo *ebitenMouse) capture() {
	ebiten.SetCursorMode(ebiten.CursorModeCaptured)
	mo.lastX, mo.lastY = ebiten.CursorPosition()
	mo.captured = true
	mo.hidden = false
}

// release gives the host pointer back, captured or hidden. The menu asks for
// it as it opens, since it needs a pointer to be clicked with.
func (mo *ebitenMouse) release() {
	ebiten.SetCursorMode(ebiten.CursorModeVisible)
	mo.hidden = false
	mo.captured = false
	mo.down = false
	mo.m.SetMouseButton(false)
}

// hint is what to put on the title bar to say how to give the pointer over or
// get it back. A machine taking a position needs neither and says nothing.
func (mo *ebitenMouse) hint() string {
	if mo.m.IsAbsoluteMouse() {
		return ""
	}
	if mo.captured {
		return "right click releases the mouse"
	}
	return "click to use the mouse"
}

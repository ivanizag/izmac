package main

import (
	"github.com/ivanizag/izmac"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

/*
The mouse of the host handed to the machine, one of the two ways it can be.

The Macintosh mouse is a relative device: the ROM counts quadrature
transitions and keeps the pointer itself, so the hardware has nowhere to put a
position and the honest way to drive it is to push it by the movement of the
host's. That needs the pointer captured, so that the host's own one does not
run off the window and so that the movement keeps coming once it reaches the
edge of the screen. The secondary button gives it back, and so does escape:
the Macintosh has one button and no escape key, so neither is anything the
machine wants.

The other way goes around the hardware and writes the position into the low
memory the ROM keeps the pointer in, which is what izmac does by default and
what pointer.go explains. Then the pointer of the machine is always under the
pointer of the host, there is nothing to capture and nothing to give back, and
the host's own pointer is hidden while it is over the screen so that only one
of the two is seen.

Which of the two it is belongs to the machine rather than to this: the menu
switches it while it runs and the command line sets it to start with.
*/
type ebitenMouse struct {
	m *izmac.Mac

	// The size of the screen of the machine, to tell a pointer over it from
	// one somewhere else. It is taken from the image the machine hands over,
	// as the window itself is.
	width  int
	height int

	// captured is the pointer taken by the machine, which only the pushed
	// mouse does, and lastX and lastY where it was when it was last looked at
	captured bool
	lastX    int
	lastY    int

	// hidden is the pointer of the host taken out of sight under the one of
	// the machine, which only the placed mouse does
	hidden bool

	/*
		down is the button as the machine was last told of it. A press only
		counts while the pointer is over the screen, so that a click meant for
		something else on the host does not go through, and a release counts
		wherever it happens, so that a drag out of the window is not left
		held down.
	*/
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

/*
updatePlaced puts the pointer of the machine where the host has its own. There
is nothing to capture: what the machine is given is a position, and the
pointer of the host is only hidden while it is over the screen so that the two
are not both drawn.
*/
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

	/*
		The position follows the pointer over the screen, and goes on
		following it through a drag that has left it: the machine holds it
		against the edge it left by, which is where a drag off the window is
		pulling from, rather than dropping it where it happened to cross.
	*/
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
		// The pointer of the host was under the one of the machine while it
		// was being placed, and there is nothing to hide it under now
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

/*
overScreen tells whether the pointer of the host is over the screen of the
machine rather than somewhere else on the host.

A window that does not have the focus is somewhere else, wherever the pointer
happens to be. Ebiten goes on reporting a position over a window that is
behind another one, and the machine has no business following a pointer that
is being used on something else, or hiding it from whatever is in front.
*/
func (mo *ebitenMouse) overScreen(x int, y int) bool {
	return ebiten.IsFocused() &&
		x >= 0 && x < mo.width && y >= 0 && y < mo.height
}

/*
hideCursor takes the pointer of the host out of sight while it is over the
screen, since the machine draws its own one under it, and gives it back as it
leaves.

The mode is only set as it changes. Ebiten takes it on every frame without
complaining, but the frames where nothing has moved are the ones with nothing
to say.
*/
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

/*
release gives the pointer of the host back, however the machine was holding
it. The menu asks for this as it opens: it is drawn over the screen of the
machine and needs a pointer to be clicked with.
*/
func (mo *ebitenMouse) release() {
	ebiten.SetCursorMode(ebiten.CursorModeVisible)
	mo.hidden = false
	mo.captured = false
	mo.down = false
	mo.m.SetMouseButton(false)
}

/*
hint is what to put on the title bar to say how to get the pointer back, or
how to give it over. A placed pointer needs neither and says nothing: it is
already where the host is pointing and there is nothing to be told.
*/
func (mo *ebitenMouse) hint() string {
	if mo.m.IsAbsoluteMouse() {
		return ""
	}
	if mo.captured {
		return "right click releases the mouse"
	}
	return "click to use the mouse"
}

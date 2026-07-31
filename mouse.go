package izmac

/*
The mouse, which is split across two chips. Each axis has a pair of square
waves ninety degrees out of phase: the interrupt signals X1 and Y1 go to the
data carrier detect inputs of the SCC, the quadrature signals X2 and Y2 to the
bits 4 and 5 of the VIA port B, and the button to the bit 3.

The ROM works out the direction from which of the two is ahead. Inside
Macintosh volume III, page III-27, states it as a table:

	interrupt edge   quadrature   direction
	positive         low          left or down
	positive         high         right or up
	negative         low          right or up
	negative         high         left or down

So the two lines are generated as a phase that walks one way for left and
down and the other for right and up:

	phase   0     1     2     3
	X1      low   high  high  low
	X2      low   low   high  high

which puts the quadrature low on the positive edge of the interrupt while the
phase is rising, and that is the left and down direction.

Two things about the pacing matter more than they look.

The ROM reads the quadrature from the VIA **when it services the interrupt**,
and the thing to hold the phase for is that read and nothing earlier. The
dispatch resets the external status of the SCC first and only then jumps to
the mouse handler that reads the port, so releasing the axis when the SCC
interrupt is cleared lets a scan line tick land in the gap between the two
and move the level out from under the handler. Every interrupt still arrives
and is answered; about half of them are simply read with the wrong level and
the direction comes out as noise.

So an axis stands still from the edge until the port the quadrature is on has
been read.

And a pixel of the host is one transition of the interrupt line, not one
phase. The line only changes on two of the four phases, so a phase per pixel
delivers half the movement asked for and the pointer feels heavy.
*/
type mouse struct {
	// The movement waiting to be paid out, in quadrature steps
	pendingX int
	pendingY int

	// The phase of each axis, from 0 to 3
	phaseX int
	phaseY int

	// An edge is waiting for the processor to read the level that goes
	// with it, and the axis holds still until it has
	edgeX bool
	edgeY bool

	button bool
}

func newMouse() *mouse {
	return &mouse{}
}

// move adds to the movement waiting to be reported. Positive is right and
// down, the way a screen is measured.
func (m *mouse) move(dx int, dy int) {
	m.pendingX = clampPending(m.pendingX + dx*quadratureStepsPerPixel)
	m.pendingY = clampPending(m.pendingY + dy*quadratureStepsPerPixel)
}

const (
	// quadratureStepsPerPixel is how many phases make one transition of
	// the interrupt line, which is what the ROM counts as a unit of travel
	quadratureStepsPerPixel = 2

	// pendingLimit keeps a fling of the mouse from taking a visible time
	// to drain afterwards
	pendingLimit = 2048
)

func clampPending(pending int) int {
	if pending > pendingLimit {
		return pendingLimit
	}
	if pending < -pendingLimit {
		return -pendingLimit
	}
	return pending
}

// setButton reports the state of the button and says whether that changed it,
// which is the transition a double click is measured between
func (m *mouse) setButton(pressed bool) bool {
	changed := m.button != pressed
	m.button = pressed
	return changed
}

/*
tick pays out one step of each axis and returns the levels of the four lines.
The X1 and Y1 it returns are what the SCC sees, the X2 and Y2 what the VIA
does.

An axis only moves while its interrupt has been answered. Stepping on top of
an edge the ROM has not looked at yet changes the quadrature under it, and
the direction it reads is then not the one the mouse went.
*/
func (m *mouse) tick(canStepX bool, canStepY bool) (x1 bool, x2 bool, y1 bool, y2 bool) {
	if canStepX && !m.edgeX {
		var edge bool
		m.phaseX, edge = stepPhase(m.phaseX, &m.pendingX, true)
		m.edgeX = edge
	}
	if canStepY && !m.edgeY {
		var edge bool
		m.phaseY, edge = stepPhase(m.phaseY, &m.pendingY, false)
		m.edgeY = edge
	}

	x1, x2 = phaseLevels(m.phaseX)
	y1, y2 = phaseLevels(m.phaseY)
	return x1, x2, y1, y2
}

/*
quadratureRead says the processor has read the port the quadrature lines are
on, which is the mouse handler taking the level that goes with the edge it
was called for. The axes are free to move again after it.
*/
func (m *mouse) quadratureRead() {
	m.edgeX = false
	m.edgeY = false
}

/*
stepPhase moves one axis a single step towards where the host says the mouse
is, and says whether that step moved the interrupt line, which is what the
processor has to be given time to read. Right and up walk the phase down and left and down walk it up, which is
what puts the quadrature on the side of the interrupt edge the ROM reads as
that direction. The y axis of the screen counts downwards, so it is the one
the other way round.
*/
func stepPhase(phase int, pending *int, horizontal bool) (int, bool) {
	if *pending == 0 {
		return phase, false
	}

	forward := *pending > 0
	if horizontal {
		// Walking the phase up reads as left and down, so right is the
		// way back down it. The screen counts downwards, which puts down
		// on the same side as left and leaves only this axis to invert.
		forward = !forward
	}

	if *pending > 0 {
		*pending--
	} else {
		*pending++
	}

	next := (phase + 3) & 3
	if forward {
		next = (phase + 1) & 3
	}

	before, _ := phaseLevels(phase)
	after, _ := phaseLevels(next)
	return next, before != after
}

// phaseLevels returns the interrupt and quadrature lines of a phase
func phaseLevels(phase int) (interrupt bool, quadrature bool) {
	return phase == 1 || phase == 2, phase == 2 || phase == 3
}

func (m *mouse) reset() {
	m.pendingX, m.pendingY = 0, 0
	m.phaseX, m.phaseY = 0, 0
	m.edgeX, m.edgeY = false, false
	m.button = false
}

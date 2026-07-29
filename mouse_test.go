package izmac

import "testing"

/*
The direction the ROM reads out of the two lines, from the table on page
III-27 of Inside Macintosh volume III: on a positive edge of the interrupt
signal a low quadrature means left or down and a high one means right or up,
and on a negative edge it is the other way round.

readDirection watches a run of ticks and reports what the ROM would make of
them, so the tests can check the direction the way the machine does rather
than by looking at the phase counter.
*/
func readDirection(m *mouse, ticks int, horizontal bool) int {
	var previous bool
	if horizontal {
		previous, _, _, _ = m.tick(true, true)
		m.quadratureRead()
	} else {
		_, _, previous, _ = m.tick(true, true)
	}

	direction := 0
	for i := 0; i < ticks; i++ {
		var interrupt, quadrature bool
		if horizontal {
			interrupt, quadrature, _, _ = m.tick(true, true)
		} else {
			_, _, interrupt, quadrature = m.tick(true, true)
		}
		m.quadratureRead()

		if interrupt == previous {
			continue
		}

		positiveEdge := interrupt && !previous
		previous = interrupt

		// Positive edge with a low quadrature, or negative with a high
		// one, is left or down
		if positiveEdge == !quadrature {
			direction--
		} else {
			direction++
		}
	}
	return direction
}

func TestMovingRightReadsAsRight(t *testing.T) {
	m := newMouse()
	m.move(20, 0)

	if got := readDirection(m, 40, true); got <= 0 {
		t.Errorf("moving right read as %v, wanted a positive count", got)
	}
}

func TestMovingLeftReadsAsLeft(t *testing.T) {
	m := newMouse()
	m.move(-20, 0)

	if got := readDirection(m, 40, true); got >= 0 {
		t.Errorf("moving left read as %v, wanted a negative count", got)
	}
}

// The screen counts downwards, so a positive delta is down, and down is the
// same side of the table as left
func TestMovingDownReadsAsDown(t *testing.T) {
	m := newMouse()
	m.move(0, 20)

	if got := readDirection(m, 40, false); got >= 0 {
		t.Errorf("moving down read as %v, wanted a negative count", got)
	}
}

func TestMovingUpReadsAsUp(t *testing.T) {
	m := newMouse()
	m.move(0, -20)

	if got := readDirection(m, 40, false); got <= 0 {
		t.Errorf("moving up read as %v, wanted a positive count", got)
	}
}

func TestAStillMouseDoesNotMoveTheLines(t *testing.T) {
	m := newMouse()

	x1, x2, y1, y2 := m.tick(true, true)
	for i := 0; i < 20; i++ {
		m.quadratureRead()
		a, b, c, d := m.tick(true, true)
		if a != x1 || b != x2 || c != y1 || d != y2 {
			t.Fatal("the lines moved with no movement asked for")
		}
	}
}

// The movement is paid out a step at a time rather than in one jump, which
// is what keeps the ROM able to follow it
func TestTheMovementIsPaidOutAStepAtATime(t *testing.T) {
	m := newMouse()
	const pixels = 3
	steps := pixels * quadratureStepsPerPixel

	m.move(pixels, 0)

	if m.pendingX != steps {
		t.Fatalf("the movement waiting is %v, wanted %v", m.pendingX, steps)
	}

	for i := steps; i > 0; i-- {
		m.quadratureRead()
		m.tick(true, true)
		if m.pendingX != i-1 {
			t.Errorf("after a tick the movement waiting is %v, wanted %v",
				m.pendingX, i-1)
		}
	}

	m.quadratureRead()
	m.tick(true, true)
	if m.pendingX != 0 {
		t.Error("the movement went past zero")
	}
}

func TestTheTwoAxesAreIndependent(t *testing.T) {
	m := newMouse()
	m.move(5, 0)

	before := m.phaseY
	for i := 0; i < 10; i++ {
		m.quadratureRead()
		m.tick(true, true)
	}
	if m.phaseY != before {
		t.Error("moving along x moved the y axis")
	}
}

// The button is active low on the port B, so a press pulls its bit down
func TestTheButtonPullsItsBitDown(t *testing.T) {
	v, _, _ := newTestVia(t)

	if v.mos.GetPortB()&viaPortBMouseSwitch == 0 {
		t.Error("the button reads pressed with nothing touching it")
	}

	v.mouse.setButton(true)
	v.refreshPortBInputs()
	if v.mos.GetPortB()&viaPortBMouseSwitch != 0 {
		t.Error("a pressed button did not pull its bit down")
	}

	v.mouse.setButton(false)
	v.refreshPortBInputs()
	if v.mos.GetPortB()&viaPortBMouseSwitch == 0 {
		t.Error("the button stayed down after it was released")
	}
}

func TestTheQuadratureReachesThePortB(t *testing.T) {
	v, _, _ := newTestVia(t)

	v.setMouseQuadrature(false, false)
	if v.mos.GetPortB()&(viaPortBMouseX2|viaPortBMouseY2) != 0 {
		t.Error("the quadrature lines did not go low")
	}

	v.setMouseQuadrature(true, true)
	if v.mos.GetPortB()&(viaPortBMouseX2|viaPortBMouseY2) != viaPortBMouseX2|viaPortBMouseY2 {
		t.Error("the quadrature lines did not go high")
	}
}

/*
A pixel of the host has to be one transition of the interrupt line, because
that is the unit the ROM counts. The line only changes on two of the four
phases, so a phase per pixel delivers half the movement and the pointer feels
heavy, which is what a mouse that is merely slow rather than wrong looks
like.
*/
func TestAPixelIsOneTransition(t *testing.T) {
	const pixels = 25

	m := newMouse()
	m.move(pixels, 0)

	previous, _, _, _ := m.tick(true, true)
	transitions := 0
	for i := 0; i < pixels*quadratureStepsPerPixel*2; i++ {
		m.quadratureRead()
		x1, _, _, _ := m.tick(true, true)
		if x1 != previous {
			transitions++
			previous = x1
		}
	}

	if transitions != pixels {
		t.Errorf("%v pixels gave %v transitions, wanted one each", pixels, transitions)
	}
}

/*
An axis held because its interrupt has not been answered must not lose the
movement, only postpone it. Dropping it instead is the difference between a
pointer that lags behind the mouse and one that stops short of where it was
pushed.
*/
func TestMovementIsPostponedAndNotLostWhileHeld(t *testing.T) {
	m := newMouse()
	m.move(10, 0)
	waiting := m.pendingX

	// Held for a long while, nothing moves and nothing is lost
	for i := 0; i < 50; i++ {
		m.tick(false, false)
	}
	if m.pendingX != waiting {
		t.Errorf("the movement waiting went from %v to %v while held",
			waiting, m.pendingX)
	}

	// Let go and it all comes out
	for i := 0; i < waiting*2; i++ {
		m.quadratureRead()
		m.tick(true, true)
	}
	if m.pendingX != 0 {
		t.Errorf("%v steps were still waiting after they were let out", m.pendingX)
	}
}

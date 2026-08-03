package izmac

import "testing"

/*
The absolute pointer, end to end on the booted machine.

What it checks is Mouse, the location at $0830 that every application reads,
and not only the RawMouse that pointer.go writes. Nothing but the cursor task
of the ROM writes Mouse, so a position turning up there is the whole chain
working: the low memory was written where the task looks, CrsrNew was set the
way the task wants it, the task ran on the vertical blanking and took the
position as a place to be rather than as a movement to be scaled.

That last part is what a test of RawMouse alone would not catch. Writing MTemp
without RawMouse also moves the pointer, in the right direction and to the
wrong place, since the task reads the difference between the two as the
movement of the frame and is free to double it.
*/
func TestThePointerGoesWhereItIsPut(t *testing.T) {
	const (
		rawMouseV = 0x082c
		rawMouseH = 0x082e

		// Mouse is where the cursor task leaves the position for everything
		// above it to read
		mouseV = 0x0830
		mouseH = 0x0832

		crsrNew = 0x08ce
	)

	m := bootedMac(t)

	readPoint := func(address uint32) (int16, int16) {
		v := int16(uint16(m.mm.Peek(address))<<8 | uint16(m.mm.Peek(address+1)))
		h := int16(uint16(m.mm.Peek(address+2))<<8 | uint16(m.mm.Peek(address+3)))
		return v, h
	}

	// Three corners of the screen and a place in the middle, far enough
	// apart that nothing but a placed pointer could get between them in the
	// three frames each is given
	for _, c := range []struct {
		name string
		x, y int
	}{
		{"the middle", 256, 171},
		{"the top left", 0, 0},
		{"the bottom right", width - 1, height - 1},
		{"the menu bar", 40, 8},
	} {
		m.SetMousePosition(c.x, c.y)
		m.RunFrames(3)

		wantH, wantV := int16(c.x), int16(c.y)

		if v, h := readPoint(rawMouseV); h != wantH || v != wantV {
			t.Errorf("the pointer put on %v, at %v,%v, left RawMouse at %v,%v",
				c.name, c.x, c.y, h, v)
		}

		if v, h := readPoint(mouseV); h != wantH || v != wantV {
			t.Errorf("the pointer put on %v, at %v,%v, left Mouse at %v,%v, "+
				"so the cursor task did not take it as a position",
				c.name, c.x, c.y, h, v)
		}
	}

	// And the task has taken the flag down again, which is what says it ran
	// rather than that nothing has looked at it yet
	if flag := m.mm.Peek(crsrNew); flag != 0 {
		t.Errorf("CrsrNew is $%02x three frames after the pointer was placed, "+
			"the cursor task has not run", flag)
	}
}

/*
The pointer placed from the first frame, which is what a frontend does when
the pointer of the host is already over the window as the machine starts, and
what every other test here misses by placing it on a machine that has already
booted.

It has to boot anyway. The ROM masks every interrupt, tests the whole of the
RAM by writing patterns over it and reading them back, and drops the mask only
once the test has passed and the low memory has been laid out. A position
written into the middle of that test is read back as a pattern that was never
written, and the machine puts up a Sad Mac 03FFFF and stops, which is exactly
what it did.
*/
func TestPlacingThePointerDoesNotDisturbTheBoot(t *testing.T) {
	const rawMouseV = 0x082c

	config := realConfig(t)

	m, err := NewMac(config)
	if err != nil {
		t.Fatal(err)
	}

	// Before the first instruction, as a frontend that has the host pointer
	// over the window does
	m.SetMousePosition(100, 100)
	m.RunFrames(bootFrames)

	if black := blackRatio(m, 2, 16); black > 0.2 {
		t.Fatalf("the top of the screen is %.0f%% black after %v frames with "+
			"the pointer placed from the start, so the machine never reached "+
			"the Finder", black*100, bootFrames)
	}

	// And the position was not simply thrown away with the boot: it is
	// placed as soon as the machine is able to take it
	v := int16(uint16(m.mm.Peek(rawMouseV))<<8 | uint16(m.mm.Peek(rawMouseV+1)))
	h := int16(uint16(m.mm.Peek(rawMouseV+2))<<8 | uint16(m.mm.Peek(rawMouseV+3)))

	if h != 100 || v != 100 {
		t.Errorf("the pointer is at %v,%v after the boot, the host has had it "+
			"at 100,100 throughout", h, v)
	}
}

/*
The cursor drawn on the screen, which is the part of this that anyone using
the machine actually sees. The low memory could hold the right position while
the arrow sat somewhere else: the cursor task erases the cursor and draws it
again at the end of the same run that moves the position, and only the screen
says that it did.

So the pointer is put on two places of the desktop in turn, and the pixels
around each are counted before and after. The arrow arriving adds black to the
box it lands in, and leaving takes it away again, back to what was under it.
*/
func TestTheCursorIsDrawnWhereThePointerIsPut(t *testing.T) {
	// The box is the size of the arrow, which is drawn to the right and
	// below the position it points at
	const box = 16

	m := bootedMac(t)

	spots := []struct {
		name string
		x, y int
	}{
		{"the window", 120, 120},
		{"the desktop", 300, 220},
	}

	empty := make([]int, len(spots))
	for i, spot := range spots {
		empty[i] = blackInBox(m, spot.x, spot.y, box)
	}

	for i, spot := range spots {
		m.SetMousePosition(spot.x, spot.y)
		m.RunFrames(5)

		if black := blackInBox(m, spot.x, spot.y, box); black <= empty[i] {
			t.Errorf("the pointer put on %v, at %v,%v, left %v black pixels "+
				"there where there were %v before, so no cursor was drawn",
				spot.name, spot.x, spot.y, black, empty[i])
		}

		// And wherever it was before is as it was before the cursor was
		// ever there, which is the erasing half of the same run
		for j, other := range spots {
			if j == i {
				continue
			}
			if black := blackInBox(m, other.x, other.y, box); black != empty[j] {
				t.Errorf("%v, at %v,%v, has %v black pixels with the pointer "+
					"on %v, and had %v before it ever went there",
					other.name, other.x, other.y, black, spot.name, empty[j])
			}
		}
	}
}

// blackInBox counts the black pixels of a square of the screen
func blackInBox(m *Mac, x int, y int, size int) int {
	buffer := m.video.frameBuffer()

	black := 0
	for row := y; row < y+size; row++ {
		for column := x; column < x+size; column++ {
			if buffer[row*bytesPerLine+column/8]&(0x80>>(column%8)) != 0 {
				black++
			}
		}
	}
	return black
}

/*
A pointer that is not moved stays where it was put, frame after frame. The
machine has a mouse of its own that reports nothing, so there is nothing to
pull the cursor off it, and the position is written again only when the two
disagree.
*/
func TestAPlacedPointerStaysWhereItIs(t *testing.T) {
	const rawMouseV = 0x082c

	m := bootedMac(t)

	m.SetMousePosition(300, 200)
	m.RunFrames(3)
	m.RunFrames(60)

	v := int16(uint16(m.mm.Peek(rawMouseV))<<8 | uint16(m.mm.Peek(rawMouseV+1)))
	h := int16(uint16(m.mm.Peek(rawMouseV+2))<<8 | uint16(m.mm.Peek(rawMouseV+3)))

	if h != 300 || v != 200 {
		t.Errorf("the pointer drifted to %v,%v over a second of standing still", h, v)
	}
}

/*
The machine switched to being pushed rather than placed, which is what the
menu line and the relative option leave it as. The positions a frontend goes
on reporting are then ignored, and the movement it reports is what moves the
pointer.
*/
func TestAMachineSwitchedToAPushedMouseIgnoresPositions(t *testing.T) {
	const rawMouseV = 0x082c

	m := bootedMac(t)

	readPoint := func() (int16, int16) {
		v := int16(uint16(m.mm.Peek(rawMouseV))<<8 | uint16(m.mm.Peek(rawMouseV+1)))
		h := int16(uint16(m.mm.Peek(rawMouseV+2))<<8 | uint16(m.mm.Peek(rawMouseV+3)))
		return v, h
	}

	m.SetMousePosition(100, 100)
	m.RunFrames(3)

	m.toggleAbsoluteMouse()
	if m.IsAbsoluteMouse() {
		t.Fatal("the machine is still placing the pointer after the mouse was switched over")
	}

	m.SetMousePosition(400, 300)
	m.RunFrames(10)

	if v, h := readPoint(); h != 100 || v != 100 {
		t.Errorf("a pushed mouse took the pointer to %v,%v, it should have "+
			"stayed at the 100,100 it was placed at", h, v)
	}

	// And pushing it still works, which is the whole point of keeping it
	m.MoveMouse(40, 30)
	m.RunFrames(30)

	if v, h := readPoint(); h <= 100 || v <= 100 {
		t.Errorf("pushing the mouse right and down left the pointer at %v,%v", h, v)
	}
}

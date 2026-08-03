package izmac

import "testing"

// newTestPointer returns a pointer placing positions on a machine that is off
// the overlay map, which is where the low memory globals are RAM
func newTestMousePointer(t *testing.T, absolute bool) (*mousePointer, *memoryManager) {
	t.Helper()

	mm := newTestMemoryManager(1024)
	mm.setOverlay(false)

	// The cursor is coupled to the mouse, which is what the machine leaves it
	// as except while an application is drawing over it
	mm.Poke(crsrCoupleAddress, 0xff)

	return newMousePointer(absolute), mm
}

// pointAt reads one of the packed points of the low memory
func pointAt(mm *memoryManager, address uint32) (v int16, h int16) {
	point := mm.peekLong(address)
	return int16(point >> 16), int16(point)
}

// bothPoints reads the two points the cursor task works from, which a
// position has to be written to both of
func bothPoints(t *testing.T, mm *memoryManager) (v int16, h int16) {
	t.Helper()

	tempV, tempH := pointAt(mm, mTempAddress)
	rawV, rawH := pointAt(mm, rawMouseAddress)

	if tempV != rawV || tempH != rawH {
		t.Fatalf("MTemp is at %v,%v and RawMouse at %v,%v, the difference "+
			"between the two is movement and the cursor task will scale it",
			tempH, tempV, rawH, rawV)
	}
	return rawV, rawH
}

func TestThePointerIsPlacedWhereItIsPut(t *testing.T) {
	p, mm := newTestMousePointer(t, true)

	p.put(100, 50)
	p.place(mm, 0)

	if v, h := bothPoints(t, mm); h != 100 || v != 50 {
		t.Errorf("the pointer put at 100,50 was placed at %v,%v", h, v)
	}

	// And the cursor task is told that the mouse has moved, which it does
	// nothing at all without
	if crsrNew := mm.Peek(crsrNewAddress); crsrNew != 0xff {
		t.Errorf("CrsrNew is $%02x, it has to carry CrsrCouple, $ff", crsrNew)
	}
}

/*
The cursor task only moves the cursor while CrsrCouple says the two are
joined, and it is CrsrCouple that the interrupt handlers copy into CrsrNew
rather than a one of their own. Placing a pointer does the same thing, so an
application that has taken the cursor over keeps it.
*/
func TestAnUncoupledCursorIsNotToldTheMouseMoved(t *testing.T) {
	p, mm := newTestMousePointer(t, true)
	mm.Poke(crsrCoupleAddress, 0)

	p.put(100, 50)
	p.place(mm, 0)

	if crsrNew := mm.Peek(crsrNewAddress); crsrNew != 0 {
		t.Errorf("CrsrNew is $%02x with the cursor uncoupled, it has to be zero", crsrNew)
	}
}

// A position off the screen is brought back to its edge, which is where a
// drag that has left the window should be pulling from
func TestThePointerIsBroughtBackToTheScreen(t *testing.T) {
	p, mm := newTestMousePointer(t, true)

	for _, c := range []struct {
		name  string
		x, y  int
		wantH int16
		wantV int16
	}{
		{"above and to the left", -10, -1, 0, 0},
		{"below and to the right", width + 100, height, width - 1, height - 1},
	} {
		p.put(c.x, c.y)
		p.place(mm, 0)

		if v, h := bothPoints(t, mm); h != c.wantH || v != c.wantV {
			t.Errorf("a pointer %v, at %v,%v, was placed at %v,%v and not at %v,%v",
				c.name, c.x, c.y, h, v, c.wantH, c.wantV)
		}
	}
}

/*
A position already reached is not written again. The machine is what it is
compared against, so this is also what keeps the cursor from being erased and
drawn again sixty times a second while nothing is moving.
*/
func TestAPointerAlreadyThereIsLeftAlone(t *testing.T) {
	p, mm := newTestMousePointer(t, true)

	p.put(100, 50)
	p.place(mm, 0)

	writes := 0
	mm.setWatch(mTempAddress, crsrCoupleAddress, func(address uint32, value uint8) {
		writes++
	})

	p.place(mm, 0)
	if writes != 0 {
		t.Errorf("placing the pointer where it already is wrote %v bytes of low memory", writes)
	}

	// And the same position put again is still the same position
	p.put(100, 50)
	p.place(mm, 0)
	if writes != 0 {
		t.Errorf("putting the pointer where it already is wrote %v bytes of low memory", writes)
	}
}

/*
The machine is compared against rather than the last position given, so a
cursor moved by anything else comes back on the next frame. An application
moving it itself, a reset and the boot are all this case.
*/
func TestAPointerMovedByTheMachineIsPutBack(t *testing.T) {
	p, mm := newTestMousePointer(t, true)

	p.put(100, 50)
	p.place(mm, 0)

	// The machine takes the cursor somewhere of its own
	mm.pokeLong(rawMouseAddress, packPoint(200, 300))
	mm.pokeLong(mTempAddress, packPoint(200, 300))

	p.place(mm, 0)
	if v, h := bothPoints(t, mm); h != 100 || v != 50 {
		t.Errorf("the pointer stayed at %v,%v where the machine put it, "+
			"instead of going back to 100,50 where the host has it", h, v)
	}
}

// A machine whose mouse is pushed rather than placed leaves the low memory
// alone, whatever a frontend tells it about the pointer of the host
func TestAPushedMouseIgnoresTheHostPointer(t *testing.T) {
	p, mm := newTestMousePointer(t, false)

	p.put(100, 50)
	p.place(mm, 0)

	if v, h := bothPoints(t, mm); h != 0 || v != 0 {
		t.Errorf("a pushed mouse put the pointer at %v,%v", h, v)
	}
}

// Until the host has pointed at the screen there is no position to place, and
// the machine's own pointer is left where it is
func TestAPointerNeverPutIsNotPlaced(t *testing.T) {
	p, mm := newTestMousePointer(t, true)

	mm.pokeLong(rawMouseAddress, packPoint(200, 300))
	p.place(mm, 0)

	if v, h := pointAt(mm, rawMouseAddress); h != 300 || v != 200 {
		t.Errorf("the pointer of the machine was moved to %v,%v before the "+
			"host had pointed anywhere", h, v)
	}
}

/*
Nothing is written while the processor is masking the interrupt the cursor
task runs on. The task would not run, so there would be nothing to gain, and
the boot does everything that must not be disturbed behind that mask: the
memory test writes patterns over the whole of the RAM and reads them back, and
a position written into the middle of it is a Sad Mac.
*/
func TestNothingIsPlacedWhileTheInterruptIsMasked(t *testing.T) {
	p, mm := newTestMousePointer(t, true)
	p.put(100, 50)

	for mask := uint8(viaInterruptLevel); mask <= 7; mask++ {
		writes := 0
		mm.setWatch(0, mm.ramTop()-1, func(address uint32, value uint8) {
			writes++
		})

		p.place(mm, mask)

		if writes != 0 {
			t.Errorf("placing the pointer with the processor masking level %v "+
				"wrote %v bytes of low memory", mask, writes)
		}
	}

	// And it is placed as soon as the machine would take the interrupt
	mm.setWatch(0, 0, nil)
	p.place(mm, 0)

	if v, h := bothPoints(t, mm); h != 100 || v != 50 {
		t.Errorf("an unmasked machine left the pointer at %v,%v", h, v)
	}
}

/*
While the overlay is on there is ROM where the low memory globals belong. The
write would go nowhere, since the memory manager drops what is written to the
ROM, but the read that decides whether it is needed would be of ROM bytes and
would ask for a write on every frame of the boot.
*/
func TestNothingIsPlacedWhileTheOverlayIsOn(t *testing.T) {
	p, mm := newTestMousePointer(t, true)
	mm.setOverlay(true)

	writes := 0
	mm.setWatch(0, mm.ramTop()-1, func(address uint32, value uint8) {
		writes++
	})

	p.put(100, 50)
	p.place(mm, 0)

	if writes != 0 {
		t.Errorf("placing the pointer on the overlay map wrote %v bytes", writes)
	}
}

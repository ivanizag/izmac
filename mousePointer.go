package izmac

import "sync/atomic"

/*
The pointer of the machine put where the pointer of the host is, which the
hardware has no way of being told.

The Macintosh mouse only ever reports movement and the ROM keeps the pointer
itself, so there is no register to write a position to. What there is, is the
low memory the cursor task works from. CrsrVBLTask runs on every vertical
blanking, and this is what it does, from `plus/hw/interrupts.s` of the
disassembly:

	MTemp - RawMouse is the movement of the frame
	which is doubled if CrsrScale is set and the distance passes CrsrThresh
	and added to RawMouse, which is then pinned to CrsrPin
	MTemp is set to the pinned RawMouse, so that the two agree again
	Mouse, the location every application reads, follows it
	and the cursor is erased and drawn again

None of which happens unless CrsrNew is set, which the quadrature interrupt
handlers do with MOVE.B CrsrCouple,CrsrNew every time they count a transition.

So a position is given to the machine by writing it to both MTemp and
RawMouse and then setting CrsrNew the way those handlers do. Both of them,
because it is the difference between the two that the task reads as movement:
writing MTemp alone would be a movement, and one the acceleration is free to
double. With the two equal there is no movement to scale, and the pinning,
the Mouse global and the drawing all happen as they do for a mouse that was
pushed there.

The button is no part of this. It is a line on the VIA that the ROM samples
for itself, so it works the same whether the pointer was pushed or placed.

Nothing is written unless the machine would take that interrupt, and that is
not only an optimization. The boot masks every interrupt at level 7 before it
does anything, tests the whole of the RAM by writing patterns over it and
reading them back, and only drops the mask once the test has passed, the low
memory has been laid out and the cursor task has been installed. A position
written into the middle of the memory test is read back as a pattern that was
never written, and the machine puts up the Sad Mac and stops. So the mask says
both things at once: whether the cursor task will run, and whether the memory
being written to is the low memory or a test pattern that has to be left
alone.

Nothing here asks what System is running, as nothing in clipboard.go does. The
cursor task is in the ROM and these addresses are the ones every System that
runs on the 128Kb ROM shares.
*/

const (
	/*
		The low memory the cursor task works from, from `include/lowmem.inc`
		of the disassembly. MTemp is where the interrupt handlers leave the
		mouse and RawMouse is where the cursor is; both are held as the
		machine holds a Point, the vertical word first.
	*/
	mTempAddress    = 0x0828
	rawMouseAddress = 0x082c

	// crsrNewAddress says the mouse has moved, and crsrCoupleAddress whether
	// the cursor is following the mouse at all
	crsrNewAddress    = 0x08ce
	crsrCoupleAddress = 0x08cf
)

/*
pointer is where the host says its own pointer is, waiting to be given to the
machine.
*/
type mousePointer struct {
	/*
		absolute is whether the pointer of the machine is put where the
		host's is rather than pushed by the movement of it, and wanted is
		where that is: a Point packed as the machine packs one, the vertical
		in the high word and the horizontal in the low one, or nowhere until
		the host has pointed at the screen at all.

		Both are written by the goroutine of a frontend and read by the one
		of the emulation, so both are kept where the two can reach them
		safely.
	*/
	absolute atomic.Bool
	wanted   atomic.Int64
}

// mousePointerNowhere is the wanted position while the host has not pointed at the
// screen yet, which is not the same as pointing at its top left corner
const mousePointerNowhere = -1

func newMousePointer(absolute bool) *mousePointer {
	p := &mousePointer{}
	p.absolute.Store(absolute)
	p.wanted.Store(mousePointerNowhere)
	return p
}

// isAbsolute tells whether the pointer of the machine is put where the host's
// is rather than pushed by the movement of it
func (p *mousePointer) isAbsolute() bool {
	return p.absolute.Load()
}

func (p *mousePointer) setAbsolute(absolute bool) {
	p.absolute.Store(absolute)
}

/*
put records where the host has its own pointer, in the pixels of the screen.

A position off the screen is brought back to its edge rather than refused. The
machine would pin it there itself, and a drag that has left the window is the
case that wants it: the pointer stays against the side it left by instead of
stopping wherever it was when it crossed.
*/
func (p *mousePointer) put(x int, y int) {
	p.wanted.Store(int64(packPoint(
		clampToScreen(y, height), clampToScreen(x, width))))
}

/*
place puts the pointer of the machine where the host has its own. It is called
as the vertical blanking starts, so that the cursor task, which runs on the
interrupt raised just after it, works from this frame's position and not from
the one before. The mask is the level the processor is refusing interrupts
below, which decides whether that interrupt will be taken at all.

What it compares against is the machine rather than the last position given.
The position stands until the host moves its pointer somewhere else, so the
machine coming up, an application moving the cursor itself and a reset all
right themselves on the next frame, instead of leaving the two pointers apart
until the host is moved.
*/
func (p *mousePointer) place(mm *memoryManager, interruptMask uint8) {
	if !p.absolute.Load() {
		return
	}

	wanted := p.wanted.Load()
	if wanted == mousePointerNowhere {
		return
	}

	/*
		A machine that would not take the vertical blanking is a machine
		whose cursor task is not going to run, so there is nothing to be
		gained by writing, and during the boot there is a great deal to lose:
		the memory test runs behind this mask and a write into it is read
		back as a pattern that was never written. Everything the ROM has to
		do before the low memory means anything is done at level 7, and the
		one place it drops the mask is when all of it is finished.
	*/
	if interruptMask >= viaInterruptLevel {
		return
	}

	// While the overlay is on there is ROM where the low memory globals
	// belong, so there is nowhere to put it yet
	if mm.overlay {
		return
	}

	if mm.peekLong(rawMouseAddress) == uint32(wanted) {
		return
	}

	mm.pokeLong(mTempAddress, uint32(wanted))
	mm.pokeLong(rawMouseAddress, uint32(wanted))
	mm.Poke(crsrNewAddress, mm.Peek(crsrCoupleAddress))
}

// interruptMaskOf returns the level the processor is refusing interrupts
// below, which the bits 8 to 10 of the status register hold
func interruptMaskOf(sr uint16) uint8 {
	return uint8(sr>>8) & 7
}

// clampToScreen brings a coordinate of the host back onto the screen of the
// machine, which is size pixels wide or tall
func clampToScreen(value int, size int) int {
	if value < 0 {
		return 0
	}
	if value >= size {
		return size - 1
	}
	return value
}

// packPoint packs a Point into the long the machine holds one in, the
// vertical word first
func packPoint(v int, h int) uint32 {
	return uint32(uint16(v))<<16 | uint32(uint16(h))
}

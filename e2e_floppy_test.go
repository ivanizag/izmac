package izmac

import (
	"os"
	"path/filepath"
	"testing"
)

/*
The diskette drive end to end, with the real ROM driving it: the machine is
booted to the Finder, a blank image is put in the drive, and the Macintosh is
asked to initialize it.

That is the whole feature in one test. Initializing writes every track of both
sides and then reads the volume back to mount it, so a disk that ends up on
the desktop has been through the encoding, the drive, the controller and the
decoding, in both directions, driven by Apple's own Sony driver rather than by
anything here that might agree with izmac about the wrong thing.

What it asserts is the image on the host. The block 2 of a Macintosh volume is
the master directory block and starts with the letters 'BD', so finding it
there means the machine wrote a file system izmac stored where it belongs.

It needs the ROM and the disk image the other end to end tests need, and skips
without them.
*/
func TestTheMachineInitializesADiskette(t *testing.T) {
	m := bootedMac(t)

	blank := filepath.Join(t.TempDir(), "blank.dsk")
	if err := os.WriteFile(blank, make([]uint8, 800*1024), 0666); err != nil {
		t.Fatal(err)
	}

	if err := m.InsertDiskette(DriveInternal, blank); err != nil {
		t.Fatal(err)
	}

	// The Finder notices the disk and puts up the dialog that offers to
	// initialize it, since a diskette of zeros is not a Macintosh one
	m.RunFrames(400)

	/*
		The two buttons, in the middle of the screen where the ROM centres
		an alert: first the two sided format, then the confirmation, whose
		Erase button lands under the pointer where it already is.
	*/
	moveMouseTo(t, m, 348, 158)
	m.RunFrames(20)
	clickMouse(m)
	m.RunFrames(120)

	clickMouse(m)
	m.RunFrames(600)

	// And the name it offers is taken as it stands
	pressKey(m, "Return")

	/*
		Writing eighty tracks of both sides takes the machine a while, so
		the image is looked at now and then rather than after a fixed wait.
		Flushing is what puts it on the host: the emulation does it by
		itself when the motor stops, which is after the Finder has finished.
	*/
	const (
		formatPolls   = 20
		framesPerPoll = 600
	)

	for poll := 0; poll < formatPolls; poll++ {
		m.RunFrames(framesPerPoll)

		if err := m.FlushDiskettes(); err != nil {
			t.Fatal(err)
		}

		image, err := os.ReadFile(blank)
		if err != nil {
			t.Fatal(err)
		}

		if masterDirectoryBlockSignature(image) == hfsSignature {
			return
		}
	}

	image, err := os.ReadFile(blank)
	if err != nil {
		t.Fatal(err)
	}

	t.Fatalf("the diskette was never initialized: the block 2 of the image "+
		"starts with $%04x and not the $%04x of a Macintosh volume",
		masterDirectoryBlockSignature(image), hfsSignature)
}

// hfsSignature is the 'BD' a Macintosh volume starts its master directory
// block with
const hfsSignature = 0x4244

// masterDirectoryBlockSignature reads the first word of the block 2 of an
// image, which is where a Macintosh volume keeps its signature
func masterDirectoryBlockSignature(image []uint8) uint16 {
	const masterDirectoryBlock = 2 * 512

	if len(image) < masterDirectoryBlock+2 {
		return 0
	}
	return uint16(image[masterDirectoryBlock])<<8 | uint16(image[masterDirectoryBlock+1])
}

/*
pointerAt is where the ROM has put the pointer, across and down. It keeps it
in RawMouse, the two words at $082c, which is the only way to find out from
here: the pointer is drawn by the machine and the mouse is only pushed.
*/
func pointerAt(m *Mac) (int16, int16) {
	const rawMouseV, rawMouseH = 0x082c, 0x082e

	v := int16(uint16(m.mm.Peek(rawMouseV))<<8 | uint16(m.mm.Peek(rawMouseV+1)))
	h := int16(uint16(m.mm.Peek(rawMouseH))<<8 | uint16(m.mm.Peek(rawMouseH+1)))

	return h, v
}

// moveMouseTo pushes the pointer to a place on the screen a bit at a time,
// since the ROM scales what the mouse reports and one push does not arrive
func moveMouseTo(t *testing.T, m *Mac, wantH int16, wantV int16) {
	t.Helper()

	for try := 0; try < 60; try++ {
		h, v := pointerAt(m)
		if h == wantH && v == wantV {
			return
		}
		m.MoveMouse(int(wantH-h), int(wantV-v))
		m.RunFrames(3)
	}

	h, v := pointerAt(m)
	t.Fatalf("the pointer stopped at %v,%v on the way to %v,%v", h, v, wantH, wantV)
}

// clickMouse presses and releases the only button the machine has
func clickMouse(m *Mac) {
	m.SetMouseButton(true)
	m.RunFrames(8)
	m.SetMouseButton(false)
	m.RunFrames(20)
}

// pressKey taps a key by the name the key code table knows it by
func pressKey(m *Mac, name string) {
	code := KeyCodes()[name]

	m.PutKey(code, true)
	m.RunFrames(10)
	m.PutKey(code, false)
	m.RunFrames(10)
}

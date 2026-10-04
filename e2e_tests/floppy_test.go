package e2e_tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ivanizag/izmac"
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
	t.Parallel()
	m := bootedMac(t)

	blank := filepath.Join(t.TempDir(), "blank.dsk")
	if err := os.WriteFile(blank, make([]uint8, 800*1024), 0666); err != nil {
		t.Fatal(err)
	}

	if err := m.InsertDiskette(izmac.DriveInternal, blank); err != nil {
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

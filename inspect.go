package izmac

import (
	"image"

	"golang.org/x/text/encoding/charmap"
)

/*
What a program driving the machine can read of it, without changing it: the
memory, a picture of the screen, and a few things the System keeps in low
memory that someone at the machine would see, such as where the pointer is and
which application is running. They are read between frames, as RunFrames
leaves the machine, and are what izmac's own tests and the activities wait
on and check.
*/

// The low memory globals read here, as Inside Macintosh names them
const (
	// MTemp and RawMouse, the pointer as the mouse put it, before the
	// cursor follows it
	lowRawMouseV = 0x082c
	lowRawMouseH = 0x082e

	// CurApName, the name of the application running, a Pascal string
	lowCurApName = 0x0910

	// CurApRefNum, the reference number of the application's resource
	// file, and FCBSPtr, the table of file control blocks it indexes
	lowCurApRefNum = 0x0900
	lowFCBSPtr     = 0x034e

	// VCBQHdr, the queue of the volumes mounted; its head is at 2
	lowVCBQHead = 0x0358

	// Inside a file control block, its volume's control block, and inside
	// one of those, the volume's name
	fcbVPtr = 20
	vcbName = 44

	// The longest names: of a volume, and of an application
	volumeNameLength      = 27
	applicationNameLength = 31
)

/*
Peek reads a byte of the RAM or the ROM as the processor would at that
address. The rest of the address space, the chips, reads as zero: reading a
chip can change it, a SCSI read moving the transfer on or a VIA read clearing
an interrupt, and a look from outside must not.
*/
func (m *Mac) Peek(address uint32) uint8 {
	address &= addressMask
	switch address >> 22 {
	case quarterRAM, quarterROM:
		if address>>22 == quarterROM && address >= romWindowEnd {
			return 0
		}
		return m.mm.Peek(address)
	}
	return 0
}

// PeekWord reads the two bytes at an address, big endian as the 68000 does
func (m *Mac) PeekWord(address uint32) uint16 {
	return uint16(m.Peek(address))<<8 | uint16(m.Peek(address+1))
}

// PeekLong reads the four bytes at an address, big endian
func (m *Mac) PeekLong(address uint32) uint32 {
	return uint32(m.PeekWord(address))<<16 | uint32(m.PeekWord(address+2))
}

// peekString reads a Pascal string, a length and the bytes, as the host
// writes text
func (m *Mac) peekString(address uint32, longest int) string {
	text := make([]uint8, min(int(m.Peek(address)), longest))
	for i := range text {
		text[i] = m.Peek(address + 1 + uint32(i))
	}
	decoded, err := charmap.Macintosh.NewDecoder().Bytes(text)
	if err != nil {
		return string(text)
	}
	return string(decoded)
}

/*
PointerPosition is where the pointer of the machine is, in the pixels of its
screen, as the mouse has put it. The ROM moves the cursor there at the next
vertical blanking, so this is where the cursor is or is about to be.
*/
func (m *Mac) PointerPosition() (h int, v int) {
	return int(int16(m.PeekWord(lowRawMouseH))), int(int16(m.PeekWord(lowRawMouseV)))
}

/*
CurrentApplication is the name of the application running, which the Segment
Loader keeps: "Finder" in the Finder, the name of the file of any other.
Before the System has started anything it is empty or noise.
*/
func (m *Mac) CurrentApplication() string {
	return m.peekString(lowCurApName, applicationNameLength)
}

/*
MountedVolumes are the names of the volumes the machine has mounted, the one
it started from first, as the File Manager keeps them: a queue of volume
control blocks, each linking to the next, with its name at 44
*/
func (m *Mac) MountedVolumes() []string {
	const most = 16
	var names []string
	for vcb := m.PeekLong(lowVCBQHead); vcb != 0 && len(names) < most; vcb = m.PeekLong(vcb) {
		names = append(names, m.peekString(vcb+vcbName, volumeNameLength))
	}
	return names
}

/*
ApplicationVolume is the name of the volume the application running was
started from: the reference number of its resource file is the offset of its
file control block in the table, whose field at 20 is the control block of its
volume. It is empty when nothing has been started.
*/
func (m *Mac) ApplicationVolume() string {
	ref := m.PeekWord(lowCurApRefNum)
	if ref == 0 {
		return ""
	}
	vcb := m.PeekLong(m.PeekLong(lowFCBSPtr) + uint32(ref) + fcbVPtr)
	return m.peekString(vcb+vcbName, volumeNameLength)
}

/*
Screenshot is a picture of the screen as it is now, of its own. GetImage is
the image the frontends draw from, which the next frame draws over: a picture
to keep is this one.
*/
func (m *Mac) Screenshot() *image.RGBA {
	screen := m.GetImage()
	kept := image.NewRGBA(screen.Bounds())
	copy(kept.Pix, screen.Pix)
	return kept
}

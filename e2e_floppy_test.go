package izmac

import (
	"os"
	"path/filepath"
	"strings"
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
	t.Parallel()
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

// pointerAt is where the pointer is on the screen, from the low memory the ROM
// keeps it in
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

	at := func() (int16, int16) { return pointerAt(m) }

	// The System of 1985 moves the pointer more slowly than later ones, so
	// there are tries enough for it too; the pointer gets there long before
	for try := 0; try < 400; try++ {
		h, v := at()
		if h == wantH && v == wantV {
			return
		}
		m.MoveMouse(int(wantH-h), int(wantV-v))
		m.RunFrames(3)
	}

	h, v := at()
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

// pressCommand taps a key with the command key held, a menu accelerator
func pressCommand(m *Mac, name string) {
	codes := KeyCodes()
	m.PutKey(codes["Command"], true)
	m.RunFrames(6)
	m.PutKey(codes[name], true)
	m.RunFrames(6)
	m.PutKey(codes[name], false)
	m.RunFrames(6)
	m.PutKey(codes["Command"], false)
	m.RunFrames(6)
}

// typeText types lower case letters and spaces, as a hand would
func typeText(m *Mac, text string) {
	for _, r := range text {
		name := "Space"
		if r != ' ' {
			name = string(r - 'a' + 'A')
		}
		pressKey(m, name)
	}
}

// currentApplication is the name of the application running, which the
// Segment Loader keeps in CurApName, a Pascal string at $0910
func currentApplication(m *Mac) string {
	const curApName = 0x0910
	name := make([]uint8, min(int(m.mm.Peek(curApName)), 31))
	for i := range name {
		name[i] = m.mm.Peek(uint32(curApName + 1 + i))
	}
	return string(name)
}

/*
waitUntil runs the machine a second at a time until something has happened,
for as many seconds as given, and tells whether it did. What another machine
or a server does on the other end of the network takes the time of the host,
and the emulated machine runs many times faster than that, more so on a host
busy with other tests: a wait for something done over the network is for its
outcome, not for a number of frames.
*/
func waitUntil(m *Mac, seconds int, done func() bool) bool {
	for i := 0; i < seconds; i++ {
		if done() {
			return true
		}
		m.RunFrames(60)
	}
	return done()
}

// waitForApplication runs the machine until an application is running, for
// as many seconds as given
func waitForApplication(t *testing.T, m *Mac, name string, seconds int) {
	t.Helper()
	if !waitUntil(m, seconds, func() bool { return currentApplication(m) == name }) {
		t.Fatalf("%v was not running after %v seconds, %q is", name, seconds, currentApplication(m))
	}
}

/*
mountedVolumes are the names of the volumes the machine has mounted, the one
it started from first: the queue of volume control blocks, VCBQHdr at $0356,
whose head is the first, each linking to the next, with its name at 44
*/
func mountedVolumes(m *Mac) []string {
	const vcbQueueHead, vcbName = 0x0358, 44
	readLong := func(address uint32) uint32 {
		return uint32(m.mm.Peek(address))<<24 | uint32(m.mm.Peek(address+1))<<16 |
			uint32(m.mm.Peek(address+2))<<8 | uint32(m.mm.Peek(address+3))
	}
	var names []string
	for vcb := readLong(vcbQueueHead); vcb != 0 && len(names) < 16; vcb = readLong(vcb) {
		name := make([]uint8, min(int(m.mm.Peek(vcb+vcbName)), 27))
		for i := range name {
			name[i] = m.mm.Peek(vcb + vcbName + 1 + uint32(i))
		}
		names = append(names, string(name))
	}
	return names
}

// startupVolume is the name of the volume the machine started from
func startupVolume(m *Mac) string {
	if volumes := mountedVolumes(m); len(volumes) != 0 {
		return volumes[0]
	}
	return ""
}

/*
typeKeys types keys by their names, each with the modifiers before it held
down, as "Shift+1" or "Option+E", one after the other as a hand would
*/
func typeKeys(m *Mac, keys ...string) {
	codes := KeyCodes()
	for _, key := range keys {
		parts := strings.Split(key, "+")
		modifiers, name := parts[:len(parts)-1], parts[len(parts)-1]
		for _, modifier := range modifiers {
			m.PutKey(codes[modifier], true)
			m.RunFrames(6)
		}
		pressKey(m, name)
		for _, modifier := range modifiers {
			m.PutKey(codes[modifier], false)
			m.RunFrames(6)
		}
	}
}

/*
applicationVolume is the name of the volume the application running was
started from: CurApRefNum at $0900 is the reference number of its resource
file, which is the offset of its file control block in the table at FCBSPtr,
$034E, whose field at 20 is the control block of its volume
*/
func applicationVolume(m *Mac) string {
	const curApRefNum, fcbsPtr, fcbVPtr, vcbName = 0x0900, 0x034e, 20, 44
	readLong := func(address uint32) uint32 {
		return uint32(m.mm.Peek(address))<<24 | uint32(m.mm.Peek(address+1))<<16 |
			uint32(m.mm.Peek(address+2))<<8 | uint32(m.mm.Peek(address+3))
	}
	ref := uint32(m.mm.Peek(curApRefNum))<<8 | uint32(m.mm.Peek(curApRefNum+1))
	if ref == 0 {
		return ""
	}
	vcb := readLong(readLong(fcbsPtr) + ref + fcbVPtr)
	name := make([]uint8, min(int(m.mm.Peek(vcb+vcbName)), 27))
	for i := range name {
		name[i] = m.mm.Peek(vcb + vcbName + 1 + uint32(i))
	}
	return string(name)
}

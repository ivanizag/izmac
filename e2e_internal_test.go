package izmac

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/ivanizag/izmac/scrap"
)

/*
The end to end tests that look inside the machine on purpose, and so stay in
the package rather than in e2e_tests with the rest: how many sessions the file
server has open, and the watcher that takes the copies of the machine to the
host, replaced to see that a copy is offered. They run on the images of
test_images, with the few hands they need, which the operator package has
for the others and which a test of this package can not import.
*/

const (
	testImages            = "test_images"
	testRom               = testImages + "/macplus.rom"
	testScsiDriver        = testImages + "/hddriver.img"
	testSystemSixDisk     = testImages + "/system6.img"
	testSystemSixDiskette = testImages + "/system6.dsk"

	// bootFrames is long enough for System 6 to reach its Finder
	bootFrames = 2400
)

// buildTestMac validates a configuration and builds its machine, which is
// closed when the test ends
func buildTestMac(t testing.TB, config *Configuration) *Mac {
	t.Helper()
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	m, err := NewMac(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	return m
}

// bootedMac is the machine on a copy of the System 6 disk, at its Finder
func bootedMac(t *testing.T) *Mac {
	t.Helper()
	config := testConfig(t)
	config.DiskFiles = []string{testImage(t, testSystemSixDisk)}
	m := buildTestMac(t, config)
	m.RunFrames(bootFrames)
	return m
}

/*
testConfig is a configuration for a machine on the test images, with nothing
in its drives yet, for an end to end test, which -short skips: the test ROM, the SCSI driver for bare volumes, and the
parameter RAM in a file of the test's own. The default is a file on the
working directory that outlives the run, and the clock starts from what it
holds: a machine booted from a parameter RAM left by a run an hour ago
believes it is an hour ago.
*/
func testConfig(t testing.TB) *Configuration {
	t.Helper()
	if testing.Short() {
		t.Skip("an end to end test, which -short leaves out")
	}

	config := NewConfiguration()
	config.RomFile = testRom
	config.ScsiDriverFile = testScsiDriver
	config.PramFile = filepath.Join(t.TempDir(), "pram.bin")
	config.Messages = io.Discard
	return config
}

// testImage is a copy of one of the test images, in the test's own directory
func testImage(t testing.TB, name string) string {
	t.Helper()

	source, err := os.Open(name)
	if err != nil {
		t.Fatalf("the test image %v is missing: %v", name, err)
	}
	defer source.Close()

	copied := filepath.Join(t.TempDir(), filepath.Base(name))
	target, err := os.Create(copied)
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()

	if _, err := io.Copy(target, source); err != nil {
		t.Fatal(err)
	}
	return copied
}

// fileServerMac is System 6.0.8 from the test diskette, the one with
// AppleShare in its System Folder, sharing a folder
func fileServerMac(t *testing.T, share string) *Mac {
	t.Helper()
	config := testConfig(t)
	config.Diskettes = []string{testImage(t, testSystemSixDiskette)}
	config.RamSizeKb = 4096
	config.Share = share
	config.PrinterPort = ""
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	m, err := NewMac(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.fileServer.Stop)
	m.RunFrames(3000)
	return m
}

// chooseFileServer goes to the server in the Chooser, as far as the dialog
// asking how to log in
func chooseFileServer(t *testing.T, m *Mac) {
	t.Helper()
	moveMouseTo(t, m, 16, 10)
	m.SetMouseButton(true)
	m.RunFrames(30)
	moveMouseTo(t, m, 50, 59)
	m.RunFrames(10)
	m.SetMouseButton(false)
	m.RunFrames(1800)
	moveMouseTo(t, m, 100, 85)
	clickMouse(m)
	m.RunFrames(1200)

	// The server, the one in the list, and OK
	moveMouseTo(t, m, 300, 88)
	clickMouse(m)
	m.RunFrames(60)
	moveMouseTo(t, m, 330, 177)
	clickMouse(m)
	m.RunFrames(900)
}

// logInAsGuest presses OK in the dialog asking how to log in, as a guest,
// which lists the volumes
func logInAsGuest(t *testing.T, m *Mac) {
	t.Helper()
	moveMouseTo(t, m, 390, 258)
	clickMouse(m)
	m.RunFrames(900)
}

/*
The file server, from a Macintosh. The Chooser finds the server by NBP, gets
its status by ASP to ask how to log in, and logs in as a guest, which opens a
session and asks the server for its volumes; Quit in that dialog closes the
session again.
*/
func TestTheChooserLogsInToTheFileServer(t *testing.T) {
	t.Parallel()
	m := fileServerMac(t, t.TempDir())
	chooseFileServer(t, m)
	if n := m.fileServer.Sessions(); n != 0 {
		t.Fatalf("%v sessions are open before logging in", n)
	}

	logInAsGuest(t, m)
	if n := m.fileServer.Sessions(); n != 1 {
		t.Fatalf("%v sessions are open after logging in, wanted one", n)
	}

	// Quit leaves the server
	moveMouseTo(t, m, 166, 258)
	clickMouse(m)
	m.RunFrames(600)
	if n := m.fileServer.Sessions(); n != 0 {
		t.Errorf("%v sessions are still open after quitting", n)
	}
}

/*
pasteFrames is how long the machine is given to take a paste. An application
asks for an event many times a second, so this is generous: what it is really
waiting for is a Finder that has finished starting up.
*/
const pasteFrames = 180

// pasteOnTheMachine delivers a paste and answers with what the Scrap Manager
// made of it
func pasteOnTheMachine(t *testing.T, m *Mac, text string) string {
	t.Helper()

	m.startPaste(text)
	for frames := 0; frames < pasteFrames && m.pastePending; frames++ {
		m.RunFrames(1)
	}
	if m.pastePending {
		t.Fatal("the paste was never taken by the machine")
	}

	// The Scrap Manager has the text now, so the way out of the emulator can
	// read it back off the scrap it built
	onTheScrap, found := scrap.Text(m.mm)
	if !found {
		stuff := scrap.Read(m.mm)
		t.Fatalf("there is no text on the scrap after the paste, the record reads %+v", stuff)
	}
	return onTheScrap
}

/*
And the way back: what the Scrap Manager was given is offered to the host,
once, by the watcher that looks at the scrap once a frame. The paste puts it
there rather than an application copying it, which is enough to exercise the
reading: the Scrap Manager built the block either way.
*/
func TestTheScrapOfTheSystemReachesTheHost(t *testing.T) {
	t.Parallel()
	m := bootedMac(t)

	// Whatever the boot left on the scrap is taken as the starting point,
	// and a paste of ours is suppressed on purpose, so the copy the host is
	// offered has to be one the machine made afterwards
	m.RunFrames(30)
	m.TakeCopiedText()

	pasteOnTheMachine(t, m, "Pasted from the host")
	m.RunFrames(30)

	if text, copied := m.TakeCopiedText(); copied {
		t.Errorf("the text pasted from the host came back to it as %q", text)
	}

	// A copy on the machine, which is the Scrap Manager being driven by the
	// emulator in exactly the way an application drives it
	m.clipboard.watcher = scrap.NewWatcher()
	m.RunFrames(30)
	m.TakeCopiedText()

	m.startPaste("Copied on the machine")
	for frames := 0; frames < pasteFrames && m.pastePending; frames++ {
		m.RunFrames(1)
	}
	m.clipboard.watcher.Suppress("")
	m.RunFrames(30)

	text, copied := m.TakeCopiedText()
	if !copied {
		t.Fatal("what the Scrap Manager holds was never offered to the host")
	}
	if text != "Copied on the machine" {
		t.Errorf("the host was offered %q", text)
	}
}

// moveMouseTo pushes the pointer to a place on the screen a bit at a time,
// since the ROM scales what the mouse reports and one push does not arrive
func moveMouseTo(t *testing.T, m *Mac, wantH int16, wantV int16) {
	t.Helper()

	at := func() (int16, int16) {
		h, v := m.PointerPosition()
		return int16(h), int16(v)
	}

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

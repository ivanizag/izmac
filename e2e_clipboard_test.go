package izmac

import (
	"os"
	"testing"

	"github.com/ivanizag/izmac/scrap"
)

/*
The clipboard against a real System, which is the only place the program that
puts the text on the scrap can be shown to work. The unit tests plant it and
run it and check that the machine comes back unharmed, but the trap it makes
is answered there by a handler that does nothing. Here it is answered by the
Scrap Manager, and what comes out of it is the clipboard the Finder and every
application will read.
*/

/*
systemSevenDisk is a second image, of a System 7 rather than the System 6 the
other end to end tests boot. Nothing in the clipboard asks which System it is
talking to, and this is what says so.

It takes about twice as long to reach the Finder, and a paste delivered before
it gets there goes to whatever asked for an event on the way and is lost when
the Finder starts. That is not a bug to fix: a paste can only go to the
application that is running, and during a boot there is not one yet.
*/
const (
	systemSevenDisk       = "frontend/macebiten/HD20SC_7.0.vhd"
	systemSevenBootFrames = 6000
)

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

// A paste on the System the other end to end tests boot
func TestAPasteReachesTheScrapOfTheSystem(t *testing.T) {
	m := bootedMac(t)

	const text = "Pasted from the host"
	if onTheScrap := pasteOnTheMachine(t, m, text); onTheScrap != text {
		t.Errorf("the scrap holds %q after pasting %q", onTheScrap, text)
	}
}

/*
The same paste on System 7, where an application asks for its events with a
different trap and the Finder is always running behind whatever else is. The
image is not in the repository and neither is the other one, so this skips the
way the rest of the end to end tests do.
*/
func TestAPasteReachesTheScrapOfSystemSeven(t *testing.T) {
	config := realConfig(t)
	if _, err := os.Stat(systemSevenDisk); err != nil {
		t.Skipf("%v is not here, this test needs it", systemSevenDisk)
	}

	config.DiskFiles = []string{systemSevenDisk}
	config.RamSizeKb = 4096
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}

	m, err := NewMac(config)
	if err != nil {
		t.Fatal(err)
	}
	m.RunFrames(systemSevenBootFrames)

	const text = "Pasted from the host"
	if onTheScrap := pasteOnTheMachine(t, m, text); onTheScrap != text {
		t.Errorf("the scrap holds %q after pasting %q", onTheScrap, text)
	}
}

/*
The line endings and the characters, over the same round trip. A paste that
arrives with the line endings of the host in it is one long paragraph in
MacWrite, and the accents are where the Macintosh had them and not where
Unicode later put them.
*/
func TestAPasteArrivesAsTheMacintoshWritesText(t *testing.T) {
	m := bootedMac(t)

	onTheScrap := pasteOnTheMachine(t, m, "one\ntwo\r\ncafé")
	if onTheScrap != "one\ntwo\ncafé" {
		t.Errorf("the scrap holds %q", onTheScrap)
	}

	// And on the scrap itself it is a carriage return and a Mac OS Roman
	// e acute, which is what an application will read
	data := scrap.Data(m.mm, scrap.Read(m.mm))
	entry, found := scrap.Entry(data, scrap.TypeText)
	if !found {
		t.Fatal("there is no text entry on the scrap")
	}
	if string(entry) != "one\rtwo\rcaf\x8e" {
		t.Errorf("the text entry of the scrap is % x", entry)
	}
}

/*
And the way back: what the Scrap Manager was given is offered to the host,
once, by the watcher that looks at the scrap once a frame. The paste puts it
there rather than an application copying it, which is enough to exercise the
reading: the Scrap Manager built the block either way.
*/
func TestTheScrapOfTheSystemReachesTheHost(t *testing.T) {
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

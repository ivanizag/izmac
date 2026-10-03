package izmac

import (
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
systemSevenBootFrames is how long System 7 takes to reach the Finder from the
System 7 test disk, about twice what System 6 takes. A paste delivered before
it gets there goes to whatever asked for an event on the way and is lost when
the Finder starts. That is not a bug to fix: a paste can only go to the
application that is running, and during a boot there is not one yet.
*/
const systemSevenBootFrames = 6000

// systemSevenMac is the machine on a copy of the System 7 test disk, with the
// memory System 7 wants, booted to the Finder
func systemSevenMac(t *testing.T) *Mac {
	t.Helper()
	config := testConfig(t)
	config.DiskFiles = []string{testImage(t, testSystemSevenDisk)}
	config.RamSizeKb = 4096
	m := buildTestMac(t, config)
	m.RunFrames(systemSevenBootFrames)
	return m
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
different trap and the Finder is always running behind whatever else is.
Nothing in the clipboard asks which System it is talking to, and this is what
says so.
*/
func TestAPasteReachesTheScrapOfSystemSeven(t *testing.T) {
	m := systemSevenMac(t)

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

/*
systemSevenFinder is System 7 at its Finder with the window of its disk open,
the icons in it where a Finder that has just started puts them: Read Me at
38,92, then Shared, System Folder and TeachText. A Finder left running a while
longer arranges them otherwise.
*/
func systemSevenFinder(t *testing.T) *Mac {
	t.Helper()
	config := testConfig(t)
	config.DiskFiles = []string{testImage(t, testSystemSevenDisk)}
	config.RamSizeKb = 4096
	m := buildTestMac(t, config)
	waitForApplication(t, m, "Finder", 120)
	m.RunFrames(600)
	pressCommand(m, "O")
	m.RunFrames(600)
	return m
}

// switchToApplication brings an application to the front from the application
// menu of System 7, at the right of the menu bar, by where it is in it
func switchToApplication(t *testing.T, m *Mac, name string, itemV int16) {
	t.Helper()
	moveMouseTo(t, m, 488, 10)
	m.SetMouseButton(true)
	m.RunFrames(30)
	moveMouseTo(t, m, 435, itemV)
	m.RunFrames(10)
	m.SetMouseButton(false)
	waitForApplication(t, m, name, 30)
	m.RunFrames(120)
}

/*
The clipboard both ways with an application running, on System 7, where the
Finder runs behind it. An application keeps what it copies to itself until it
is switched out, and reads the scrap again when it is switched back in, so
each way crosses a switch: TeachText copies its text and the host has it once
the Finder is in front; the host pastes while the Finder is, TeachText pastes
that once it is back, a word is typed after it, and the whole of it, copied,
is the host's when the Finder is in front again.
*/
func TestTheClipboardCrossesApplicationsBothWays(t *testing.T) {
	const finder, teachText = 92, 110
	m := systemSevenFinder(t)
	doubleClickAt(t, m, 38, 92)
	waitForApplication(t, m, "TeachText", 30)
	m.RunFrames(600)
	m.TakeCopiedText()

	pressCommand(m, "A")
	pressCommand(m, "C")
	m.RunFrames(60)
	switchToApplication(t, m, "Finder", finder)
	const document = "This is a text file on the System 7 test disk.\n"
	if text, copied := m.TakeCopiedText(); !copied || text != document {
		t.Errorf("the host was offered %q, %v, wanted the text TeachText copied", text, copied)
	}

	pasteOnTheMachine(t, m, "pasted from the host")
	m.RunFrames(60)
	switchToApplication(t, m, "TeachText", teachText)
	pressCommand(m, "A")
	pressCommand(m, "V")
	typeText(m, " again")
	pressCommand(m, "A")
	pressCommand(m, "C")
	m.RunFrames(60)
	m.TakeCopiedText()
	switchToApplication(t, m, "Finder", finder)

	if text, copied := m.TakeCopiedText(); !copied || text != "pasted from the host again" {
		t.Errorf("the host was offered %q, %v, wanted what was pasted and typed", text, copied)
	}
}

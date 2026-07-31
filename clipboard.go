package izmac

import (
	"bytes"
	"fmt"
	"sync/atomic"

	"github.com/ivanizag/izmac/scrap"
)

/*
The clipboard of the machine joined to the clipboard of the host.

The two directions are not alike. A copy made on the machine only has to be
noticed, which is a couple of peeks at the low memory the Scrap Manager keeps
its record in, taken once a frame. A paste from the host has to be put on the
scrap, and only the machine can do that, so the machine is made to run a short
program of ours. Both of those live in the scrap package; what is here is the
wiring: when to look, when it is safe to run the program, and how to get the
text across to the goroutine of the frontend and back.

Nothing here asks what System is running. The two things that would tempt one
to, the scrap being unloaded to disk and an application publishing its scrap
only when it is switched out, are answered by reading the record of the Scrap
Manager itself, which says the same thing on every System.
*/
type clipboard struct {
	watcher *scrap.Watcher

	/*
		copied is the text copied on the machine and not yet taken by the
		frontend, and note is anything the clipboard has to say to whoever
		is watching. Both are written by the emulation goroutine and read by
		the frontend, so they are kept where both can reach them safely.
	*/
	copied atomic.Pointer[string]
	note   atomic.Pointer[string]

	// pending is the paste waiting for the machine to reach a point where
	// it can be delivered, and running tells that it is being delivered now
	pending *scrap.Injection
	running bool

	// saved is the state of the processor while the program of ours runs in
	// its place, and waiting how many frames are left before a paste that
	// never found its moment is given up on
	saved   []byte
	waiting int
}

/*
pasteTimeoutFrames is how long a paste waits for the machine to ask for an
event. An application does that many times a second, so a paste that has not
been taken in ten seconds is not going to be: the machine is at the disk
question mark, or in a modal loop of the ROM, or halted.
*/
const pasteTimeoutFrames = 10 * 60

func newClipboard() *clipboard {
	return &clipboard{watcher: scrap.NewWatcher()}
}

// HasClipboard tells whether the clipboard of the machine is shared with the
// one of the host
func (m *Mac) HasClipboard() bool {
	return m.clipboard != nil
}

/*
PasteText puts text from the host on the clipboard of the machine. It returns
as soon as the text is queued: the paste itself waits for the machine to be
somewhere it can be interrupted, which is usually the next frame and is never
the moment this is called.
*/
func (m *Mac) PasteText(text string) {
	if m.clipboard == nil {
		return
	}
	m.commandChannel <- &commandText{id: CommandPasteText, text: text}
}

// TakeCopiedText returns the text copied on the machine since the last call,
// and whether there was any
func (m *Mac) TakeCopiedText() (string, bool) {
	if m.clipboard == nil {
		return "", false
	}
	return take(&m.clipboard.copied)
}

/*
TakeClipboardNote returns what the clipboard has to say about a paste that did
not work, for a frontend to show. Copying and pasting are not asked for one at
a time and mostly say nothing, so this is how the one case that needs a word
gets one.
*/
func (m *Mac) TakeClipboardNote() (string, bool) {
	if m.clipboard == nil {
		return "", false
	}
	return take(&m.clipboard.note)
}

func take(from *atomic.Pointer[string]) (string, bool) {
	text := from.Swap(nil)
	if text == nil {
		return "", false
	}
	return *text, true
}

func (c *clipboard) say(note string) {
	c.note.Store(&note)
}

// startPaste takes a paste asked for by the frontend. It runs on the emulation
// goroutine, as everything else here does.
func (m *Mac) startPaste(text string) {
	if m.clipboard == nil {
		return
	}

	/*
		A paste already running is the machine part way through the program
		of ours, with its registers put aside waiting to be given back.
		Replacing it there would leave the machine running our leftovers with
		nothing to restore, so the new one is refused. The window is a few
		hundred cycles wide and only a second press of the paste key can land
		in it, which is why saying so is enough and queueing is not needed.
	*/
	if m.clipboard.running {
		m.clipboard.say("Nothing was pasted: the last one is still going through")
		return
	}

	injection, err := scrap.NewInjection(text)
	if err != nil {
		m.clipboard.say(fmt.Sprintf("Nothing was pasted: %v", err))
		return
	}

	m.clipboard.pending = injection
	m.clipboard.running = false
	m.clipboard.waiting = pasteTimeoutFrames
	m.pastePending = true
}

/*
clipboardFrame looks at the scrap once a frame. It is where a copy made on the
machine is noticed, and where a paste that the machine never took is given up
on.
*/
func (m *Mac) clipboardFrame() {
	c := m.clipboard

	// While the overlay is on there is ROM where the low memory globals
	// belong, so there is nothing to read yet
	if !m.mm.overlay {
		if text, found := c.watcher.Poll(m.mm); found {
			c.copied.Store(&text)
		}
	}

	if c.pending == nil || c.running {
		return
	}

	c.waiting--
	if c.waiting <= 0 {
		c.say("Nothing was pasted: the machine never asked for an event")
		m.cancelPaste()
	}
}

/*
clipboardStep runs before every instruction while a paste is waiting, which is
for a fraction of a second and not while the machine is going about its
business. It starts the program of ours when the machine reaches a point where
it can, and takes the machine back when it has run.
*/
func (m *Mac) clipboardStep() {
	c := m.clipboard

	if c.running {
		if m.cpu.GetPC() == c.pending.ReturnPC() {
			m.finishPaste()
		}
		return
	}

	if !c.pending.Ready(m.mm, m.cpu.GetPC(), m.cpu.GetA(7)) {
		return
	}

	var saved bytes.Buffer
	if err := m.cpu.Save(&saved); err != nil {
		c.say(fmt.Sprintf("Nothing was pasted: %v", err))
		m.cancelPaste()
		return
	}

	c.saved = saved.Bytes()
	m.cpu.SetPC(c.pending.Plant(m.mm, m.cpu.GetA(7)))
	c.running = true
}

/*
finishPaste puts the machine back where it was. The registers go back as they
were saved, the program counter with them, so the instruction the machine was
about to run is the trap it was interrupted at and it asks for its event as if
nothing had happened.

The cycle counter of the processor is saved and restored along with the
registers, so the program of ours costs the machine no emulated time. The
counter the rest of the emulator runs on is not touched: it has already
counted those cycles, and the scan lines and the sound that hang from it are
not going to be unwound.
*/
func (m *Mac) finishPaste() {
	c := m.clipboard

	result := c.pending.Result(m.mm)
	text := c.pending.Text()

	if err := m.cpu.Load(bytes.NewReader(c.saved)); err != nil {
		// The machine can not be put back, which leaves it running our
		// program's leftovers. Nothing good follows, so say so loudly.
		c.say(fmt.Sprintf("The machine could not be restored after a paste: %v", err))
		m.cancelPaste()
		return
	}

	if result != 0 {
		c.say(fmt.Sprintf("Nothing was pasted, PutScrap answered %v", result))
	} else {
		// The paste bumped the count of the scrap, and without this the
		// next look at it would send the text straight back to the host
		c.watcher.Suppress(text)
	}

	m.cancelPaste()
}

func (m *Mac) cancelPaste() {
	m.clipboard.pending = nil
	m.clipboard.running = false
	m.clipboard.saved = nil
	m.pastePending = false
}

// resetClipboard forgets what was on the scrap and drops a paste in flight,
// since after a reset there is no application to take it and nothing on the
// scrap that the host has not seen
func (m *Mac) resetClipboard() {
	m.clipboard.watcher = scrap.NewWatcher()
	m.cancelPaste()
}

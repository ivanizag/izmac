package main

import (
	"fmt"

	"github.com/atotto/clipboard"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ivanizag/izmac"
)

/*
The clipboard of the host joined to the one of the machine.

The two directions are asked for differently. A copy on the machine is noticed
by the emulator itself, so all that is left here is to put the text on the
clipboard of the host as it arrives. A paste has to be fetched, and fetching it
means asking the host, which on most systems means running a program: so it is
done when the window is given the focus, which is the moment a copy made
somewhere else has just happened, and on the paste key for when it is not.

Between them they keep one memory of the last text that crossed, in either
direction. Without it, giving the window the focus after a copy on the machine
would push the same text back the other way, and a paste of something the
machine already has would interrupt it for nothing.

None of it is drawn over the screen of the machine. A clipboard that is
working has nothing to say, and the one thing worth saying, that a paste could
not be delivered, is worth reading properly: it goes to the terminal with the
rest of what the emulator reports.
*/
type ebitenClipboard struct {
	m *izmac.Mac

	// crossed is the last text that went either way, which is what says
	// whether the clipboard of the host holds something new
	crossed string

	/*
		focused is whether the window had the focus when it was last looked
		at, and started whether it has been looked at at all. The first
		reading is taken and not acted on: a window is given the focus as it
		opens, and a machine that is still booting has no application to take
		a paste, so pasting there is ten seconds of waiting followed by a
		complaint about a paste nobody asked for.
	*/
	focused bool
	started bool

	// broken tells that the clipboard of the host can not be reached, so
	// that it is tried once and reported once rather than on every frame
	broken bool

	/*
		asked tells that the last paste was one the user asked for, and is
		what decides whether the emulator's answer is worth printing. A paste
		that went of its own accord and could not be delivered, which is what
		clicking on a window that is still booting does, is not something the
		user needs telling about: they did not ask for it and nothing of
		theirs was lost.
	*/
	asked bool
}

func newEbitenClipboard(m *izmac.Mac) *ebitenClipboard {
	return &ebitenClipboard{m: m}
}

// update carries a copy made on the machine over to the host, and takes the
// clipboard of the host when the window is given the focus
func (c *ebitenClipboard) update() {
	if !c.m.HasClipboard() {
		return
	}

	if text, copied := c.m.TakeCopiedText(); copied {
		c.toHost(text)
	}

	focused := ebiten.IsFocused()
	if !c.started {
		c.started = true
	} else if focused && !c.focused {
		c.asked = false
		c.fromHost()
	}
	c.focused = focused

	// A paste the machine could not take is only known on the other side
	if note, has := c.m.TakeClipboardNote(); has && c.asked {
		fmt.Println(note)
	}
}

// paste is the paste key or the menu forcing the clipboard of the host on the
// machine, which always says what came of it
func (c *ebitenClipboard) paste() {
	if !c.m.HasClipboard() {
		fmt.Println("The clipboard is not shared")
		return
	}

	c.asked = true
	fmt.Println(c.fromHost())
}

// toHost puts text copied on the machine on the clipboard of the host
func (c *ebitenClipboard) toHost(text string) {
	c.crossed = text

	if err := clipboard.WriteAll(text); err != nil {
		c.broken = true
		fmt.Printf("The clipboard of the host could not be written: %v\n", err)
	}
}

/*
fromHost puts the clipboard of the host on the clipboard of the machine and
answers with what happened. Text the machine already has is not sent again: a
paste interrupts the application for a moment, and that is not something to do
every time the window is clicked on.
*/
func (c *ebitenClipboard) fromHost() string {
	if c.broken {
		return "The clipboard of the host can not be reached"
	}

	text, err := clipboard.ReadAll()
	if err != nil {
		c.broken = true
		return fmt.Sprintf("The clipboard of the host could not be read: %v", err)
	}

	switch {
	case text == "":
		return "The clipboard of the host is empty"
	case text == c.crossed:
		return "The machine has it already"
	}

	c.crossed = text
	c.m.PasteText(text)
	return "Pasting into the machine"
}

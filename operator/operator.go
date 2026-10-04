/*
Package operator is someone sitting at a Macintosh that izmac emulates: a hand
on the mouse and the keyboard, and eyes on the screen, driving a machine that
a program runs with RunFrames. It is what izmac's end to end tests and the
activities of its documentation are written with, and what anyone writing
their own is meant to use.

Everything is done the way a person would do it, and at the pace of one: the
pointer is pushed across the screen with the mouse rather than put in place, a
click holds the button down for as long as a finger does, and a menu item is
chosen by dragging down the menu. The machine runs while that happens, so it
sees what it would see from a person, and draws what it would draw.

The timings are frames of the machine, sixtieths of a second of its time,
which runs as fast as the host can go. They are what the System and the
applications of the Plus need to take a click as a click, a double click as a
double click, and a key as a key; they were found by trying, and are what the
end to end tests run on.
*/
package operator

import (
	"fmt"
	"strings"

	"github.com/ivanizag/izmac"
)

/*
Operator drives one machine. It holds nothing between calls but the machine,
so making one where it is needed is as good as keeping one.
*/
type Operator struct {
	m *izmac.Mac
}

// New is an operator at a machine
func New(m *izmac.Mac) *Operator {
	return &Operator{m: m}
}

// Mac is the machine the operator is at
func (o *Operator) Mac() *izmac.Mac {
	return o.m
}

// Run lets the machine run for a number of frames, a sixtieth of a second each
func (o *Operator) Run(frames int) {
	if frames > 0 {
		o.m.RunFrames(uint64(frames))
	}
}

const (
	// framesPerSecond is how many frames make a second of the machine
	framesPerSecond = 60

	// The pace of a hand on the mouse
	moveTries       = 400
	moveFrames      = 3
	clickDownFrames = 8
	clickUpFrames   = 20
	doubleGapFrames = 8
	dragHoldFrames  = 20

	// The pace of a hand on a menu: how long the button is held on the
	// title before going down, how long the item is held, and how long
	// the menu takes to go away
	menuOpenFrames  = 30
	menuItemFrames  = 10
	menuCloseFrames = 30
	menuAwayFrames  = 5

	// menuBarV is the height of the titles in the menu bar, and
	// awayH and awayV a place away from any menu to let go of one
	menuBarV = 10
	awayH    = 300
	awayV    = 300

	// The pace of a hand on the keyboard
	keyDownFrames     = 10
	keyUpFrames       = 10
	modifierFrames    = 6
	pasteFrames       = 180
	commandKeyName    = "Command"
	modifierSeparator = "+"
)

/*
MoveTo pushes the pointer to a place on the screen, in its pixels, a bit at a
time. The ROM scales what the mouse reports by how fast it moves, so one push
does not land where it was aimed: it is pushed again from where it got to
until it is there. It fails when the pointer stops short, which is what a
place off the screen or a machine that is not reading its mouse does.
*/
func (o *Operator) MoveTo(h int, v int) error {
	for try := 0; try < moveTries; try++ {
		atH, atV := o.m.PointerPosition()
		if atH == h && atV == v {
			return nil
		}
		o.m.MoveMouse(h-atH, v-atV)
		o.Run(moveFrames)
	}
	atH, atV := o.m.PointerPosition()
	return fmt.Errorf("the pointer stopped at %v,%v on the way to %v,%v", atH, atV, h, v)
}

// Hold presses the button down and leaves it down, and Release lets it go:
// the machine has to be run in between for it to see anything
func (o *Operator) Hold() {
	o.m.SetMouseButton(true)
}

// Release lets go of the button
func (o *Operator) Release() {
	o.m.SetMouseButton(false)
}

// Click presses and releases the only button the machine has, where the
// pointer is
func (o *Operator) Click() {
	o.Hold()
	o.Run(clickDownFrames)
	o.Release()
	o.Run(clickUpFrames)
}

// ClickAt moves the pointer to a place and clicks there
func (o *Operator) ClickAt(h int, v int) error {
	if err := o.MoveTo(h, v); err != nil {
		return err
	}
	o.Click()
	return nil
}

// DoubleClick clicks twice where the pointer is, close enough together for
// the machine to take them as one double click
func (o *Operator) DoubleClick() {
	o.Click()
	o.Run(doubleGapFrames)
	o.Click()
}

// DoubleClickAt moves the pointer to a place and double clicks there, which
// opens what is there
func (o *Operator) DoubleClickAt(h int, v int) error {
	if err := o.MoveTo(h, v); err != nil {
		return err
	}
	o.DoubleClick()
	return nil
}

/*
Drag takes what is at a place to another, as a hand does: the button pressed
on it and held, the pointer moved there by way of the middle, and the button
let go. What happens on letting go, a copy or a move, is left to run.
*/
func (o *Operator) Drag(fromH int, fromV int, toH int, toV int) error {
	if err := o.MoveTo(fromH, fromV); err != nil {
		return err
	}
	o.Hold()
	o.Run(dragHoldFrames)
	if err := o.MoveTo((fromH+toH)/2, (fromV+toV)/2); err != nil {
		return err
	}
	if err := o.MoveTo(toH, toV); err != nil {
		return err
	}
	o.Run(dragHoldFrames)
	o.Release()
	return nil
}

// OpenMenu presses the button on the title of a menu of the menu bar, at
// its place across, and holds it open
func (o *Operator) OpenMenu(titleH int) error {
	if err := o.MoveTo(titleH, menuBarV); err != nil {
		return err
	}
	o.Hold()
	o.Run(menuOpenFrames)
	return nil
}

// CloseMenu lets go of a menu held open without choosing anything in it, away
// from it
func (o *Operator) CloseMenu() error {
	if err := o.MoveTo(awayH, awayV); err != nil {
		return err
	}
	o.Run(menuAwayFrames)
	o.Release()
	o.Run(menuCloseFrames)
	return nil
}

/*
ChooseFromMenu opens a menu by its title and chooses the item at a height in
it. It goes down the menu first and across after, a little into it: across
the menu bar would be into the next menu.
*/
func (o *Operator) ChooseFromMenu(titleH int, itemV int) error {
	if err := o.OpenMenu(titleH); err != nil {
		return err
	}
	if err := o.MoveTo(titleH, itemV); err != nil {
		return err
	}
	if err := o.MoveTo(titleH+30, itemV); err != nil {
		return err
	}
	o.Run(menuItemFrames)
	o.Release()
	return nil
}

// keyCode is the code of a key by its name in izmac.KeyCodes
func keyCode(name string) (uint8, error) {
	code, ok := izmac.KeyCodes()[name]
	if !ok {
		return 0, fmt.Errorf("there is no key called %q", name)
	}
	return code, nil
}

// HoldKey presses a key by its name and leaves it down, as a modifier is held
// while something else is done
func (o *Operator) HoldKey(name string) error {
	code, err := keyCode(name)
	if err != nil {
		return err
	}
	o.m.PutKey(code, true)
	return nil
}

// ReleaseKey lets go of a key held down
func (o *Operator) ReleaseKey(name string) error {
	code, err := keyCode(name)
	if err != nil {
		return err
	}
	o.m.PutKey(code, false)
	return nil
}

// PressKey taps a key by the name izmac.KeyCodes knows it by
func (o *Operator) PressKey(name string) error {
	if err := o.HoldKey(name); err != nil {
		return err
	}
	o.Run(keyDownFrames)
	if err := o.ReleaseKey(name); err != nil {
		return err
	}
	o.Run(keyUpFrames)
	return nil
}

// Command taps a key with the command key held, the shortcut of a menu item,
// quickly, as a hand that already has a finger on the command key does
func (o *Operator) Command(name string) error {
	return o.chord([]string{commandKeyName}, name, true)
}

/*
TypeKeys types keys by their names, each with the modifiers before it held
down, as "Shift+1" or "Option+E", one after the other as a hand would
*/
func (o *Operator) TypeKeys(keys ...string) error {
	for _, key := range keys {
		parts := strings.Split(key, modifierSeparator)
		if err := o.chord(parts[:len(parts)-1], parts[len(parts)-1], false); err != nil {
			return err
		}
	}
	return nil
}

/*
chord holds the modifiers down, taps a key, and lets go of them in the order
they were pressed. A quick tap is the one of a shortcut, a key typed is
pressed as long as any other.
*/
func (o *Operator) chord(modifiers []string, name string, quick bool) error {
	for _, modifier := range modifiers {
		if err := o.HoldKey(modifier); err != nil {
			return err
		}
		o.Run(modifierFrames)
	}
	if quick {
		code, err := keyCode(name)
		if err != nil {
			return err
		}
		o.m.PutKey(code, true)
		o.Run(modifierFrames)
		o.m.PutKey(code, false)
		o.Run(modifierFrames)
	} else if err := o.PressKey(name); err != nil {
		return err
	}
	for _, modifier := range modifiers {
		if err := o.ReleaseKey(modifier); err != nil {
			return err
		}
		o.Run(modifierFrames)
	}
	return nil
}

/*
TypeString types a text on the keyboard of the Plus, the United States one:
capitals and the punctuation of the upper row of a key with the shift key, a
new line with the return key and a tab with the tab key. A character the
keyboard has no key for, an accented letter among them, is an error, before
anything is typed.
*/
func (o *Operator) TypeString(text string) error {
	keys := make([]string, 0, len(text))
	for _, r := range text {
		key, ok := keyFor(r)
		if !ok {
			return fmt.Errorf("there is no key for %q on the keyboard", r)
		}
		keys = append(keys, key)
	}
	return o.TypeKeys(keys...)
}

// keyFor is the key that types a character, with the shift key if it takes it
func keyFor(r rune) (string, bool) {
	switch {
	case r >= 'a' && r <= 'z':
		return string(r - 'a' + 'A'), true
	case r >= 'A' && r <= 'Z':
		return "Shift+" + string(r), true
	case r >= '0' && r <= '9':
		return string(r), true
	}
	key, ok := punctuationKeys()[r]
	return key, ok
}

// punctuationKeys is the key of each character that is not a letter or a
// digit, on the keyboard of the Plus
func punctuationKeys() map[rune]string {
	return map[rune]string{
		' ': "Space", '\n': "Return", '\t': "Tab",
		'.': "Period", ',': "Comma", '\'': "Quote", ';': "Semicolon",
		'-': "Minus", '=': "Equal", '/': "Slash", '[': "LeftBracket",
		']': "RightBracket", '\\': "Backslash", '`': "Backquote",
		'"': "Shift+Quote", ':': "Shift+Semicolon", '!': "Shift+1",
		'@': "Shift+2", '#': "Shift+3", '$': "Shift+4", '%': "Shift+5",
		'^': "Shift+6", '&': "Shift+7", '*': "Shift+8", '(': "Shift+9",
		')': "Shift+0", '_': "Shift+Minus", '+': "Shift+Equal",
		'?': "Shift+Slash", '<': "Shift+Comma", '>': "Shift+Period",
		'{': "Shift+LeftBracket", '}': "Shift+RightBracket",
		'|': "Shift+Backslash", '~': "Shift+Backquote",
	}
}

/*
Paste hands text to the machine as a paste from the host, the way the
frontend does when its window takes the focus, and runs the machine until the
Scrap Manager has taken it. The text is then the Clipboard of the Macintosh,
for the application to paste with its own Paste. It fails when nothing takes
it in three seconds, which is a machine with no application asking for
events.
*/
func (o *Operator) Paste(text string) error {
	o.m.PasteText(text)
	o.Run(1)
	for frames := 0; frames < pasteFrames && o.m.IsPasting(); frames++ {
		o.Run(1)
	}
	if o.m.IsPasting() {
		return fmt.Errorf("the paste of %q was never taken by the machine", text)
	}
	return nil
}

/*
WaitUntil runs the machine a second of its time at a time until something has
happened, for as many seconds as given, and tells whether it did. What is
done over a network, by another machine or a server, takes the time of the
host while the machine runs faster than that: a wait for it is a wait for
its outcome, not for a number of frames.
*/
func (o *Operator) WaitUntil(seconds int, done func() bool) bool {
	for i := 0; i < seconds; i++ {
		if done() {
			return true
		}
		o.Run(framesPerSecond)
	}
	return done()
}

// WaitForApplication runs the machine until an application is running, by
// its name, for as many seconds of the machine as given
func (o *Operator) WaitForApplication(name string, seconds int) error {
	if !o.WaitUntil(seconds, func() bool { return o.m.CurrentApplication() == name }) {
		return fmt.Errorf("%v was not running after %v seconds, %q is", name, seconds, o.m.CurrentApplication())
	}
	return nil
}

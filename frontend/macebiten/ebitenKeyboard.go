package main

import (
	"github.com/ivanizag/izmac"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

/*
The keyboard of the host mapped onto the one of the Macintosh. Ebiten reports
physical keys rather than characters, which is what is wanted here: the ROM
does its own translation from the codes the keyboard sends, so what has to be
delivered is which key moved and which way, not what it would type.
*/
type ebitenKeyboard struct {
	m *izmac.Mac

	// keys maps a key of the host to the raw code the Macintosh keyboard
	// sends for it
	keys map[ebiten.Key]uint8

	// down is what the machine has been told is held, so that it can be
	// let go of when the keyboard stops being watched
	down map[ebiten.Key]bool
}

func newEbitenKeyboard(m *izmac.Mac) *ebitenKeyboard {
	return &ebitenKeyboard{
		m:    m,
		keys: buildKeyMap(),
		down: make(map[ebiten.Key]bool),
	}
}

// update reports the keys that went down or came up since the last frame
func (k *ebitenKeyboard) update() {
	/*
		A window that has lost the focus is told nothing more, the release of
		a key held as it went included. That matters most for the very
		combinations the host keeps for itself: command-tab takes the focus
		away with the command key down, and the machine would hold it down
		for ever afterwards, which turns every keystroke into a menu
		accelerator.
	*/
	if !ebiten.IsFocused() {
		k.releaseAll()
		return
	}

	k.updateEmulatorKeys()

	for key, code := range k.keys {
		if inpututil.IsKeyJustPressed(key) {
			k.m.PutKey(code, true)
			k.down[key] = true
		}
		if inpututil.IsKeyJustReleased(key) {
			k.m.PutKey(code, false)
			delete(k.down, key)
		}
	}
}

/*
releaseAll tells the machine that everything held has been let go. It is
called when something else takes the keyboard, the menu for instance: the
release of a key held at that moment would otherwise never be reported and
the machine would hold it down for ever.
*/
func (k *ebitenKeyboard) releaseAll() {
	for key := range k.down {
		k.m.PutKey(k.keys[key], false)
		delete(k.down, key)
	}
}

/*
The function keys drive the emulator rather than the machine, the way
izapple2 does it. The Macintosh keyboard has none of them, so they are free
to use and are not in the map above.

	F5        run as fast as the host can, and back
	Ctrl-F5   say what speed it is reaching
	F4        show or hide the trace of the processor
	Ctrl-F2   reset, as the programmer's switch does
	Pause     stop the machine and let it go again
*/
func (k *ebitenKeyboard) updateEmulatorKeys() {
	ctrl := ebiten.IsKeyPressed(ebiten.KeyControlLeft) ||
		ebiten.IsKeyPressed(ebiten.KeyControlRight)

	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyF5):
		if ctrl {
			k.m.SendCommand(izmac.CommandShowSpeed)
		} else {
			k.m.SendCommand(izmac.CommandToggleSpeed)
		}

	case inpututil.IsKeyJustPressed(ebiten.KeyF4):
		k.m.SendCommand(izmac.CommandToggleCPUTrace)

	case inpututil.IsKeyJustPressed(ebiten.KeyF2):
		if ctrl {
			k.m.SendCommand(izmac.CommandReset)
		}

	case inpututil.IsKeyJustPressed(ebiten.KeyPause):
		k.m.SendCommand(izmac.CommandPauseUnpause)
	}
}

/*
buildKeyMap pairs the keys of the host with the names of the table in izmac.
The Macintosh has one key where a modern keyboard has two in a few places,
and both of the host's are sent as the one the Macintosh knows.

The command key of the Macintosh is mapped from two keys of the host: its own
command key, which is what the fingers reach for, and the alt or option key,
which is what is left when the host keeps a combination for itself. Command-Q
and command-tab on macOS never arrive, and on most Linux desktops the super
key belongs to the window manager, so option is there to type those with.
*/
func buildKeyMap() map[ebiten.Key]uint8 {
	codes := izmac.KeyCodes()

	named := map[ebiten.Key]string{
		ebiten.KeyA: "A", ebiten.KeyB: "B", ebiten.KeyC: "C", ebiten.KeyD: "D",
		ebiten.KeyE: "E", ebiten.KeyF: "F", ebiten.KeyG: "G", ebiten.KeyH: "H",
		ebiten.KeyI: "I", ebiten.KeyJ: "J", ebiten.KeyK: "K", ebiten.KeyL: "L",
		ebiten.KeyM: "M", ebiten.KeyN: "N", ebiten.KeyO: "O", ebiten.KeyP: "P",
		ebiten.KeyQ: "Q", ebiten.KeyR: "R", ebiten.KeyS: "S", ebiten.KeyT: "T",
		ebiten.KeyU: "U", ebiten.KeyV: "V", ebiten.KeyW: "W", ebiten.KeyX: "X",
		ebiten.KeyY: "Y", ebiten.KeyZ: "Z",

		ebiten.Key0: "0", ebiten.Key1: "1", ebiten.Key2: "2", ebiten.Key3: "3",
		ebiten.Key4: "4", ebiten.Key5: "5", ebiten.Key6: "6", ebiten.Key7: "7",
		ebiten.Key8: "8", ebiten.Key9: "9",

		ebiten.KeyMinus:        "Minus",
		ebiten.KeyEqual:        "Equal",
		ebiten.KeyBracketLeft:  "LeftBracket",
		ebiten.KeyBracketRight: "RightBracket",
		ebiten.KeyBackslash:    "Backslash",
		ebiten.KeySemicolon:    "Semicolon",
		ebiten.KeyQuote:        "Quote",
		ebiten.KeyBackquote:    "Backquote",
		ebiten.KeyComma:        "Comma",
		ebiten.KeyPeriod:       "Period",
		ebiten.KeySlash:        "Slash",

		ebiten.KeySpace:       "Space",
		ebiten.KeyTab:         "Tab",
		ebiten.KeyEnter:       "Return",
		ebiten.KeyNumpadEnter: "Enter",
		ebiten.KeyBackspace:   "Backspace",
		ebiten.KeyCapsLock:    "CapsLock",

		// The Macintosh has one of each of these, the host has two
		ebiten.KeyShiftLeft:    "Shift",
		ebiten.KeyShiftRight:   "Shift",
		ebiten.KeyControlLeft:  "Option",
		ebiten.KeyControlRight: "Option",
		ebiten.KeyAltLeft:      "Command",
		ebiten.KeyAltRight:     "Command",
		ebiten.KeyMetaLeft:     "Command",
		ebiten.KeyMetaRight:    "Command",
	}

	keys := make(map[ebiten.Key]uint8, len(named))
	for key, name := range named {
		if code, known := codes[name]; known {
			keys[key] = code
		}
	}
	return keys
}

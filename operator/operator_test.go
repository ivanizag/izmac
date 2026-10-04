package operator

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ivanizag/izmac"
)

/*
What can be seen of an operator without a System to drive: the keys it
would type, and what it says of a machine that does not answer. The end to
end tests are where it drives one, all of it.
*/

// idleMac is a machine started with nothing in it but a blank diskette, on
// the ROM of the tests: it sits at the disk with the question mark, with a
// pointer and a mouse, and nothing to run
func idleMac(t *testing.T) *izmac.Mac {
	t.Helper()
	if testing.Short() {
		t.Skip("a machine started on the ROM, which -short leaves out")
	}
	romFile := filepath.Join("..", "test_images", "macplus.rom")

	// A blank diskette, so that nothing is fetched to start from
	blank := filepath.Join(t.TempDir(), "blank.dsk")
	if err := os.WriteFile(blank, make([]uint8, 800*1024), 0o644); err != nil {
		t.Fatal(err)
	}

	config := izmac.NewConfiguration()
	config.RomFile = romFile
	config.Diskettes = []string{blank}
	config.PramFile = filepath.Join(t.TempDir(), "pram.bin")
	config.Messages = io.Discard
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	m, err := izmac.NewMac(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	m.RunFrames(1)
	return m
}

func TestEveryCharacterOfTheKeyboardHasAKeyThatExists(t *testing.T) {
	codes := izmac.KeyCodes()
	for r := rune(0x20); r < 0x7f; r++ {
		key, ok := keyFor(r)
		if !ok {
			t.Errorf("there is no key for %q", r)
			continue
		}
		name := key[strings.LastIndex(key, modifierSeparator)+1:]
		if _, known := codes[name]; !known {
			t.Errorf("%q is typed with %q, which is not a key", r, key)
		}
	}
}

func TestCapitalsAndSymbolsTakeTheShiftKey(t *testing.T) {
	for r, wanted := range map[rune]string{'a': "A", 'A': "Shift+A", '7': "7", '&': "Shift+7", '\n': "Return"} {
		if key, _ := keyFor(r); key != wanted {
			t.Errorf("%q is typed with %q, wanted %q", r, key, wanted)
		}
	}
}

func TestACharacterWithNoKeyIsNotTyped(t *testing.T) {
	o := New(idleMac(t))
	before := o.Mac().GetFrames()
	if err := o.TypeString("café"); err == nil {
		t.Errorf("an é was typed on a keyboard with no key for it")
	}
	if o.Mac().GetFrames() != before {
		t.Errorf("the letters before it were typed all the same")
	}
}

func TestAnUnknownKeyIsAnError(t *testing.T) {
	o := New(idleMac(t))
	if err := o.PressKey("Escape"); err == nil {
		t.Errorf("a key the Plus does not have was pressed")
	}
}

func TestThePointerIsPushedWhereItIsAimed(t *testing.T) {
	o := New(idleMac(t))
	if err := o.MoveTo(200, 150); err != nil {
		t.Fatal(err)
	}
	if h, v := o.Mac().PointerPosition(); h != 200 || v != 150 {
		t.Errorf("the pointer is at %v,%v, wanted 200,150", h, v)
	}
}

func TestAPointerAimedOffTheScreenIsAnError(t *testing.T) {
	o := New(idleMac(t))
	if err := o.MoveTo(-50, 100); err == nil {
		t.Errorf("the pointer got off the screen")
	}
}

func TestWaitingForWhatNeverComesTakesTheTimeGiven(t *testing.T) {
	o := New(idleMac(t))
	before := o.Mac().GetFrames()
	if o.WaitUntil(3, func() bool { return false }) {
		t.Fatalf("what never happens happened")
	}
	if ran := o.Mac().GetFrames() - before; ran != 3*framesPerSecond {
		t.Errorf("the wait ran %v frames, wanted three seconds", ran)
	}
	if err := o.WaitForApplication("Finder", 1); err == nil {
		t.Errorf("the Finder was found running on a machine with no System")
	}
}

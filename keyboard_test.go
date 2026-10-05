package izmac

import "testing"

// ask sends a command and runs the clock on until the answer is due
func ask(k *keyboard, command uint8) uint8 {
	k.command(command)

	for i := 0; i < 100; i++ {
		if answer, answered, _ := k.tick(keyboardSendCycles / 4); answered {
			return answer
		}
	}
	return 0
}

func TestTheModelNumberIsAnswered(t *testing.T) {
	k := newKeyboard()

	if got := ask(k, keyboardCmdModel); got != keyboardModel {
		t.Errorf("the model number reads $%02x, wanted $%02x", got, keyboardModel)
	}
	if keyboardModel&1 == 0 {
		t.Error("the model number does not have its bit 0 set")
	}
}

func TestTheTestCommandIsAcknowledged(t *testing.T) {
	k := newKeyboard()

	if got := ask(k, keyboardCmdTest); got != keyboardAck {
		t.Errorf("the test answered $%02x, wanted the acknowledge $%02x", got, keyboardAck)
	}
}

/*
An idle keyboard answers a Null and not silence. Saying nothing makes the
Macintosh decide the keyboard has been unplugged and start again from the
model number, so this is the path most likely to leave the machine looking
broken.
*/
func TestAnIdleKeyboardAnswersNull(t *testing.T) {
	k := newKeyboard()

	for i := 0; i < 3; i++ {
		if got := ask(k, keyboardCmdInquiry); got != keyboardNull {
			t.Errorf("an idle keyboard answered $%02x, wanted the Null $%02x",
				got, keyboardNull)
		}
	}
}

func TestAKeyIsReportedDownAndUp(t *testing.T) {
	k := newKeyboard()
	const a = 0x01 // The A key

	k.putKey(a, true)
	k.putKey(a, false)

	if got := ask(k, keyboardCmdInquiry); got != a {
		t.Errorf("the key down reads $%02x, wanted $%02x", got, a)
	}
	if got := ask(k, keyboardCmdInquiry); got != a|keyboardKeyUp {
		t.Errorf("the key up reads $%02x, wanted $%02x", got, a|keyboardKeyUp)
	}

	// And then nothing is left
	if got := ask(k, keyboardCmdInquiry); got != keyboardNull {
		t.Errorf("a third inquiry answered $%02x, wanted the Null", got)
	}
}

func TestTheTransitionsComeOutInOrder(t *testing.T) {
	k := newKeyboard()
	codes := keyCodes()

	typed := []string{"H", "E", "L", "L", "O"}
	for _, name := range typed {
		k.putKey(codes[name], true)
		k.putKey(codes[name], false)
	}

	for _, name := range typed {
		if got := ask(k, keyboardCmdInquiry); got != codes[name] {
			t.Fatalf("the down of %v reads $%02x, wanted $%02x", name, got, codes[name])
		}
		if got := ask(k, keyboardCmdInquiry); got != codes[name]|keyboardKeyUp {
			t.Fatalf("the up of %v reads $%02x", name, got)
		}
	}
}

// The driver strips the release bit and shifts the rest one place right, so
// the codes have to land where the software expects them
func TestTheCodesMatchWhatTheDriverMakesOfThem(t *testing.T) {
	codes := keyCodes()

	for _, c := range []struct {
		name string
		key  uint8
	}{
		{"A", 0x00}, {"S", 0x01}, {"Z", 0x06}, {"Q", 0x0c},
		{"Space", 0x31}, {"Return", 0x24}, {"Tab", 0x30},
	} {
		raw, known := codes[c.name]
		if !known {
			t.Fatalf("%v is not in the table", c.name)
		}
		if got := (raw &^ keyboardKeyUp) >> 1; got != c.key {
			t.Errorf("%v is raw $%02x, which the driver reads as $%02x, wanted $%02x",
				c.name, raw, got, c.key)
		}
	}
}

func TestEveryCodeIsDistinctAndWellFormed(t *testing.T) {
	seen := make(map[uint8]string)

	for name, code := range keyCodes() {
		if code&1 == 0 && code&keyboardKeypad == 0 {
			t.Errorf("%v is $%02x, which does not have its bit 0 set", name, code)
		}
		if code == keyboardNull || code == keyboardKeypadPrefix {
			t.Errorf("%v is $%02x, the Null", name, code)
		}
		if other, clash := seen[code]; clash && other != name {
			t.Errorf("%v and %v are both $%02x", name, other, code)
		}
		seen[code] = name
	}
}

func TestTheModelCommandClearsWhatWasWaiting(t *testing.T) {
	k := newKeyboard()
	k.putKey(0x01, true)

	// The keyboard resets itself on a model number command, so a key
	// pressed before the machine noticed it is gone
	ask(k, keyboardCmdModel)

	if got := ask(k, keyboardCmdInquiry); got != keyboardNull {
		t.Errorf("a transition survived the reset, the inquiry answered $%02x", got)
	}
}

func TestTheAnswerIsNotInstant(t *testing.T) {
	k := newKeyboard()
	k.command(keyboardCmdInquiry)

	if _, answered, sent := k.tick(1); answered || sent {
		t.Error("the keyboard answered in the same cycle it was asked")
	}

	// The command finishes going out first
	if _, answered, sent := k.tick(keyboardSendCycles); !sent || answered {
		t.Error("the command did not finish going out before the answer")
	}
	if _, answered, _ := k.tick(keyboardAnswerCycles); !answered {
		t.Error("the keyboard never answered")
	}
}

func TestABurstOfTypingDoesNotGrowForEver(t *testing.T) {
	k := newKeyboard()

	for i := 0; i < 1000; i++ {
		k.putKey(0x01, true)
	}

	if len(k.queue) > keyboardQueueLimit {
		t.Errorf("the queue grew to %v, past the limit of %v",
			len(k.queue), keyboardQueueLimit)
	}
}

// The whole path, from a key going down to the byte appearing in the shift
// register with its interrupt raised
func TestTheKeyboardReachesTheShiftRegister(t *testing.T) {
	v, _, _ := newTestVia(t)
	const a = 0x01

	v.keyboard.putKey(a, true)

	// The processor shifts the inquiry out
	v.poke(viaAddress(viaRegShift), keyboardCmdInquiry)

	// Neither event has happened yet
	if v.mos.Read(13)&viaIntShiftRegister != 0 {
		t.Error("the shift register interrupt was raised before anything was due")
	}

	v.tick(keyboardSendCycles)
	listenForTheKeyboard(v)
	v.tick(keyboardAnswerCycles)

	if v.mos.Read(13)&viaIntShiftRegister == 0 {
		t.Fatal("the answer did not raise the shift register interrupt")
	}
	if got := v.peek(viaAddress(viaRegShift)); got != a {
		t.Errorf("the shift register holds $%02x, wanted $%02x", got, a)
	}

	// Reading it clears the flag
	if v.mos.Read(13)&viaIntShiftRegister != 0 {
		t.Error("reading the shift register did not clear its interrupt")
	}
}

// listenForTheKeyboard does what the ROM does once the command has gone out:
// the shift register turned around to shift in on the keyboard's clock, and
// read, which starts it listening
func listenForTheKeyboard(v *via) {
	const viaRegAuxControl, shiftInExternally = 11, 0x0c
	v.poke(viaAddress(viaRegAuxControl), shiftInExternally)
	v.peek(viaAddress(viaRegShift))
}

/*
A Macintosh busy at a higher interrupt level, with AppleTalk most of all,
turns the shift register around late. The keyboard waits for it, as it waits
for the data line: an answer clocked in before would be read as the end of
the command and thrown away, and the key with it.
*/
func TestTheKeyboardWaitsForTheMacintoshToListen(t *testing.T) {
	v, _, _ := newTestVia(t)
	const a = 0x01

	v.keyboard.putKey(a, true)
	v.poke(viaAddress(viaRegShift), keyboardCmdInquiry)
	v.tick(keyboardSendCycles)

	// Long past when the answer was due, with nobody listening
	for i := 0; i < 10; i++ {
		v.tick(keyboardAnswerCycles)
	}
	if got := v.peek(viaAddress(viaRegShift)); got != keyboardCmdInquiry {
		t.Fatalf("the shift register holds $%02x before the Macintosh listens, wanted the command", got)
	}

	listenForTheKeyboard(v)
	v.tick(1)
	if v.mos.Read(13)&viaIntShiftRegister == 0 {
		t.Fatal("the answer did not arrive once the Macintosh listened")
	}
	if got := v.peek(viaAddress(viaRegShift)); got != a {
		t.Errorf("the shift register holds $%02x, wanted the key $%02x", got, a)
	}
}

// A command sent while an answer waits gives the key in it back to the
// keyboard, which reports it again
func TestAKeyWaitingWhenACommandComesIsNotLost(t *testing.T) {
	v, _, _ := newTestVia(t)
	const a = 0x01

	v.keyboard.putKey(a, true)
	v.poke(viaAddress(viaRegShift), keyboardCmdInquiry)
	v.tick(keyboardSendCycles)
	v.tick(keyboardAnswerCycles)

	// The Macintosh gives up on that inquiry and sends another
	v.poke(viaAddress(viaRegShift), keyboardCmdInquiry)
	v.tick(keyboardSendCycles)
	listenForTheKeyboard(v)
	v.tick(keyboardAnswerCycles)

	if got := v.peek(viaAddress(viaRegShift)); got != a {
		t.Errorf("the second inquiry got $%02x, wanted the key $%02x", got, a)
	}
}

/*
A key of the keypad, or an arrow, comes after the prefix $79, on the way
down and on the way up, and the byte after it is what the driver reads as a
key code $40 higher: the left arrow is $79 $0d, and $46
*/
func TestAnArrowComesAfterThePrefix(t *testing.T) {
	k := newKeyboard()
	left := keyCodes()["Left"]

	k.putKey(left, true)
	k.putKey(left, false)

	for _, want := range []uint8{keyboardKeypadPrefix, 0x0d, keyboardKeypadPrefix, 0x0d | keyboardKeyUp} {
		if got := ask(k, keyboardCmdInstant); got != want {
			t.Errorf("the keyboard answered $%02x, wanted $%02x", got, want)
		}
	}
	if got := ask(k, keyboardCmdInquiry); got != keyboardNull {
		t.Errorf("after the arrow the keyboard answered $%02x, wanted the Null", got)
	}
}

// The key codes the driver makes of the arrows and the keypad, from Inside
// Macintosh volume V for the keyboard of the Plus
func TestTheKeypadCodesMatchWhatTheDriverMakesOfThem(t *testing.T) {
	codes := keyCodes()

	for _, c := range []struct {
		name string
		key  uint8
	}{
		{"Left", 0x46}, {"Right", 0x42}, {"Up", 0x4d}, {"Down", 0x48},
		{"Keypad0", 0x52}, {"Keypad5", 0x57}, {"Keypad9", 0x5c},
		{"Enter", 0x4c}, {"Clear", 0x47},
	} {
		raw := codes[c.name]
		if raw&keyboardKeypad == 0 {
			t.Errorf("%v is not sent after the prefix", c.name)
		}
		if got := (raw&^keyboardKeypad)>>1 + 0x40; got != c.key {
			t.Errorf("%v is raw $%02x, which the driver reads as $%02x, wanted $%02x",
				c.name, raw, got, c.key)
		}
	}
}

// The keyboard says it is the one of the Plus, which has the keypad
func TestTheKeyboardIsThePluses(t *testing.T) {
	if got := ask(newKeyboard(), keyboardCmdModel); got != 0x0b {
		t.Errorf("the model number is $%02x, wanted the $0b of the M0110A", got)
	}
}

/*
An arrow the Macintosh never got goes back with its prefix, or the byte on
its own would be read as a key of the main block: $0d alone is the Z
*/
func TestAnArrowGivenBackKeepsItsPrefix(t *testing.T) {
	k := newKeyboard()
	k.putKey(keyCodes()["Left"], true)

	if got := ask(k, keyboardCmdInquiry); got != keyboardKeypadPrefix {
		t.Fatalf("the keyboard answered $%02x, wanted the prefix", got)
	}
	taken := ask(k, keyboardCmdInstant)
	k.giveBack(taken)

	for _, want := range []uint8{keyboardKeypadPrefix, 0x0d} {
		if got := ask(k, keyboardCmdInquiry); got != want {
			t.Errorf("given back, the keyboard answered $%02x, wanted $%02x", got, want)
		}
	}
}

/*
The + of the keypad is a shifted left arrow: the shift key down, the arrow
after the prefix, and on the way up the shift key up before the arrow
*/
func TestThePlusOfTheKeypadIsAShiftedArrow(t *testing.T) {
	k := newKeyboard()
	plus := keyCodes()["KeypadPlus"]

	k.putKey(plus, true)
	k.putKey(plus, false)

	for _, want := range []uint8{0x71, keyboardKeypadPrefix, 0x0d, 0xf1, keyboardKeypadPrefix, 0x8d} {
		if got := ask(k, keyboardCmdInstant); got != want {
			t.Errorf("the keyboard answered $%02x, wanted $%02x", got, want)
		}
	}
}

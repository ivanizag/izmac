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
		if code&1 == 0 {
			t.Errorf("%v is $%02x, which does not have its bit 0 set", name, code)
		}
		if code&keyboardKeyUp != 0 {
			t.Errorf("%v is $%02x, which collides with the release bit", name, code)
		}
		if code == keyboardNull {
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

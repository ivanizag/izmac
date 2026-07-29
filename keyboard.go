package izmac

/*
The Macintosh keyboard, an Intel 8021 on the end of a four wire telephone
cable. It talks to the VIA shift register over a bidirectional serial link
with the clock on CB1 and the data on CB2, and the protocol is in Inside
Macintosh volume III, pages III-30 and III-31.

The Macintosh starts every exchange. It sends Model Number first and retries
it every half second until something answers, then settles into sending
Inquiry every quarter second. The keyboard answers an Inquiry with a key
transition if one has happened and with a Null if none has. That Null matters
more than it looks: a keyboard that simply says nothing when idle makes the
Macintosh decide it has been unplugged and start over from Model Number, for
ever.

	Inquiry       $10   a key transition, or Null $7b
	Instant       $14   the same without the wait
	Model Number  $16   bit 0 set, the model on bits 1 to 3
	Test          $36   $7d for pass, $77 for fail

A key transition is one byte: bit 7 clear for a press and set for a release,
the code on bits 6 to 1, and bit 0 always set. The driver reads it as
(response & $7f) >> 1.

The eight bits of a byte take about three milliseconds on the wire. That is
not emulated, but the answer is not instant either: it is held back a while,
because a keyboard that replies inside the same instruction that asked is a
situation the ROM never sees on real hardware.
*/
type keyboard struct {
	// queue holds the transitions waiting to be reported
	queue []uint8

	// response is what the keyboard will answer, and delay how many cycles
	// are left before whatever comes next
	response uint8
	delay    uint64
	stage    keyboardStage
}

/*
A byte each way is two events and not one. The chip reports that the eight
bits written to it have gone out, which is what tells the Macintosh to turn
the shift register around and listen, and only then does the answer arrive.
Delivering the answer without the first of those leaves the Macintosh still
waiting to finish sending, and it gives up and asks again for ever.
*/
type keyboardStage int

const (
	keyboardIdle keyboardStage = iota
	keyboardSending
	keyboardAnswering
)

const (
	keyboardCmdInquiry = 0x10
	keyboardCmdInstant = 0x14
	keyboardCmdModel   = 0x16
	keyboardCmdTest    = 0x36

	// keyboardNull is what an Inquiry gets when no key has moved
	keyboardNull uint8 = 0x7b
	keyboardAck  uint8 = 0x7d
	keyboardNak  uint8 = 0x77

	/*
		keyboardModel answers the Model Number command: bit 0 set, the
		model number on bits 1 to 3 and the next device on bits 4 to 6,
		with bit 7 set when something else is chained on. A plain
		keyboard with no keypad is model 1 and nothing beyond it.
	*/
	keyboardModel uint8 = 0x03

	// keyboardKeyUp marks a release
	keyboardKeyUp uint8 = 1 << 7

	// keyboardQueueLimit keeps a burst of typing from growing without end
	// while the ROM is busy elsewhere
	keyboardQueueLimit = 32

	/*
		The wire times, in processor cycles. The Macintosh clocks its
		eight bits out at 400 microseconds each and the keyboard answers
		at 330, so a byte takes about 3.2 milliseconds going out and 2.6
		coming back.
	*/
	keyboardSendCycles   uint64 = 3200 * 7833 / 1000
	keyboardAnswerCycles uint64 = 2600 * 7833 / 1000
)

func newKeyboard() *keyboard {
	return &keyboard{queue: make([]uint8, 0, keyboardQueueLimit)}
}

// PutKey queues a transition. The code is the raw one the keyboard sends,
// from the table in Inside Macintosh, and down says whether the key went
// down or came up.
func (k *keyboard) putKey(code uint8, down bool) {
	if len(k.queue) >= keyboardQueueLimit {
		return
	}

	transition := code
	if !down {
		transition |= keyboardKeyUp
	}
	k.queue = append(k.queue, transition)
}

// command takes a byte the Macintosh shifted out and works out the answer
func (k *keyboard) command(value uint8) {
	switch value {
	case keyboardCmdInquiry, keyboardCmdInstant:
		k.answer(k.nextTransition())
	case keyboardCmdModel:
		// The keyboard resets itself and reports what it is
		k.queue = k.queue[:0]
		k.answer(keyboardModel)
	case keyboardCmdTest:
		k.answer(keyboardAck)
	default:
		// An unknown command is answered with a Null rather than with
		// silence, which would make the Macintosh give up on us
		k.answer(keyboardNull)
	}
}

// nextTransition takes the oldest transition, or the Null that says nothing
// has happened
func (k *keyboard) nextTransition() uint8 {
	if len(k.queue) == 0 {
		return keyboardNull
	}

	transition := k.queue[0]
	k.queue = k.queue[1:]
	return transition
}

func (k *keyboard) answer(value uint8) {
	k.response = value
	k.delay = keyboardSendCycles
	k.stage = keyboardSending
}

/*
tick counts down the wire time. It reports the command finishing its way out
first, and the answer after that, so that the Macintosh gets the two events
the hardware gives it.
*/
func (k *keyboard) tick(cycles uint64) (value uint8, answered bool, sent bool) {
	if k.stage == keyboardIdle {
		return 0, false, false
	}

	if k.delay > cycles {
		k.delay -= cycles
		return 0, false, false
	}

	if k.stage == keyboardSending {
		k.stage = keyboardAnswering
		k.delay = keyboardAnswerCycles
		return 0, false, true
	}

	k.stage = keyboardIdle
	return k.response, true, false
}

func (k *keyboard) reset() {
	k.queue = k.queue[:0]
	k.stage = keyboardIdle
	k.delay = 0
}

/*
keyCodes returns the raw transition codes of the United States keyboard, from
Figure 9 on page III-32 of Inside Macintosh volume III. They are not the codes
the software sees: the driver strips the release bit and shifts the rest one
place right, so the $01 of the A key becomes the key code 0.
*/
func keyCodes() map[string]uint8 {
	return map[string]uint8{
		// The top row, the backquote across to the backspace
		"Backquote": 0x65, "1": 0x25, "2": 0x27, "3": 0x29, "4": 0x2b,
		"5": 0x2f, "6": 0x2d, "7": 0x35, "8": 0x39, "9": 0x33,
		"0": 0x3b, "Minus": 0x37, "Equal": 0x31, "Backspace": 0x67,

		// The second row, the tab across to the backslash
		"Tab": 0x61, "Q": 0x19, "W": 0x1b, "E": 0x1d, "R": 0x1f,
		"T": 0x23, "Y": 0x21, "U": 0x41, "I": 0x45, "O": 0x3f,
		"P": 0x47, "LeftBracket": 0x43, "RightBracket": 0x3d,
		"Backslash": 0x55,

		// The third row, the caps lock across to the return
		"CapsLock": 0x73, "A": 0x01, "S": 0x03, "D": 0x05, "F": 0x07,
		"G": 0x0b, "H": 0x09, "J": 0x4d, "K": 0x51, "L": 0x4b,
		"Semicolon": 0x53, "Quote": 0x4f, "Return": 0x49,

		// The fourth row, between the two shifts, which share a code
		"Shift": 0x71, "Z": 0x0d, "X": 0x0f, "C": 0x11, "V": 0x13,
		"B": 0x17, "N": 0x5b, "M": 0x5d, "Comma": 0x57, "Period": 0x5f,
		"Slash": 0x59,

		// The bottom row. Both option keys share a code as the shifts do,
		// and the enter key beside the space is not the return key.
		"Option": 0x75, "Command": 0x6f, "Space": 0x63, "Enter": 0x69,
	}
}

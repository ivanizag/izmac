package scrap

import "testing"

const (
	// A stack pointer and a heap limit as an application at its event loop
	// would have them, with the eight kilobytes of stack it is promised
	testStackPointer = 0x0009_0000
	testApplLimit    = testStackPointer - 8*1024

	testTrapAddress = 0x0002_0000
)

// readyMemory is a machine where a paste can be delivered: the Scrap Manager
// has started, the application has its stack, and it is at the trap that asks
// for its next event
func readyMemory() testMemory {
	mem := testMemory{}
	mem.pokeWord(stateAddress, 1)
	mem.pokeLong(applLimitAddress, testApplLimit)
	mem.pokeWord(testTrapAddress, trapWaitNextEvent)
	return mem
}

func TestTheProgramIsAsLongAsItSaysItIs(t *testing.T) {
	injection, err := NewInjection("Hello")
	if err != nil {
		t.Fatal(err)
	}

	code := injection.build(testStackPointer-64, testStackPointer)
	if len(code)-len(injection.text) != codeSize {
		t.Errorf("the program is %v bytes and the text is at %v",
			len(code)-len(injection.text), codeSize)
	}
}

/*
The program planted on the stack of the application, checked where it is worth
checking: the two displacements. The one on the LEA is what keeps the call from
pushing over its own text, and the one on the PEA is counted from the extension
word rather than from the instruction, which is a place to be out by two.
*/
func TestTheProgramIsPlantedBelowTheStack(t *testing.T) {
	mem := readyMemory()

	injection, err := NewInjection("Hello")
	if err != nil {
		t.Fatal(err)
	}

	base := injection.Plant(mem, testStackPointer)

	if base >= testStackPointer {
		t.Fatalf("the program was planted at $%06x, at or above the stack pointer $%06x",
			base, testStackPointer)
	}
	if base+uint32(injection.needed()) > testStackPointer {
		t.Errorf("the program and its text reach $%06x, past the stack pointer $%06x",
			base+uint32(injection.needed()), testStackPointer)
	}

	if got := peekWord(mem, base); got != opLeaFromSP {
		t.Errorf("the program starts with $%04x, wanted the LEA $%04x", got, opLeaFromSP)
	}

	// LEA d16(A7),A7 puts the stack just below the program, so what the
	// call pushes grows away from the text
	newStack := uint32(int32(testStackPointer) + int32(int16(peekWord(mem, base+2))))
	if newStack != base {
		t.Errorf("the program moves the stack to $%06x, wanted the $%06x it was planted at",
			newStack, base)
	}

	// PEA d16(PC) has to point at the text, which follows the program
	extension := base + codeSize - 8
	if got := peekWord(mem, extension-2); got != opPeaFromPC {
		t.Fatalf("$%04x is not the PEA $%04x it was taken for", got, opPeaFromPC)
	}
	text := extension + uint32(peekWord(mem, extension))
	if text != base+codeSize {
		t.Errorf("the program points at $%06x for its text, which is at $%06x",
			text, base+codeSize)
	}

	planted := make([]byte, len(injection.text))
	for at := range planted {
		planted[at] = mem.Peek(text + uint32(at))
	}
	if string(planted) != "Hello" {
		t.Errorf("the text was planted as %q", planted)
	}

	// And the last instruction, the one that is never run
	if got := peekWord(mem, injection.ReturnPC()); got != opIllegal {
		t.Errorf("the program ends with $%04x, wanted the ILLEGAL $%04x", got, opIllegal)
	}
}

/*
Where the program may be started. Everything here is a way of ending up with a
Toolbox call made from somewhere the Toolbox can not be called from, which on a
machine with no memory protection is not an error but a corrupted heap.
*/
func TestTheProgramIsOnlyRunWhereItIsSafe(t *testing.T) {
	injection, err := NewInjection("Hello")
	if err != nil {
		t.Fatal(err)
	}

	ready := readyMemory()
	if !injection.Ready(ready, testTrapAddress, testStackPointer) {
		t.Fatal("a machine at its event loop was not taken as ready")
	}

	// Anywhere that is not the trap asking for the next event
	elsewhere := readyMemory()
	elsewhere.pokeWord(testTrapAddress, 0x4e71) // NOP
	if injection.Ready(elsewhere, testTrapAddress, testStackPointer) {
		t.Error("the program would have been run in the middle of an application")
	}

	// Before the Scrap Manager has started there is nothing to put a scrap on
	early := readyMemory()
	early.pokeWord(stateAddress, 0xffff)
	if injection.Ready(early, testTrapAddress, testStackPointer) {
		t.Error("the program would have been run before the Scrap Manager started")
	}

	// And a stack with the heap right underneath it has no room to spare
	tight := readyMemory()
	tight.pokeLong(applLimitAddress, testStackPointer-64)
	if injection.Ready(tight, testTrapAddress, testStackPointer) {
		t.Error("the program would have been planted over the application heap")
	}
}

// The other trap, which is what an application of System 7 calls to ask for
// its next event and what an application of System 6 does not have
func TestBothOfTheEventTrapsAreTaken(t *testing.T) {
	injection, err := NewInjection("Hello")
	if err != nil {
		t.Fatal(err)
	}

	for _, trap := range []uint16{trapGetNextEvent, trapWaitNextEvent} {
		mem := readyMemory()
		mem.pokeWord(testTrapAddress, trap)

		if !injection.Ready(mem, testTrapAddress, testStackPointer) {
			t.Errorf("the trap $%04x was not taken as a place to paste at", trap)
		}
	}
}

/*
A paste larger than the stack of an application can hold is refused rather than
made to fit. The alternative is taking memory that belongs to something else,
and the something else is a running application.
*/
func TestAPasteTooLargeIsRefused(t *testing.T) {
	text := make([]byte, maxTextLength+1)
	for i := range text {
		text[i] = 'a'
	}

	if _, err := NewInjection(string(text)); err == nil {
		t.Error("a paste larger than the limit was accepted")
	}

	if _, err := NewInjection(""); err == nil {
		t.Error("a paste of nothing at all was accepted")
	}
}

// What the machine ends up holding is what the host asked to paste, once the
// line endings and anything the Macintosh has no byte for have been dealt with
func TestThePasteSaysWhatTheMachineWillHold(t *testing.T) {
	injection, err := NewInjection("one\r\ntwo ☃")
	if err != nil {
		t.Fatal(err)
	}

	if text := injection.Text(); text != "one\ntwo ?" {
		t.Errorf("the machine will hold %q, wanted %q", text, "one\ntwo ?")
	}
}

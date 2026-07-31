package izmac

import (
	"bytes"
	"testing"

	"github.com/ivanizag/izmac/storage"
)

/*
The registers the test machine holds while it waits at its event loop, and
where it waits. A paste has to leave every one of them as it found them: the
application whose stack the program of ours ran on carries on with the next
instruction, and the next instruction is the trap it was interrupted at.
*/
const (
	testLoopAddress  = 0x0040_003a
	testUserStack    = 0x0009_0000
	testApplLimit    = 0x0008_0000
	testMarkerD0     = 0x0000_0011
	testMarkerD1     = 0x0000_0022
	testMarkerA0     = 0x00ab_cde0
	testMarkerA1     = 0x0012_3456
	testHandlerRom   = 0x0040_0060
	testSuperStack   = 0x000a_0000
	testProgramStart = 0x0040_0008
)

/*
newPastingMac builds a machine that behaves, for the few instructions this is
about, like an application waiting for an event: it installs a handler for the
line 1010 traps, gives itself a heap limit and a stack, and asks for its next
event for ever.

	MOVE.L  #handler,$0028   ; the line 1010 vector, where a trap ends up
	MOVE.L  #limit,$0130     ; ApplLimit, the floor under the stack
	MOVE.W  #1,$096a         ; the Scrap Manager has started
	MOVE.L  #stack,A0        ; or MOVEA.L #stack,A7 in supervisor mode
	MOVE    A0,USP           ; or NOP
	MOVEQ   #$11,D0          ; markers, to see the registers come back
	MOVEQ   #$22,D1
	MOVE.L  #$00abcde0,A0
	MOVE.L  #$00123456,A1
	MOVE    #$0700,SR        ; or two NOPs, to stay in supervisor mode
	loop:
	_WaitNextEvent
	BRA.S   loop

	handler:
	ADDQ.L  #2,2(A7)         ; step over the trap word and carry on
	RTE

Both modes are built because a Macintosh runs in supervisor mode from the
reset onwards and a paste has to work there, while user mode is where the two
stack pointers are different and a restore that got them the wrong way round
would show. The two are assembled to the same length so that the loop and the
handler are at the same addresses either way.

The handler answers every trap the same way, which is enough here: what is
being tested is that the machine can be taken away and put back, not what the
Scrap Manager does with the text once it has it. That takes a real ROM and a
real System, and is what the end to end tests are for.
*/
func newPastingMac(t *testing.T, userMode bool) *Mac {
	t.Helper()

	data := make([]uint8, storage.RomSize)

	at := 0
	word := func(values ...uint16) {
		for _, value := range values {
			data[at] = uint8(value >> 8)
			data[at+1] = uint8(value)
			at += 2
		}
	}
	long := func(value uint32) {
		word(uint16(value>>16), uint16(value))
	}

	// The reset vectors, the stack of the supervisor well away from the one
	// the application will use
	long(testSuperStack)
	long(testProgramStart)

	word(0x21fc) // MOVE.L #handler,$0028
	long(testHandlerRom)
	word(0x0028)

	word(0x21fc) // MOVE.L #limit,$0130
	long(testApplLimit)
	word(0x0130)

	word(0x31fc, 0x0001, 0x096a) // MOVE.W #1,$096a

	if userMode {
		word(0x207c) // MOVE.L #stack,A0
		long(testUserStack)
		word(0x4e60) // MOVE A0,USP
	} else {
		word(0x2e7c) // MOVEA.L #stack,A7
		long(testUserStack)
		word(0x4e71) // NOP
	}

	word(0x7000 | testMarkerD0) // MOVEQ #$11,D0
	word(0x7200 | testMarkerD1) // MOVEQ #$22,D1
	word(0x207c)                // MOVE.L #$00abcde0,A0
	long(testMarkerA0)
	word(0x227c) // MOVE.L #$00123456,A1
	long(testMarkerA1)

	if userMode {
		word(0x46fc, 0x0700) // MOVE #$0700,SR, into user mode
	} else {
		word(0x4e71, 0x4e71) // NOP NOP, staying in supervisor mode
	}

	if at != testLoopAddress-testProgramStart+8 {
		t.Fatalf("the loop was assembled at $%06x and not at $%06x",
			at+testProgramStart-8, testLoopAddress)
	}
	word(0xa860) // _WaitNextEvent
	word(0x60fc) // BRA.S to the trap

	at = testHandlerRom - 0x400000
	word(0x54af, 0x0002) // ADDQ.L #2,2(A7)
	word(0x4e73)         // RTE

	config := NewConfiguration()
	config.RomFile = "<test>"

	m := newMac(config, storage.RomFromData(data), nil)

	// The machine is started here rather than by the first run, so that the
	// overlay can be taken off: the program writes to the low memory the
	// globals live in, and while the overlay is on that is the ROM
	m.reset()
	m.mm.setOverlay(false)
	runToEventLoop(t, m)

	return m
}

/*
runToEventLoop steps the machine until it is at the trap that asks for an
event. It is not enough to run a frame and look: the machine goes round the
trap, the handler and the branch, so where a frame happens to end says nothing
about whether it got there.
*/
func runToEventLoop(t *testing.T, m *Mac) {
	t.Helper()

	for steps := 0; steps < 100; steps++ {
		if m.cpu.GetPC() == testLoopAddress {
			return
		}
		m.step()
	}

	t.Fatalf("the machine is at $%06x and never reached the event loop at $%06x",
		m.cpu.GetPC(), testLoopAddress)
}

/*
runPaste lets the machine run until the paste has been delivered, one
instruction at a time so that it stops the moment it is: a frame at a time
would leave the machine thousands of instructions past the interesting one,
and what is interesting is exactly the state it was put back into.

The instruction restored along with the registers is run before this returns,
because the restoring happens at the top of the same step that goes on to
execute it. That instruction is the trap the machine was interrupted at, so
the machine ends up in the handler and the address it took the trap from is on
the supervisor stack, which is what says where it came back to.
*/
func runPaste(t *testing.T, m *Mac, text string) {
	t.Helper()

	m.startPaste(text)
	for steps := 0; steps < 1000 && m.pastePending; steps++ {
		m.step()
	}

	if m.pastePending {
		t.Fatal("the paste was not delivered to an application asking for events")
	}
}

/*
returnedTo is the address the machine took its last trap from, off the
exception frame the processor left behind. The frame goes on the supervisor
stack, which is a stack of its own only while the machine is in user mode: a
machine that never left supervisor mode has one stack and the frame is on it.
*/
func returnedTo(m *Mac, userMode bool) uint32 {
	stack := uint32(testUserStack)
	if userMode {
		stack = testSuperStack
	}

	address := uint32(0)
	for i := uint32(0); i < 4; i++ {
		address = address<<8 | uint32(m.mm.Peek(stack-4+i))
	}
	return address
}

/*
A paste, end to end on the emulated machine: the program is planted, run, and
the machine is put back where it was. The registers are what says it worked.
Anything left of ours in one of them is a value an application would carry on
with as if it were its own.
*/
func TestTheMachineIsPutBackAfterAPaste(t *testing.T) {
	modes := []struct {
		name     string
		userMode bool
	}{
		{"in supervisor mode, as a Macintosh runs", false},
		{"in user mode, where the two stack pointers differ", true},
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			m := newPastingMac(t, mode.userMode)
			runPaste(t, m, "Hello")

			if came := returnedTo(m, mode.userMode); came != testLoopAddress {
				t.Errorf("the machine came back at $%06x, wanted the trap at $%06x it was taken from",
					came, testLoopAddress)
			}

			/*
				Round the handler and back to the trap before the registers are
				read. The trap the machine came back to has been taken by now,
				and a processor part way through an exception is not showing
				the registers of the application.
			*/
			runToEventLoop(t, m)

			registers := []struct {
				name   string
				got    uint32
				wanted uint32
			}{
				{"D0", m.cpu.GetD(0), testMarkerD0},
				{"D1", m.cpu.GetD(1), testMarkerD1},
				{"A0", m.cpu.GetA(0), testMarkerA0},
				{"A1", m.cpu.GetA(1), testMarkerA1},
				{"A7", m.cpu.GetA(7), testUserStack},
			}
			for _, register := range registers {
				if register.got != register.wanted {
					t.Errorf("%v came back as $%08x, wanted $%08x",
						register.name, register.got, register.wanted)
				}
			}

			// And it goes on running, which a machine left with our program's
			// leftovers in its registers would not
			m.RunFrames(1)
			runToEventLoop(t, m)
		})
	}
}

/*
The text has to reach the memory of the machine, below the stack pointer of
the application.

This is the machine in user mode, where the trap the machine comes back to
pushes its exception frame on the other stack. In supervisor mode that frame
lands on the last six bytes of the program instead, which is harmless because
the Scrap Manager has taken a copy of the text by then, and would make this a
test of where the frame goes rather than of where the text went.
*/
func TestTheTextIsPlantedOnTheStackOfTheApplication(t *testing.T) {
	m := newPastingMac(t, true)
	runPaste(t, m, "Hello")

	const below = 128
	stack := make([]byte, below)
	for at := range stack {
		stack[at] = m.mm.Peek(testUserStack - below + uint32(at))
	}

	if !bytes.Contains(stack, []byte("Hello")) {
		t.Errorf("the text is not in the %v bytes under the stack pointer: % x", below, stack)
	}
}

/*
A machine that never asks for an event is a machine with no application
running: the disk question mark, or a ROM that has given up. The paste waits,
because the application may be about to start, and then says so rather than
staying pending for ever.
*/
func TestAPasteThatIsNeverTakenIsGivenUpOn(t *testing.T) {
	// The test machine of the run loop, a ROM that branches to itself and
	// asks for nothing
	m := newTestMac(t)
	m.RunFrames(1)

	m.startPaste("Hello")
	m.RunFrames(pasteTimeoutFrames + 1)

	if m.pastePending {
		t.Error("a paste nothing was ever going to take is still waiting")
	}
	if _, said := m.TakeClipboardNote(); !said {
		t.Error("nothing was said about a paste that was never delivered")
	}
}

/*
A copy on the machine, from the scrap in its memory to the frontend. The scrap
is written here rather than copied by an application, since what is being
tested is the way out of the emulator and not the Scrap Manager.
*/
func TestACopyOnTheMachineReachesTheFrontend(t *testing.T) {
	m := newPastingMac(t, true)

	// The first look at the scrap is the one that is remembered and not
	// reported, so it is taken before anything is put there
	m.RunFrames(settleFramesForTest)
	if _, copied := m.TakeCopiedText(); copied {
		t.Fatal("an untouched scrap was offered to the host")
	}

	putTestScrap(m, "copied on the machine")
	m.RunFrames(settleFramesForTest)

	text, copied := m.TakeCopiedText()
	if !copied {
		t.Fatal("a copy on the machine was not offered to the host")
	}
	if text != "copied on the machine" {
		t.Errorf("the copy came out as %q", text)
	}

	// And it is offered once
	if _, copied := m.TakeCopiedText(); copied {
		t.Error("the same copy was offered twice")
	}
}

// settleFramesForTest is long enough for the watcher to be sure the scrap has
// stopped changing
const settleFramesForTest = 8

/*
putTestScrap writes a scrap into the memory of the machine as the Scrap
Manager would leave it: the record in low memory, a handle, and a block of one
text entry.
*/
func putTestScrap(m *Mac, text string) {
	const (
		handle = 0x0005_0000
		block  = 0x0005_1000
	)

	pokeLong := func(address uint32, value uint32) {
		for i := uint32(0); i < 4; i++ {
			m.mm.Poke(address+i, uint8(value>>(24-8*i)))
		}
	}

	entry := append([]byte("TEXT"), 0, 0, 0, uint8(len(text)))
	entry = append(entry, text...)

	pokeLong(block-8, 0) // nothing before it
	for at, b := range entry {
		m.mm.Poke(block+uint32(at), b)
	}

	pokeLong(handle, block)
	pokeLong(0x0960, uint32(len(entry)))
	pokeLong(0x0964, handle)
	m.mm.Poke(0x0968, 0)
	m.mm.Poke(0x0969, 1) // the count, bumped by the copy
	m.mm.Poke(0x096a, 0)
	m.mm.Poke(0x096b, 1) // the state, in memory
}

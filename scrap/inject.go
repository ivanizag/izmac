package scrap

import "fmt"

/*
Putting text on the scrap of a running machine.

Reading the scrap is a matter of following a handle, but filling it is not:
the block belongs to the Memory Manager, and the only thing that can hand it a
new one of the right size is the machine itself. So the machine is made to do
it. A short program is planted in the memory of the application, the processor
is pointed at it, and when it has run the registers are put back where they
were and the machine carries on with no idea that anything happened.

	LEA     d(A7),A7      ; the stack below the program and its text
	SUBQ.L  #4,A7         ; room for the result
	_ZeroScrap            ; empty the scrap and bump its counter
	ADDQ.L  #4,A7
	SUBQ.L  #4,A7         ; room for the result
	MOVE.L  #length,-(A7) ; PutScrap takes its arguments on the stack,
	MOVE.L  #'TEXT',-(A7) ; in the Pascal order, and takes them off again
	PEA     text(PC)
	_PutScrap
	ADDQ.L  #4,A7
	ILLEGAL               ; never run, it is where the emulator takes over
	text...

Where it runs matters as much as what it does. It is planted on the stack of
the application, below the stack pointer, which is memory nothing else can be
using and which needs no freeing afterwards. And it is started only when the
application is about to ask for its next event, which is the one moment where
calling the Toolbox from outside is no different from the application calling
it itself: the stack is all but empty and nothing is halfway through the heap.

	_GetNextEvent   $a970  what an application of System 6 calls
	_WaitNextEvent  $a860  what an application of System 7 calls

That trap is the whole of the test, and it has to be: the processor is in
supervisor mode here as it is everywhere else on this machine. A Macintosh
sets the supervisor bit as it comes out of reset and never clears it again,
so an application, an interrupt handler and the ROM are all the same to the
status register and it says nothing about where the machine is. What says it
is that the machine is asking for an event, which an interrupt handler does
not do.

The last instruction is never executed. The emulator watches for the program
counter reaching it and restores the registers there, so the ILLEGAL is only
what would happen if it ever failed to.
*/

const (
	trapZeroScrap = 0xa9fc
	trapPutScrap  = 0xa9fe

	trapGetNextEvent  = 0xa970
	trapWaitNextEvent = 0xa860

	// The instructions of the program above, in the order they are emitted
	opLeaFromSP   = 0x4fef // LEA d16(A7),A7
	opSubqLong4SP = 0x598f // SUBQ.L #4,A7
	opAddqLong4SP = 0x588f // ADDQ.L #4,A7
	opMovePushImm = 0x2f3c // MOVE.L #imm,-(A7)
	opPeaFromPC   = 0x487a // PEA d16(PC)
	opIllegal     = 0x4afc

	// codeSize is the length of the program, with the text following it
	codeSize = 34

	/*
		maxTextLength is as much text as will be pasted in one go. It is the
		program and the text together that have to fit on the stack of the
		application, and an application is only promised eight kilobytes of
		stack, so a paste larger than this is refused rather than made to fit
		by taking memory that belongs to something else.
	*/
	maxTextLength = 16 * 1024

	/*
		stackWorkspace is what is left free below the text for the Toolbox
		call itself to use. ZeroScrap and PutScrap take their memory from the
		system heap and only their arguments and locals from the stack, so
		this is a kilobyte where a few dozen bytes would do.
	*/
	stackWorkspace = 1024
)

// Injection is one paste, from the moment it is asked for to the moment the
// machine has taken it
type Injection struct {
	text []byte

	// returnPC is where the program ends, which is how the emulator knows it
	// is over. It is only meaningful once the injection has been planted.
	returnPC uint32
	stackTop uint32
}

// NewInjection prepares the text to be put on the scrap of the machine
func NewInjection(text string) (*Injection, error) {
	data := ToMac(text)
	if len(data) == 0 {
		return nil, fmt.Errorf("there is nothing to paste")
	}
	if len(data) > maxTextLength {
		return nil, fmt.Errorf("%v characters is more than the %v that can be pasted at once",
			len(data), maxTextLength)
	}

	return &Injection{text: data}, nil
}

// Text returns the text as the machine will hold it, which is what the host
// asked to paste after the line endings and any character the Macintosh does
// not have have been dealt with
func (i *Injection) Text() string {
	return FromMac(i.text)
}

/*
Ready tells whether the machine is at a point where the program can be run: an
application about to ask for an event, a Scrap Manager that has started, and
room on the stack for the program and its text.
*/
func (i *Injection) Ready(mem Memory, pc uint32, sp uint32) bool {
	if !isEventTrap(peekWord(mem, pc)) {
		return false
	}

	if !Read(mem).Started() {
		return false
	}

	return i.fits(mem, sp)
}

// fits tells whether the program and its text can be put on the stack of the
// application without reaching the heap growing up towards it
func (i *Injection) fits(mem Memory, sp uint32) bool {
	limit := peekLong(mem, applLimitAddress)
	if limit == 0 || sp <= limit {
		return false
	}

	return sp-limit >= uint32(i.needed())+stackWorkspace
}

// needed is how many bytes the program and its text take, kept even so that
// the stack pointer stays aligned
func (i *Injection) needed() int {
	needed := codeSize + len(i.text)
	if needed%2 != 0 {
		needed++
	}
	return needed
}

/*
Plant writes the program below the stack pointer of the application and
returns the address to start it at. The registers have to have been saved
before this is called, and restored when the program counter reaches
ReturnPC().
*/
func (i *Injection) Plant(mem Memory, sp uint32) uint32 {
	base := (sp - uint32(i.needed())) &^ 1

	code := i.build(base, sp)
	for at, b := range code {
		mem.Poke(base+uint32(at), b)
	}

	i.returnPC = base + codeSize - 2
	i.stackTop = base
	return base
}

// ReturnPC is the address of the last instruction of the program, the one that
// is never run
func (i *Injection) ReturnPC() uint32 {
	return i.returnPC
}

/*
Result is what PutScrap answered, zero when the text was taken. The Pascal
convention leaves the result on the stack under the arguments, so it is the
long the program dropped just before it ended.
*/
func (i *Injection) Result(mem Memory) int32 {
	return int32(peekLong(mem, i.stackTop-4))
}

/*
build assembles the program to run at base with the stack pointer at sp. The
displacements are what makes it worth doing here and not by hand: the one on
the LEA is where the stack has to go so that the pushes of the call do not land
on the text, and the one on the PEA is from the extension word of the
instruction rather than from the instruction itself.
*/
func (i *Injection) build(base uint32, sp uint32) []byte {
	code := make([]byte, 0, codeSize+len(i.text))

	// The stack goes just below the program, so that what the call pushes
	// grows away from the text instead of over it
	code = appendWord(code, opLeaFromSP)
	code = appendWord(code, uint16(int16(int32(base)-int32(sp))))

	// ZeroScrap, which empties the scrap and bumps the count that tells
	// every application that the clipboard has changed
	code = appendWord(code, opSubqLong4SP)
	code = appendWord(code, trapZeroScrap)
	code = appendWord(code, opAddqLong4SP)

	// PutScrap(length, 'TEXT', text)
	code = appendWord(code, opSubqLong4SP)
	code = appendWord(code, opMovePushImm)
	code = appendLong(code, uint32(len(i.text)))
	code = appendWord(code, opMovePushImm)
	code = appendLong(code, typeCode(TypeText))
	code = appendWord(code, opPeaFromPC)
	// The displacement of a PC relative address is counted from the
	// extension word that holds it, which is where the code ends now
	code = appendWord(code, uint16(codeSize-len(code)))
	code = appendWord(code, trapPutScrap)
	code = appendWord(code, opAddqLong4SP)

	code = appendWord(code, opIllegal)

	return append(code, i.text...)
}

// isEventTrap tells whether the word is one of the traps an application calls
// to ask for its next event
func isEventTrap(word uint16) bool {
	return word == trapGetNextEvent || word == trapWaitNextEvent
}

// typeCode turns the four characters of an entry type into the long the
// Toolbox takes
func typeCode(entryType string) uint32 {
	code := uint32(0)
	for i := 0; i < 4; i++ {
		code = code<<8 | uint32(entryType[i])
	}
	return code
}

func appendWord(code []byte, value uint16) []byte {
	return append(code, uint8(value>>8), uint8(value))
}

func appendLong(code []byte, value uint32) []byte {
	return append(code, uint8(value>>24), uint8(value>>16), uint8(value>>8), uint8(value))
}

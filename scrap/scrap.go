/*
Package scrap reaches the clipboard of the emulated Macintosh from outside it.

What the Macintosh calls the clipboard is the desk scrap, and the Scrap
Manager leaves it where anything watching the memory can find it: a record of
five fields in low memory, and a handle to the block holding the data.

	$0960  size    long  how many bytes the scrap holds
	$0964  handle        the block itself
	$0968  count   word  bumped by every ZeroScrap, so a change means a copy
	$096a  state   word  over zero in memory, zero on disk, under zero
	                     before the Scrap Manager has started
	$096c  name    ptr   the name of the file the scrap is unloaded to

The addresses are the same on every System the 128Kb ROM runs, System 7
included. What System 7 changes is when the scrap gets filled and not where it
lives, so nothing here is told which System it is looking at: every decision is
taken from the five fields themselves.

The block is a sequence of entries, each a four character type, a length, and
that many bytes padded to an even boundary:

	'TEXT'  the plain text, in Mac OS Roman with carriage returns
	'styl'  the runs of style that go with it, ignored here
	'PICT'  a picture, which would need a PICT decoder to be of any use

The scrap holds one entry of each type at most, so the text of the clipboard is
the 'TEXT' entry if there is one.
*/
package scrap

// Memory is the address space of the machine, as much of it as this package
// needs. The memory manager of the emulator satisfies it.
type Memory interface {
	Peek(address uint32) uint8
	Poke(address uint32, value uint8)
}

const (
	// The ScrapStuff record, from Inside Macintosh volume I, page I-458
	sizeAddress   = 0x0960
	handleAddress = 0x0964
	countAddress  = 0x0968
	stateAddress  = 0x096a

	/*
		applLimitAddress is how far the application heap is allowed to grow,
		which is also the floor under the stack of the application. It is what
		says whether there is room to run something of ours on that stack.
	*/
	applLimitAddress = 0x0130

	/*
		masterPointerMask takes the flags off a master pointer. A handle
		points at a pointer, and that pointer is not quite an address: the
		Memory Manager of a machine with 24 address lines keeps the locked,
		purgeable and resource flags in the byte that the processor does not
		decode. Every machine the 128Kb ROM runs on has 24 of them, so there
		is no case here where the flags are part of the address.
	*/
	masterPointerMask = 0x00ff_ffff

	// sizeLimit is as large a scrap as this package will read. The scrap of
	// a machine with four megabytes is nowhere near it, and a wild size
	// read while the Scrap Manager is halfway through a change is well past.
	sizeLimit = 1 << 20
)

// TypeText is the entry of the scrap holding plain text
const TypeText = "TEXT"

// Stuff is the ScrapStuff record of the machine
type Stuff struct {
	Size   uint32
	Handle uint32
	Count  uint16

	// State is over zero when the scrap is in memory, zero when it has been
	// unloaded to the Clipboard file, and under zero before the Scrap
	// Manager has started
	State int16
}

// InMemory tells whether the scrap can be read where it is. A scrap that has
// been unloaded to disk is in a file of the boot volume and out of reach.
func (s Stuff) InMemory() bool {
	return s.State > 0 && s.Handle != 0 && s.Size > 0 && s.Size <= sizeLimit
}

// Started tells whether the Scrap Manager has run, which it does early in the
// boot. Nothing should be put on the scrap before it has.
func (s Stuff) Started() bool {
	return s.State >= 0
}

// Read takes the ScrapStuff record of the machine
func Read(mem Memory) Stuff {
	return Stuff{
		Size:   peekLong(mem, sizeAddress),
		Handle: peekLong(mem, handleAddress),
		Count:  peekWord(mem, countAddress),
		State:  int16(peekWord(mem, stateAddress)),
	}
}

// Data returns the scrap as it is in memory, or nil when there is nothing to
// read there
func Data(mem Memory, stuff Stuff) []byte {
	if !stuff.InMemory() {
		return nil
	}

	address := peekLong(mem, stuff.Handle) & masterPointerMask
	if address == 0 {
		return nil
	}

	data := make([]byte, stuff.Size)
	for i := range data {
		data[i] = mem.Peek(address + uint32(i))
	}
	return data
}

// Text returns the text on the scrap of the machine, and whether there was any
func Text(mem Memory) (string, bool) {
	data := Data(mem, Read(mem))
	if data == nil {
		return "", false
	}

	entry, found := Entry(data, TypeText)
	if !found {
		return "", false
	}

	return FromMac(entry), true
}

func peekWord(mem Memory, address uint32) uint16 {
	return uint16(mem.Peek(address))<<8 | uint16(mem.Peek(address+1))
}

func peekLong(mem Memory, address uint32) uint32 {
	return uint32(peekWord(mem, address))<<16 | uint32(peekWord(mem, address+2))
}

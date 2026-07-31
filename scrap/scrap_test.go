package scrap

import "testing"

// testMemory is as much of an address space as these tests need, with nothing
// in it until something is put there
type testMemory map[uint32]uint8

func (m testMemory) Peek(address uint32) uint8 {
	return m[address]
}

func (m testMemory) Poke(address uint32, value uint8) {
	m[address] = value
}

func (m testMemory) pokeWord(address uint32, value uint16) {
	m.Poke(address, uint8(value>>8))
	m.Poke(address+1, uint8(value))
}

func (m testMemory) pokeLong(address uint32, value uint32) {
	m.pokeWord(address, uint16(value>>16))
	m.pokeWord(address+2, uint16(value))
}

func (m testMemory) pokeBytes(address uint32, data []byte) {
	for at, b := range data {
		m.Poke(address+uint32(at), b)
	}
}

const (
	// The handle and the block it leads to, anywhere in the RAM of a machine
	testHandle = 0x0001_2000
	testBlock  = 0x0003_4000

	/*
		testMasterFlags is what the Memory Manager keeps in the top byte of a
		master pointer, a locked and purgeable block here. It is in these
		tests because a scrap read without taking it off is read from an
		address eight megabytes away from the one that was meant.
	*/
	testMasterFlags = 0xc0 << 24
)

// putScrap sets the machine up as if the given block were on its scrap
func putScrap(mem testMemory, block []byte) {
	mem.pokeLong(sizeAddress, uint32(len(block)))
	mem.pokeLong(handleAddress, testHandle)
	mem.pokeWord(countAddress, 1)
	mem.pokeWord(stateAddress, 1)

	mem.pokeLong(testHandle, testBlock|testMasterFlags)
	mem.pokeBytes(testBlock, block)
}

// buildBlock makes a scrap block of the entries given, in order
func buildBlock(entries ...string) []byte {
	block := make([]byte, 0, 64)

	for i := 0; i < len(entries); i += 2 {
		contents := []byte(entries[i+1])

		block = append(block, entries[i]...)
		block = append(block,
			uint8(len(contents)>>24), uint8(len(contents)>>16),
			uint8(len(contents)>>8), uint8(len(contents)))
		block = append(block, contents...)

		if len(contents)%2 != 0 {
			block = append(block, 0)
		}
	}

	return block
}

/*
The text of the clipboard, followed from the low memory globals to the block
and back out as a Go string. The flags on the master pointer and the carriage
returns of the Macintosh are both on the way.
*/
func TestTheTextOfTheScrapIsRead(t *testing.T) {
	mem := testMemory{}
	putScrap(mem, buildBlock(
		"styl", "\x00\x01ignored",
		"TEXT", "one\rtwo"))

	text, found := Text(mem)
	if !found {
		t.Fatal("the text of the scrap was not found")
	}
	if text != "one\ntwo" {
		t.Errorf("the scrap read as %q, wanted %q", text, "one\ntwo")
	}
}

// A scrap the Scrap Manager has written to the Clipboard file is not in memory
// to be read, and reading whatever the handle used to point at would hand the
// host something that is no longer the clipboard
func TestAScrapOnDiskIsNotRead(t *testing.T) {
	mem := testMemory{}
	putScrap(mem, buildBlock("TEXT", "on the disk"))
	mem.pokeWord(stateAddress, 0)

	if _, found := Text(mem); found {
		t.Error("a scrap that had been unloaded to disk was read anyway")
	}
}

// The Scrap Manager has not started before the machine has booted, and what is
// at those addresses until then is not a scrap
func TestAScrapIsNotReadBeforeTheManagerStarts(t *testing.T) {
	mem := testMemory{}
	putScrap(mem, buildBlock("TEXT", "too early"))
	mem.pokeWord(stateAddress, 0xffff)

	if Read(mem).Started() {
		t.Error("the Scrap Manager is reported as started with the state under zero")
	}
	if _, found := Text(mem); found {
		t.Error("a scrap was read before the Scrap Manager started")
	}
}

// A picture on the clipboard is a clipboard with no text on it, which is not
// the same as an empty one and is equally not something to send to the host
func TestAScrapWithNoTextIsNotRead(t *testing.T) {
	mem := testMemory{}
	putScrap(mem, buildBlock("PICT", "not a picture, but not text either"))

	if _, found := Text(mem); found {
		t.Error("a scrap holding only a picture answered with text")
	}
}

// A handle that leads nowhere is what is there before anything has been
// copied, and following it would read the vectors at the bottom of the memory
func TestAnEmptyScrapIsNotRead(t *testing.T) {
	mem := testMemory{}
	putScrap(mem, buildBlock("TEXT", "gone"))
	mem.pokeLong(testHandle, 0)

	if _, found := Text(mem); found {
		t.Error("a handle pointing at nothing was read as a scrap")
	}
}

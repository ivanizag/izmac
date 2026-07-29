package izmac

import "testing"

// newTestMemoryManager returns a memory manager with a ROM whose bytes are
// recognizable: the byte at the ROM offset i is i, wrapped
func newTestMemoryManager(ramSizeKb int) *memoryManager {
	rom := make([]uint8, romSize)
	for i := range rom {
		rom[i] = uint8(i)
	}
	return newMemoryManager(ramSizeKb, rom)
}

func TestOverlayPutsTheRomOnTheResetVectors(t *testing.T) {
	m := newTestMemoryManager(1024)

	// The processor takes the reset stack pointer and program counter from
	// the first eight bytes, they have to come from the ROM
	for address := uint32(0); address < 8; address++ {
		if m.Peek(address) != uint8(address) {
			t.Fatalf("with the overlay set, $%06x is not the ROM", address)
		}
	}

	m.setOverlay(false)
	for address := uint32(0); address < 8; address++ {
		if m.Peek(address) != 0 {
			t.Fatalf("with the overlay clear, $%06x is not the RAM", address)
		}
	}
}

func TestOverlayMovesTheRam(t *testing.T) {
	m := newTestMemoryManager(1024)

	// With the overlay set the RAM answers at $600000
	m.Poke(overlayRAMBase+0x1234, 0x42)
	if m.Peek(overlayRAMBase+0x1234) != 0x42 {
		t.Error("the RAM is not reachable at $600000 while the overlay is set")
	}

	// And it is the same RAM that shows at zero once the overlay is cleared
	m.setOverlay(false)
	if m.Peek(0x1234) != 0x42 {
		t.Error("the RAM moved at $600000 is not the RAM at zero")
	}
}

func TestRomIsNotWritable(t *testing.T) {
	m := newTestMemoryManager(1024)
	m.setOverlay(false)

	before := m.Peek(romBase + 0x100)
	m.Poke(romBase+0x100, before^0xff)
	if m.Peek(romBase+0x100) != before {
		t.Error("the ROM was modified by a write")
	}
}

func TestDevicesAreMirroredOverTheirRegion(t *testing.T) {
	m := newTestMemoryManager(1024)
	m.setOverlay(false)

	// The 128Kb of ROM repeat inside the 256Kb the sockets can hold, and
	// no further
	value := m.Peek(romBase + 0x40)
	if m.Peek(romBase+romSize+0x40) != value {
		t.Error("the ROM does not repeat inside its window")
	}
	if m.Peek(romWindowEnd+0x40) == value {
		t.Error("the ROM answers past the end of its window")
	}

	// And the 1Mb of RAM repeats over the RAM region
	m.Poke(0x1000, 0x55)
	if m.Peek(0x101000) != 0x55 {
		t.Error("the RAM does not mirror over its region")
	}
}

func TestQuartersAreDecoded(t *testing.T) {
	m := newTestMemoryManager(1024)
	m.setOverlay(false)

	probe := &probeDevice{}
	m.scsi = probe
	m.scc = probe
	m.iwm = probe
	m.via = probe

	for _, c := range []struct {
		name    string
		address uint32
	}{
		{"SCSI", scsiBase},
		{"SCC read", 0x800000},
		{"SCC write", 0xa00000},
		{"IWM", iwmBase},
		{"VIA", 0xefe1fe},
	} {
		probe.reads = 0
		m.Peek(c.address)
		if probe.reads != 1 {
			t.Errorf("%v at $%06x was not reached", c.name, c.address)
		}
	}

	// The RAM and the ROM must not reach any device
	probe.reads = 0
	m.Peek(0x1000)
	m.Peek(romBase)
	if probe.reads != 0 {
		t.Error("an access to the RAM or the ROM reached a device")
	}
}

func TestAddressesAreMaskedTo24Bits(t *testing.T) {
	m := newTestMemoryManager(1024)
	m.setOverlay(false)

	// The Memory Manager keeps flags on the high byte of the master
	// pointers, so the high byte has to be dropped
	m.Poke(0x001234, 0x77)
	if m.Peek(0xff001234) != 0x77 {
		t.Error("the high byte of the address was not ignored")
	}
}

func TestRamTop(t *testing.T) {
	for _, c := range []struct {
		ramSizeKb int
		videoMain uint32
	}{
		{1024, 0x0fa700},
		{4096, 0x3fa700},
	} {
		m := newTestMemoryManager(c.ramSizeKb)
		got := m.ramTop() - videoMainOffset
		if got != c.videoMain {
			t.Errorf("%vKb: the main video buffer is at $%06x, wanted $%06x",
				c.ramSizeKb, got, c.videoMain)
		}
	}
}

// probeDevice counts the accesses that reach it
type probeDevice struct {
	reads  int
	writes int
}

func (d *probeDevice) peek(address uint32) uint8 {
	d.reads++
	return 0
}

func (d *probeDevice) poke(address uint32, value uint8) {
	d.writes++
}

/*
The ROM works out whether the machine has SCSI by comparing a long at $420000
with one at $440000. The first is the second copy of the ROM inside the 256Kb
its sockets can hold and the second is past the end of it, so on a Plus they
differ. Mirroring the ROM over the whole quarter makes them equal, and then
the ROM decides there is no SCSI, skips the bus scan and never finds a disk.
*/
func TestTheRomCanTellThatTheMachineHasScsi(t *testing.T) {
	m := newTestMemoryManager(1024)
	m.setOverlay(false)

	longAt := func(address uint32) uint32 {
		return uint32(m.Peek(address))<<24 | uint32(m.Peek(address+1))<<16 |
			uint32(m.Peek(address+2))<<8 | uint32(m.Peek(address+3))
	}

	inside := longAt(0x420000)
	outside := longAt(0x440000)

	if inside == outside {
		t.Errorf("$420000 and $440000 both read $%08x, so the ROM sees no SCSI", inside)
	}
	if inside != longAt(romBase) {
		t.Error("$420000 is not the second copy of the ROM")
	}
}

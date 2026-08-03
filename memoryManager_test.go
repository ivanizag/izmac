package izmac

import (
	"testing"

	"github.com/ivanizag/izmac/component"
	"github.com/ivanizag/izmac/storage"
)

// newTestMemoryManager returns a memory manager with a ROM whose bytes are
// recognizable: the byte at the ROM offset i is i, wrapped
func newTestMemoryManager(ramSizeKb int) *memoryManager {
	rom := make([]uint8, storage.RomSize)
	for i := range rom {
		rom[i] = uint8(i)
	}
	return newMemoryManager(ramSizeKb, rom, false)
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
	if m.Peek(romBase+storage.RomSize+0x40) != value {
		t.Error("the ROM does not repeat inside its window")
	}
	if m.Peek(romWindowEnd+0x40) == value {
		t.Error("the ROM answers past the end of its window")
	}

	// And the 1Mb of RAM repeats over the RAM region
	m.Poke(0x1000, 0x55)
	if m.Peek(0x10_1000) != 0x55 {
		t.Error("the RAM does not mirror over its region")
	}
}

/*
Every chip is reached on the quarter it belongs to. Each is checked against
state only that chip keeps: the SCSI mode register and the VIA data direction
register read back what was written to them, and the IWM latches its enable
line from the address of the access alone.
*/
func TestQuartersAreDecoded(t *testing.T) {
	v, m, _ := newTestVia(t)
	m.via = v
	m.setOverlay(false)

	// The NCR 5380 on the ROM quarter, its registers 16 bytes apart. The
	// second of them is the mode register.
	const scsiModeRegister = scsiBase + 2*0x10
	m.Poke(scsiModeRegister, 0x55)
	if got := m.Peek(scsiModeRegister); got != 0x55 {
		t.Errorf("the SCSI mode register at $%06x reads $%02x, wanted $55",
			scsiModeRegister, got)
	}

	// The IWM on the low half of the last quarter
	m.Peek(iwmAddress(iwmSwEnblH))
	if !m.iwm.enable {
		t.Errorf("the IWM at $%06x was not reached", iwmAddress(iwmSwEnblH))
	}

	// And the VIA on the high half of it
	m.Poke(viaAddress(viaRegDdrA), 0x55)
	if got := m.Peek(viaAddress(viaRegDdrA)); got != 0x55 {
		t.Errorf("the VIA data direction register at $%06x reads $%02x, wanted $55",
			viaAddress(viaRegDdrA), got)
	}
}

/*
The serial controller is reached on its own quarter, on the read side and on
the write side, and it is not behind the device interface so it is checked by
writing a register and reading it back. The register 15 is the one to use:
the low registers report the state of the chip rather than what was written
to them, and 15 is the one the ROM itself reads back. It is reached with the
point high command, since the pointer is only three bits wide.
*/
func TestTheSerialQuarterIsDecoded(t *testing.T) {
	m := newTestMemoryManager(1024)
	m.setOverlay(false)

	const pointAt15 = 0x0f

	for _, base := range []uint32{0x80_0000, 0xa0_0000} {
		m.Poke(base, pointAt15)
		m.Poke(base, 0x5a)

		m.Poke(base, pointAt15)
		if got := m.Peek(base); got != 0x5a {
			t.Errorf("$%06x did not reach the serial controller, it read $%02x",
				base, got)
		}
	}
}

func TestAddressesAreMaskedTo24Bits(t *testing.T) {
	m := newTestMemoryManager(1024)
	m.setOverlay(false)

	// The Memory Manager keeps flags on the high byte of the master
	// pointers, so the high byte has to be dropped
	m.Poke(0x00_1234, 0x77)
	if m.Peek(0xff00_1234) != 0x77 {
		t.Error("the high byte of the address was not ignored")
	}
}

func TestRamTop(t *testing.T) {
	for _, c := range []struct {
		ramSizeKb int
		videoMain uint32
	}{
		{1024, 0x0f_a700},
		{4096, 0x3f_a700},
	} {
		m := newTestMemoryManager(c.ramSizeKb)
		got := m.ramTop() - videoMainOffset
		if got != c.videoMain {
			t.Errorf("%vKb: the main video buffer is at $%06x, wanted $%06x",
				c.ramSizeKb, got, c.videoMain)
		}
	}
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

	inside := longAt(0x42_0000)
	outside := longAt(0x44_0000)

	if inside == outside {
		t.Errorf("$420000 and $440000 both read $%08x, so the ROM sees no SCSI", inside)
	}
	if inside != longAt(romBase) {
		t.Error("$420000 is not the second copy of the ROM")
	}
}

const sccReadBase = 0x9f_fff8

func sccAddress(offset uint32) uint32 {
	return sccReadBase + offset
}

/*
The four ports of the serial controller land on the right channel and side.
The chip has no idea where it sits, so this is the map's business.
*/
func TestTheFourPortsAreDecoded(t *testing.T) {
	for _, c := range []struct {
		offset  uint32
		channel int
		control bool
	}{
		{sccOffsetBControl, component.ChannelB, true},
		{sccOffsetAControl, component.ChannelA, true},
		{sccOffsetBData, component.ChannelB, false},
		{sccOffsetAData, component.ChannelA, false},
	} {
		channel, control := sccPort(sccAddress(c.offset))
		if channel != c.channel || control != c.control {
			t.Errorf("the offset %v reached the channel %v control %v, wanted %v and %v",
				c.offset, channel, control, c.channel, c.control)
		}
	}
}

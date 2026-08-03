package izmac

import (
	"github.com/ivanizag/izmac/component"
	"github.com/ivanizag/izmac/scsi"
)

/*
The address decoding of the Macintosh Plus. The 16Mb the processor can reach
is split in four quarters: RAM, ROM and SCSI, the SCC, and the IWM and the
VIA. Inside each quarter only a few address lines are decoded, so every device
is mirrored many times over its region. The ROM relies on that, decode
loosely.

The overlay, on the bit 4 of the VIA port A, is set at reset. It puts the ROM
over the first quarter, which is where the processor takes the reset stack
pointer and program counter from, and moves the RAM to $600000. The boot code
jumps to the ROM on its normal position and clears the bit.
*/

const (
	// addressMask is applied to every access, the 68000 has 24 address lines
	addressMask uint32 = 0x00ff_ffff

	quarterRAM  = 0 // $000000-$3fffff
	quarterROM  = 1 // $400000-$7fffff, with the SCSI
	quarterSCC  = 2 // $800000-$bfffff
	quarterMisc = 3 // $c00000-$ffffff, the IWM and the VIA

	// romWindowEnd is where the ROM stops answering. The sockets of the
	// Plus take up to 256Kb, so the decoding covers that much and the
	// 128Kb fitted repeats twice inside it. Past it nothing drives the bus.
	//
	// This is not a detail: the ROM finds out whether the machine has SCSI
	// by comparing a long at $420000, which is the second copy of itself,
	// with one at $440000, which is outside the window. Mirroring the ROM
	// over the whole quarter makes those equal and the ROM decides there is
	// no SCSI, skips the bus scan at $407d40 and never looks for a disk.
	romWindowEnd = 0x44_0000

	scsiBase = 0x58_0000
	scsiEnd  = 0x60_0000

	// unmappedValue is what an address nothing answers at reads as
	unmappedValue = 0xff

	overlayRAMBase = 0x60_0000

	iwmBase = 0xc0_0000
	viaBase = 0xe0_0000
)

// memoryManager decodes the address space. It implements iz68000.Memory.
type memoryManager struct {
	ram []uint8
	rom []uint8

	ramMask uint32
	romMask uint32

	// overlay is the reset time address map, the ROM over the address zero
	overlay bool

	/*
		The chips on the map. Each is at one place and there is never a
		second one of its kind, so they are held as what they are.

			scsi     $580000
			scc      $9ffff8   read    bCtl +0  aCtl +2  bData +4  aData +6
			         $bffff9   write
			iwm      $c00000
			via      $e00000

		The via is the one the machine has to fill in later: it is built
		around this manager, so it can not exist before it does.
	*/
	scsi *scsi.Bus
	scc  *component.SCC8530
	iwm  *iwm
	via  *via

	// A watch on a range of the RAM, to find what writes a low memory
	// global. Nil unless a frontend asked for it.
	watchFrom    uint32
	watchTo      uint32
	watchHandler func(address uint32, value uint8)
}

// setWatch reports every write to a range of the address space
func (m *memoryManager) setWatch(from uint32, to uint32, handler func(address uint32, value uint8)) {
	m.watchFrom = from
	m.watchTo = to
	m.watchHandler = handler
}

// notifyWatch reports a write when it falls inside the range watched
func (m *memoryManager) notifyWatch(address uint32, value uint8) {
	if m.watchHandler == nil {
		return
	}
	if address >= m.watchFrom && address <= m.watchTo {
		m.watchHandler(address, value)
	}
}

func newMemoryManager(ramSizeKb int, romData []uint8, floppyTrace bool) *memoryManager {
	ramSize := uint32(ramSizeKb) * 1024
	m := &memoryManager{
		ram:     make([]uint8, ramSize),
		rom:     romData,
		ramMask: ramSize - 1,
		romMask: uint32(len(romData)) - 1,
		overlay: true,
	}

	m.scsi = scsi.NewBus()
	m.scc = component.NewSCC8530()
	m.iwm = newIwm(floppyTrace)

	return m
}

// ramTop is the address just past the last byte of RAM. The video and sound
// buffers hang from it.
func (m *memoryManager) ramTop() uint32 {
	return uint32(len(m.ram))
}

// setOverlay switches between the reset time and the normal address maps
func (m *memoryManager) setOverlay(overlay bool) {
	m.overlay = overlay
}

// Peek returns the byte at the given address
func (m *memoryManager) Peek(address uint32) uint8 {
	address &= addressMask

	switch address >> 22 {
	case quarterRAM:
		if m.overlay {
			return m.rom[address&m.romMask]
		}
		return m.ram[address&m.ramMask]

	case quarterROM:
		if m.overlay && address >= overlayRAMBase {
			return m.ram[address&m.ramMask]
		}
		if address < romWindowEnd {
			return m.rom[address&m.romMask]
		}
		if address >= scsiBase && address < scsiEnd {
			return m.scsi.Peek(address)
		}
		return unmappedValue

	case quarterSCC:
		return m.scc.Read(sccPort(address))

	default: // quarterMisc
		if address < viaBase {
			return m.iwm.peek(address)
		}
		return m.via.peek(address)
	}
}

/*
PeekCode returns the byte at the given address. iz68000 offers it as a place
to take advantage of instructions being fetched from a narrower part of the
map than data is, but there is nothing here to take: the decoding is a switch
on two bits and the same branches either way, and a specialised copy of the
RAM and ROM cases measured no faster over a boot than this does. What it did
do was make the map exist in two places, which is how the window the ROM
answers over came to be fixed in one of them and not the other.
*/
func (m *memoryManager) PeekCode(address uint32) uint8 {
	return m.Peek(address)
}

// Poke stores a byte at the given address. Writes to the ROM are ignored, the
// power on tests do them.
func (m *memoryManager) Poke(address uint32, value uint8) {
	address &= addressMask

	switch address >> 22 {
	case quarterRAM:
		if !m.overlay {
			m.ram[address&m.ramMask] = value
			m.notifyWatch(address&m.ramMask, value)
		}

	case quarterROM:
		if m.overlay && address >= overlayRAMBase {
			m.ram[address&m.ramMask] = value
			m.notifyWatch(address&m.ramMask, value)
		} else if address >= scsiBase && address < scsiEnd {
			m.scsi.Poke(address, value)
		}

	case quarterSCC:
		channel, control := sccPort(address)
		m.scc.Write(channel, control, value)

	default: // quarterMisc
		if address < viaBase {
			m.iwm.poke(address, value)
		} else {
			m.via.poke(address, value)
		}
	}
}

/*
peekLong and pokeLong read and write a long, the four bytes of an address in
the order the machine holds them.

They are methods of the manager and are deliberately not part of the Memory
interface iz68000 takes. That one stays byte granular: the processor composes
its own words and longs and raises the address error on an odd one while it
does, and moving that in here would take the address errors away, which the
ROM and MacsBug both depend on. Nothing above the processor is bound by that,
and the low memory globals are read and written a long at a time.
*/
func (m *memoryManager) peekLong(address uint32) uint32 {
	value := uint32(0)
	for i := uint32(0); i < 4; i++ {
		value = value<<8 | uint32(m.Peek(address+i))
	}
	return value
}

func (m *memoryManager) pokeLong(address uint32, value uint32) {
	for i := uint32(0); i < 4; i++ {
		m.Poke(address+i, uint8(value>>(24-8*i)))
	}
}

const (
	// The offsets from the base of each of the two serial ports
	sccOffsetBControl = 0
	sccOffsetAControl = 2
	sccOffsetBData    = 4
	sccOffsetAData    = 6
)

// sccPort works out which channel of the serial controller an address reaches
// and whether it is the control or the data side of it
func sccPort(address uint32) (channel int, control bool) {
	switch address & 0x06 {
	case sccOffsetBControl:
		return component.ChannelB, true
	case sccOffsetAControl:
		return component.ChannelA, true
	case sccOffsetBData:
		return component.ChannelB, false
	default:
		return component.ChannelA, false
	}
}

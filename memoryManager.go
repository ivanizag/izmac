package izmac

import "github.com/ivanizag/izmac/scsi"

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

// device is anything mapped on the address space other than RAM and ROM
type device interface {
	peek(address uint32) uint8
	poke(address uint32, value uint8)
}

// memoryManager decodes the address space. It implements iz68000.Memory.
type memoryManager struct {
	ram []uint8
	rom []uint8

	ramMask uint32
	romMask uint32

	// overlay is the reset time address map, the ROM over the address zero
	overlay bool

	scsi device
	scc  device
	iwm  device
	via  device

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

func newMemoryManager(ramSizeKb int, romData []uint8) *memoryManager {
	ramSize := uint32(ramSizeKb) * 1024
	m := &memoryManager{
		ram:     make([]uint8, ramSize),
		rom:     romData,
		ramMask: ramSize - 1,
		romMask: uint32(len(romData)) - 1,
		overlay: true,
	}

	null := &nullDevice{}
	m.scsi = null
	m.scc = null
	m.iwm = null
	m.via = null

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
			return m.scsi.peek(address)
		}
		return unmappedValue

	case quarterSCC:
		return m.scc.peek(address)

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
			m.scsi.poke(address, value)
		}

	case quarterSCC:
		m.scc.poke(address, value)

	default: // quarterMisc
		if address < viaBase {
			m.iwm.poke(address, value)
		} else {
			m.via.poke(address, value)
		}
	}
}

/*
scsiBus puts the bus on the address space. The device interface is unexported
so that the map of the machine stays private, and the bus is its own package,
so the two are joined here rather than by exporting one to suit the other.
*/
type scsiBus struct {
	bus *scsi.Bus
}

func (d *scsiBus) peek(address uint32) uint8        { return d.bus.Peek(address) }
func (d *scsiBus) poke(address uint32, value uint8) { d.bus.Poke(address, value) }

// nullDevice stands for the devices not implemented yet. Reads return $ff as
// an undriven bus does.
type nullDevice struct{}

func (d *nullDevice) peek(address uint32) uint8        { return 0xff }
func (d *nullDevice) poke(address uint32, value uint8) {}

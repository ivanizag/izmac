package izmac

import "fmt"

/*
The power on tests of the ROM report their failures on the screen as a Sad
Mac with a six digit code below it, YYXXXX, the class and the subcode. The
same values are left on the processor registers: the low word of D7 is the
class, its high word holds flags Apple used internally, and D6 is the
subcode.

Reading them from the registers is what lets the failures be reported before
there is any video, which is the whole point of the first milestone.
*/

const (
	sadMacClassRomChecksum   = 0x01
	sadMacClassRamBus        = 0x02
	sadMacClassRamByteWrite  = 0x03
	sadMacClassRamModulo     = 0x04
	sadMacClassRamAddressing = 0x05
	sadMacClassException     = 0x0f
)

// sadMacClasses returns the tests the ROM reports on. The classes 2 to 5 are
// the four memory tests and their subcode is a bit mask of the failing chips.
func sadMacClasses() map[uint16]string {
	return map[uint16]string{
		sadMacClassRomChecksum:   "ROM checksum",
		sadMacClassRamBus:        "RAM bus subtest",
		sadMacClassRamByteWrite:  "RAM byte write test",
		sadMacClassRamModulo:     "RAM modulo 3 pattern test",
		sadMacClassRamAddressing: "RAM address uniqueness test",
		sadMacClassException:     "exception before the system was loaded",
	}
}

/*
The subcodes of the exception class count from the bus error, which is the
vector 2, so the subcode is the vector number less one. Values past the line
1111 emulator are not exceptions and are used by the ROM for other reports.
*/
func sadMacExceptions() map[uint32]string {
	return map[uint32]string{
		1:  "bus error",
		2:  "address error",
		3:  "illegal instruction",
		4:  "divide by zero",
		5:  "CHK instruction",
		6:  "TRAPV instruction",
		7:  "privilege violation",
		8:  "trace",
		9:  "line 1010 emulator",
		10: "line 1111 emulator",
	}
}

// sadMac is what the power on tests left on the registers
type sadMac struct {
	class   uint16
	flags   uint16
	subcode uint32
}

func newSadMac(d7 uint32, d6 uint32) sadMac {
	return sadMac{
		class:   uint16(d7),
		flags:   uint16(d7 >> 16),
		subcode: d6,
	}
}

// isRamTest tells if the subcode is a bit mask of the failing RAM chips
func (s sadMac) isRamTest() bool {
	return s.class >= sadMacClassRamBus && s.class <= sadMacClassRamAddressing
}

// String describes the failure the way the screen would show it
func (s sadMac) String() string {
	name, known := sadMacClasses()[s.class]
	if !known {
		name = "unknown test"
	}

	description := fmt.Sprintf("Sad Mac %02X%04X: %v", s.class, uint16(s.subcode), name)

	switch {
	case s.class == sadMacClassException:
		exception, known := sadMacExceptions()[s.subcode]
		if known {
			description += fmt.Sprintf(", %v", exception)
		} else {
			description += fmt.Sprintf(", subcode %v is not an exception", s.subcode)
		}

	case s.isRamTest():
		description += fmt.Sprintf(", failing chips %v", ramChipMask(s.subcode))
	}

	if s.flags != 0 {
		description += fmt.Sprintf(" (flags $%04x)", s.flags)
	}

	return description
}

// ramChipMask renders the bit mask of the memory tests as the list of the
// bits that failed
func ramChipMask(mask uint32) string {
	if mask == 0 {
		return "none"
	}

	list := ""
	for bit := 0; bit < 32; bit++ {
		if mask&(1<<bit) != 0 {
			if list != "" {
				list += ", "
			}
			list += fmt.Sprintf("%v", bit)
		}
	}
	return list
}

/*
haltDetector spots the processor stopping on a tight loop, which is where the
ROM ends after drawing a Sad Mac. Watching for the loop rather than for a
known address keeps this working across the three ROM revisions and needs no
symbols.

Two things have to be true at once and neither is enough alone.

Staying inside a narrow range of addresses is not enough: the ROM checksum
runs a tight loop over the whole 128Kb and each memory test runs one over the
whole RAM. What tells those apart from a halt is that they make progress, so
the registers have to stand still as well.

Standing still is not enough either, and this one is subtler. The ROM waits
for the tick counter with

	CMP.L  ($16a).W,D0
	BEQ    *

which changes no register and stays on two addresses for as long as a whole
frame, until the vertical blanking interrupt moves the counter. Measuring the
wait in instructions declares that halted almost immediately, which is a lie
that costs a lot of time to see through. So the wait is measured in cycles
and has to outlast several frames, which no legitimate wait for an interrupt
does and which a real halt never ends.
*/
type haltDetector struct {
	// window is the addresses the loop is allowed to span
	window uint32
	// cyclesNeeded is how long nothing may change before the machine is
	// called halted
	cyclesNeeded uint64

	lowest      uint32
	highest     uint32
	fingerprint uint32
	startCycles uint64
	watching    bool
	halted      bool
}

func newHaltDetector(window uint32, cyclesNeeded uint64) *haltDetector {
	return &haltDetector{window: window, cyclesNeeded: cyclesNeeded}
}

// inspect takes the program counter, a fingerprint of the registers and the
// cycle count before each instruction, and returns true once the processor
// has looked stuck for long enough
func (h *haltDetector) inspect(pc uint32, fingerprint uint32, cycles uint64) bool {
	if !h.watching || fingerprint != h.fingerprint {
		// Something got done, start watching again from here
		h.restart(pc, fingerprint, cycles)
		return false
	}

	if pc < h.lowest {
		h.lowest = pc
	}
	if pc > h.highest {
		h.highest = pc
	}
	if h.highest-h.lowest > h.window {
		h.restart(pc, fingerprint, cycles)
		return false
	}

	h.halted = cycles-h.startCycles >= h.cyclesNeeded
	return h.halted
}

func (h *haltDetector) restart(pc uint32, fingerprint uint32, cycles uint64) {
	h.lowest, h.highest = pc, pc
	h.fingerprint = fingerprint
	h.startCycles = cycles
	h.watching = true
	h.halted = false
}

func (h *haltDetector) reset() {
	h.watching = false
	h.halted = false
}

// registerFingerprint summarizes the registers so that a loop getting
// something done can be told from one that is not
func (m *Mac) registerFingerprint() uint32 {
	var f uint32
	for i := 0; i < 8; i++ {
		f = f*31 + m.cpu.GetD(i)
		f = f*31 + m.cpu.GetA(i)
	}
	return f
}

// GetSadMac returns what the power on tests reported, and whether the
// machine has settled on the loop that follows a failure
func (m *Mac) GetSadMac() (string, bool) {
	s := newSadMac(m.cpu.GetD(7), m.cpu.GetD(6))
	return s.String(), m.halt.halted
}

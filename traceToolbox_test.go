package izmac

import (
	"strings"
	"testing"
)

func TestTheLineATrapsAreRecognized(t *testing.T) {
	for _, c := range []struct {
		word    uint16
		isTrap  bool
		comment string
	}{
		{0xa000, true, "the first OS trap"},
		{0xa9c9, true, "a Toolbox trap"},
		{0xafff, true, "the last of the line"},
		{0x4e71, false, "a NOP"},
		{0x9fff, false, "just below the line"},
		{0xb000, false, "just above the line"},
	} {
		if isLineA(c.word) != c.isTrap {
			t.Errorf("$%04x, %v, was not classified correctly", c.word, c.comment)
		}
	}
}

func TestTheTrapKindComesFromTheBit11(t *testing.T) {
	// $a019 is InitZone, an operating system trap
	if name := trapName(0xa019); name != "InitZone" {
		t.Errorf("$a019 named %q, wanted InitZone", name)
	}

	// $a86e is InitGraf, a Toolbox one
	if name := trapName(0xa86e); name != "InitGraf" {
		t.Errorf("$a86e named %q, wanted InitGraf", name)
	}
}

func TestAnUnknownTrapIsReportedByNumber(t *testing.T) {
	// Not every number is a trap, and what is not still has to trace
	name := trapName(0xa0ff)
	if !strings.Contains(name, "OS trap") || !strings.Contains(name, "255") {
		t.Errorf("an unknown OS trap named %q", name)
	}

	name = trapName(0xabff)
	if !strings.Contains(name, "Toolbox trap") || !strings.Contains(name, "1023") {
		t.Errorf("an unknown Toolbox trap named %q", name)
	}
}

func TestTheAutoPopBitDoesNotChangeTheTrapNumber(t *testing.T) {
	// The bit 10 is a flag, not part of the number
	if trapName(0xa86e) != trapName(0xac6e) {
		t.Error("the bit 10 changed the trap identified")
	}
}

// The tables come from the trap macros of the ROM disassembly and cover the
// whole set, not the handful the boot happens to use
func TestTheTrapTablesAreComplete(t *testing.T) {
	if got := len(osTrapNames()); got < 60 {
		t.Errorf("the operating system table holds %v traps, wanted the whole set", got)
	}
	if got := len(toolboxTrapNames()); got < 200 {
		t.Errorf("the Toolbox table holds %v traps, wanted the whole set", got)
	}
}

func TestTheTrapsSeenDuringABootAreNamed(t *testing.T) {
	for word, wanted := range map[uint16]string{
		0xa002: "Read",
		0xa019: "InitZone",
		0xa02e: "BlockMove",
		0xa06c: "InitFS",
		0xa86e: "InitGraf",
		0xa8fe: "InitFonts",
		0xa913: "NewWindow",
		0xa970: "GetNextEvent",
		0xa9c8: "SysBeep",
		0xa9c9: "SysError",
		0xa9ff: "Debugger",
		0xa815: "SCSIDispatch",
	} {
		if got := trapName(word); got != wanted {
			t.Errorf("$%04x named %q, wanted %q", word, got, wanted)
		}
	}
}

/*
An operating system trap carries flags on its bits 8 to 10 as well as the
number on the low eight, so the variants of a call all name the same trap.
The raw word is printed beside the name, which is where the flags are read.
*/
func TestTheFlagVariantsNameTheSameTrap(t *testing.T) {
	for _, word := range []uint16{0xa022, 0xa122, 0xa222, 0xa322} {
		if got := trapName(word); got != "NewHandle" {
			t.Errorf("$%04x named %q, wanted NewHandle", word, got)
		}
	}
}

func TestNoTrapIsNamedTwice(t *testing.T) {
	for _, table := range []map[uint16]string{osTrapNames(), toolboxTrapNames()} {
		seen := make(map[string]uint16)
		for number, name := range table {
			if other, clash := seen[name]; clash {
				t.Errorf("%v is both $%03x and $%03x", name, other, number)
			}
			seen[name] = number
		}
	}
}

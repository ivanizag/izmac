package izmac

import (
	"strings"
	"testing"

	"github.com/ivanizag/izmac/storage"
)

/*
The path the real ROM takes out of the reset: it starts on the overlay map
with the ROM at the address zero, jumps to the ROM on its normal address,
and only then clears the overlay through the VIA. Doing it in that order
matters, clearing the overlay first would pull the code out from under the
program counter.

This builds a small ROM that follows the same steps and ends on the loop the
ROM settles into after a failure, with a code on D7 and D6.
*/
func buildOverlayTestRom() []uint8 {
	data := make([]uint8, storage.RomSize)

	code := []uint8{
		// $000008, still on the overlay map
		0x4e, 0xf9, 0x00, 0x40, 0x00, 0x14, // JMP $00400014
	}
	copy(data[8:], code)

	code = []uint8{
		// $400014, running from the ROM on its normal address
		0x13, 0xfc, 0x00, 0xff, 0x00, 0xef, 0xe7, 0xfe, // MOVE.B #$ff,$00efe7fe  DDRA all outputs
		0x13, 0xfc, 0x00, 0x00, 0x00, 0xef, 0xff, 0xfe, // MOVE.B #$00,$00effffe  port A, clears the overlay
		0x7e, 0x0f, // MOVEQ #$0f,D7   the exception class
		0x7c, 0x02, // MOVEQ #2,D6     an address error
		0x60, 0xfe, // BRA *           the loop the ROM ends on
	}
	copy(data[0x14:], code)

	// The reset stack pointer and program counter
	data[0], data[1], data[2], data[3] = 0x00, 0x60, 0x04, 0x00
	data[4], data[5], data[6], data[7] = 0x00, 0x00, 0x00, 0x08

	return data
}

func newOverlayTestMac(t *testing.T) *Mac {
	t.Helper()

	config := NewConfiguration()
	config.RomFile = "<test>"
	config.Trace = "sadmac"

	return mustNewMac(t, config, storage.RomFromData(buildOverlayTestRom()), nil, nil)
}

func TestTheRomLeavesTheOverlayMap(t *testing.T) {
	m := newOverlayTestMac(t)
	m.RunFrames(1)

	if m.mm.overlay {
		t.Fatal("the overlay was not cleared through the VIA")
	}

	if m.GetPC() != 0x40_0028 {
		t.Errorf("the run ended at $%06x, wanted $400028", m.GetPC())
	}
}

func TestTheMachineHaltsAndReportsTheCode(t *testing.T) {
	m := newOverlayTestMac(t)
	m.RunFrames(60)

	report, halted := m.GetSadMac()
	if !halted {
		t.Fatal("the loop at the end of the ROM was not detected")
	}

	if !strings.Contains(report, "0F0002") {
		t.Errorf("%q does not show the code the screen would", report)
	}
	if !strings.Contains(report, "address error") {
		t.Errorf("%q does not name the exception", report)
	}
}

func TestTheRunStopsEarlyOnAHalt(t *testing.T) {
	m := newOverlayTestMac(t)
	m.RunFrames(10000)

	// The run has to give up long before the frames asked for
	if m.GetFrames() > 10 {
		t.Errorf("the run went on for %v frames after the machine halted", m.GetFrames())
	}
}

func TestTheRomIsReachableOnBothMapsWhileTheOverlayIsSet(t *testing.T) {
	m := newOverlayTestMac(t)
	m.reset()

	// The jump at the start of the ROM works because the same bytes answer
	// at $000008 and at $400008
	for address := uint32(8); address < 0x30; address++ {
		if m.mm.Peek(address) != m.mm.Peek(romBase+address) {
			t.Fatalf("$%06x and $%06x differ while the overlay is set",
				address, romBase+address)
		}
	}
}

package izmac

import (
	"testing"

	"github.com/ivanizag/izmac/component"
)

// viaAddress returns the address of a VIA register, the way the ROM reaches
// it: the base $efe1fe with the registers 512 bytes apart
func viaAddress(reg uint32) uint32 {
	return 0xef_e1fe + reg*0x200
}

const (
	viaRegPortB = 0
	viaRegDdrB  = 2
	viaRegDdrA  = 3
	viaRegShift = 10
	viaRegPortA = 15 // The Macintosh uses the port A without handshaking
)

func newTestVia(t *testing.T) (*via, *memoryManager, *video) {
	t.Helper()

	mm := newTestMemoryManager(1024)
	v := newVideo(mm)
	return newVia(mm, v, newIwm(false), component.NewAppleRTC("", false), newKeyboard(), newMouse(), newSound(mm)), mm, v
}

func TestTheViaRegistersAreEvery512Bytes(t *testing.T) {
	for reg := uint32(0); reg < 16; reg++ {
		got := viaRegister(viaAddress(reg))
		if uint32(got) != reg {
			t.Errorf("$%06x decoded to the register %v, wanted %v",
				viaAddress(reg), got, reg)
		}
	}
}

func TestTheViaIsMirroredOverItsRegion(t *testing.T) {
	// Only the address lines 9 to 12 are decoded, so the block repeats
	base := viaAddress(viaRegDdrA)
	for _, address := range []uint32{base, base + 0x2000, base + 0x10_0000} {
		if viaRegister(address) != viaRegDdrA {
			t.Errorf("$%06x does not reach the register %v", address, viaRegDdrA)
		}
	}
}

func TestTheOverlayIsSetWhileThePinsAreInputs(t *testing.T) {
	_, mm, _ := newTestVia(t)

	// After a reset the port A is all inputs and the pull ups keep the
	// overlay asserted, which is what puts the ROM on the reset vectors
	if !mm.overlay {
		t.Error("the overlay is not set after a reset")
	}
}

func TestTheRomClearsTheOverlay(t *testing.T) {
	via, mm, _ := newTestVia(t)

	// The ROM makes the port A an output and writes the overlay bit low
	via.poke(viaAddress(viaRegDdrA), 0xff)
	via.poke(viaAddress(viaRegPortA), 0x00)

	if mm.overlay {
		t.Error("writing the port A bit 4 low did not clear the overlay")
	}

	// And setting it back brings the ROM to the address zero again
	via.poke(viaAddress(viaRegPortA), viaPortAOverlay)
	if !mm.overlay {
		t.Error("writing the port A bit 4 high did not set the overlay")
	}
}

func TestAPinConfiguredAsInputDoesNotDriveTheOverlay(t *testing.T) {
	via, mm, _ := newTestVia(t)

	// The overlay bit stays an input, so writing the output register must
	// not reach the memory manager
	via.poke(viaAddress(viaRegDdrA), 0xff&^viaPortAOverlay)
	via.poke(viaAddress(viaRegPortA), 0x00)

	if !mm.overlay {
		t.Error("an output register write reached a pin configured as an input")
	}
}

func TestTheVideoPageIsSelectedByPortA(t *testing.T) {
	via, _, v := newTestVia(t)

	via.poke(viaAddress(viaRegDdrA), 0xff)

	via.poke(viaAddress(viaRegPortA), viaPortAVideoPage)
	if v.alternate {
		t.Error("the port A bit 6 high did not select the main video page")
	}

	via.poke(viaAddress(viaRegPortA), 0x00)
	if !v.alternate {
		t.Error("the port A bit 6 low did not select the alternate video page")
	}
}

func TestTheViaRunsAtATenthOfTheProcessor(t *testing.T) {
	via, _, _ := newTestVia(t)

	// Nine cycles are not enough for one tick of the E clock
	via.tick(9)
	if via.eClockRemainder != 9 {
		t.Errorf("after 9 cycles the remainder is %v, wanted 9", via.eClockRemainder)
	}

	// The tenth completes it
	via.tick(1)
	if via.eClockRemainder != 0 {
		t.Errorf("after 10 cycles the remainder is %v, wanted 0", via.eClockRemainder)
	}

	// And the remainder is kept between calls
	via.tick(25)
	if via.eClockRemainder != 5 {
		t.Errorf("after 25 more cycles the remainder is %v, wanted 5", via.eClockRemainder)
	}
}

func TestTheViaIsReachableThroughTheMemoryManager(t *testing.T) {
	via, mm, _ := newTestVia(t)
	mm.via = via
	mm.setOverlay(false)

	// Writing the data direction register through the address space and
	// reading it back
	mm.Poke(viaAddress(viaRegDdrB), 0x55)
	if mm.Peek(viaAddress(viaRegDdrB)) != 0x55 {
		t.Error("the VIA is not reachable through the memory manager")
	}
}

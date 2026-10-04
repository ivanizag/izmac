package izmac

import (
	"slices"
	"testing"
)

// inspectedMac is a machine with nothing running, its RAM out from under the
// ROM, and low memory filled in by the test as the System would
func inspectedMac(t *testing.T) *Mac {
	t.Helper()
	m := newTestMac(t)
	m.RunFrames(1)
	m.mm.setOverlay(false)
	return m
}

func putBytes(m *Mac, address uint32, data ...uint8) {
	copy(m.mm.ram[address:], data)
}

func putLong(m *Mac, address uint32, value uint32) {
	putBytes(m, address, uint8(value>>24), uint8(value>>16), uint8(value>>8), uint8(value))
}

func putString(m *Mac, address uint32, text string) {
	putBytes(m, address, append([]uint8{uint8(len(text))}, text...)...)
}

func TestPeekReadsTheRamAndTheRomBigEndian(t *testing.T) {
	m := inspectedMac(t)
	putBytes(m, 0x1000, 0x12, 0x34, 0x56, 0x78)

	if got := m.PeekLong(0x1000); got != 0x12345678 {
		t.Errorf("the long reads %08x", got)
	}
	if got := m.PeekWord(0x1002); got != 0x5678 {
		t.Errorf("the word reads %04x", got)
	}
	// The reset vector of the test ROM, at the start of the ROM's window
	if got := m.PeekLong(0x40_0004); got != 0x00000008 {
		t.Errorf("the ROM reads %08x at the reset program counter", got)
	}
}

func TestPeekLeavesTheChipsAlone(t *testing.T) {
	m := inspectedMac(t)
	for _, address := range []uint32{0x58_0000, 0x9f_fff8, 0xbf_fff0, 0xef_e1fe} {
		if got := m.Peek(address); got != 0 {
			t.Errorf("the chip at %06x read %02x, wanted nothing", address, got)
		}
	}
}

func TestThePointerIsWhereTheMouseLeftIt(t *testing.T) {
	m := inspectedMac(t)
	putBytes(m, lowRawMouseV, 0x00, 0x64)
	putBytes(m, lowRawMouseH, 0x01, 0x2c)

	if h, v := m.PointerPosition(); h != 300 || v != 100 {
		t.Errorf("the pointer is at %v,%v, wanted 300,100", h, v)
	}
}

func TestTheApplicationIsNamedInMacRoman(t *testing.T) {
	m := inspectedMac(t)
	putString(m, lowCurApName, "Caf\x8e")

	if got := m.CurrentApplication(); got != "Café" {
		t.Errorf("the application is %q", got)
	}
}

func TestTheVolumesAreTheQueueInOrder(t *testing.T) {
	m := inspectedMac(t)
	const first, second = 0x2000, 0x2100
	putLong(m, lowVCBQHead, first)
	putLong(m, first, second)
	putString(m, first+vcbName, "System 6 HD")
	putLong(m, second, 0)
	putString(m, second+vcbName, "Letters")

	if got := m.MountedVolumes(); !slices.Equal(got, []string{"System 6 HD", "Letters"}) {
		t.Errorf("the volumes are %q", got)
	}
}

func TestTheApplicationVolumeIsTheOneOfItsResourceFile(t *testing.T) {
	m := inspectedMac(t)
	if got := m.ApplicationVolume(); got != "" {
		t.Errorf("with nothing started the volume is %q", got)
	}

	const fcbs, ref, vcb = 0x3000, 0x0040, 0x2000
	putLong(m, lowFCBSPtr, fcbs)
	putBytes(m, lowCurApRefNum, 0x00, ref)
	putLong(m, fcbs+ref+fcbVPtr, vcb)
	putString(m, vcb+vcbName, "Paint")

	if got := m.ApplicationVolume(); got != "Paint" {
		t.Errorf("the application's volume is %q", got)
	}
}

func TestAScreenshotIsNotDrawnOver(t *testing.T) {
	m := inspectedMac(t)
	shot := m.Screenshot()
	before := slices.Clone(shot.Pix)

	m.GetImage().Pix[0] ^= 0xff
	if !slices.Equal(before, shot.Pix) {
		t.Errorf("the screenshot changed with the screen")
	}
}

func TestRunFramesTakesTheCommands(t *testing.T) {
	m := newTestMac(t)
	m.RunFrames(1)

	m.SendCommand(CommandKill)
	m.RunFrames(10)
	if m.GetFrames() != 1 {
		t.Errorf("the machine ran %v frames after being killed, wanted none", m.GetFrames()-1)
	}
}

func TestClosingTwiceIsFine(t *testing.T) {
	m := newTestMac(t)
	m.RunFrames(1)
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Errorf("closing again: %v", err)
	}
}

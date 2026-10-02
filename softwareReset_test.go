package izmac

import (
	"testing"

	"github.com/ivanizag/izmac/storage"
)

/*
resetOnceRom is a ROM that counts its starts in RAM and executes RESET on the
first one, then branches to itself. The count is what says the RESET started
the machine again.
*/
func resetOnceRom() *storage.Rom {
	data := make([]uint8, storage.RomSize)
	copy(data, []uint8{
		0x00, 0x60, 0x04, 0x00, // the reset stack pointer
		0x00, 0x00, 0x00, 0x08, // the reset program counter
		0x52, 0x79, 0x00, 0x60, 0x01, 0x00, // ADDQ.W #1,$600100
		0x0c, 0x79, 0x00, 0x01, 0x00, 0x60, 0x01, 0x00, // CMPI.W #1,$600100
		0x66, 0x02, // BNE.S past the RESET
		0x4e, 0x70, // RESET
		0x60, 0xfe, // BRA.S to itself
	})
	return storage.RomFromData(data)
}

// starts is the count the ROM keeps
func starts(m *Mac) int {
	return int(m.mm.Peek(0x600100))<<8 | int(m.mm.Peek(0x600101))
}

func TestTheResetInstructionStartsTheMachineAgain(t *testing.T) {
	config, _ := quietConfiguration()
	config.RomFile = "<test>"
	m := ensureNewMac(t, config, resetOnceRom(), nil, nil)

	m.reset()
	m.RunFrames(2)

	if n := starts(m); n != 2 {
		t.Fatalf("the machine started %v times, wanted twice: once and again after RESET", n)
	}

	// With no disk to start from, that was a Shut Down
	if !m.IsReadyToSwitchOff() {
		t.Errorf("a RESET that left no disk to start from was not taken for a Shut Down")
	}

	// And a diskette put in is something to start from again
	floppy := writeImage(t, "floppy.dsk", 800*1024, false)
	if err := m.InsertDiskette(DriveInternal, floppy); err != nil {
		t.Fatal(err)
	}
	m.RunFrames(1)
	if m.IsReadyToSwitchOff() {
		t.Errorf("the machine is still ready to be switched off with a diskette in it")
	}
}

func TestAResetWithADiskIsARestart(t *testing.T) {
	config, _ := quietConfiguration()
	config.RomFile = "<test>"
	m := ensureNewMac(t, config, resetOnceRom(),
		[]storage.BlockDisk{storage.NewBlockDiskMemory(64)}, nil)

	m.reset()
	m.RunFrames(2)

	if n := starts(m); n != 2 {
		t.Fatalf("the machine started %v times, wanted twice", n)
	}
	if m.IsReadyToSwitchOff() {
		t.Errorf("a RESET with a hard disk on the bus was taken for a Shut Down")
	}
}

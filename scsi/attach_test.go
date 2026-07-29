package scsi

import (
	"testing"

	"github.com/ivanizag/izmac/storage"
)

// Each device answers only to its own id, so the driver reaches the one it
// selected and not another
func TestEachDiskAnswersToItsOwnId(t *testing.T) {
	bus := NewBus()
	for id := 0; id < 3; id++ {
		disk := storage.NewBlockDiskMemory(uint32(16 + id))
		bus.Attach(NewDisk(uint8(id), disk, false))
	}

	for id := uint8(0); id < 3; id++ {
		selectTarget(bus, id)
		if bus.selected == nil {
			t.Fatalf("selecting the id %v answered nothing", id)
		}
		if bus.selected.id != id {
			t.Errorf("selecting the id %v answered the id %v", id, bus.selected.id)
		}

		// Read the capacity, which is different on each of them
		data, status := runCommand(bus,
			[]uint8{cmdReadCapacity, 0, 0, 0, 0, 0, 0, 0, 0, 0})
		if status != statusGood {
			t.Fatalf("the id %v answered the status $%02x", id, status)
		}

		last := uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])
		if wanted := uint32(16+id) - 1; last != wanted {
			t.Errorf("the id %v reports the last block %v, wanted %v", id, last, wanted)
		}
	}
}

func TestSelectingAnIdWithNothingOnItAnswersNothing(t *testing.T) {
	bus := NewBus()
	bus.Attach(NewDisk(0, storage.NewBlockDiskMemory(16), false))

	selectTarget(bus, 3)
	if bus.selected != nil {
		t.Errorf("the id 3 was answered by the device at the id %v", bus.selected.id)
	}
	if bus.currentPhase() != phaseBusFree {
		t.Errorf("the bus is on %v after selecting nothing", bus.currentPhase())
	}
}

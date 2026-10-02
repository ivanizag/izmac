package izmac

import (
	"testing"

	"github.com/ivanizag/izmac/storage"
)

// The alert that says the power can go is told by the code SysError leaves
// in low memory, and by nothing else
func TestTheShutDownAlertIsTheMachineReadyToSwitchOff(t *testing.T) {
	config, _ := quietConfiguration()
	config.RomFile = "<test>"
	m := ensureNewMac(t, config, storage.RomFromData(make([]uint8, storage.RomSize)), nil, nil)

	// Low memory is the ROM until the overlay goes, which the ROM does as it
	// starts and this machine has no ROM to do
	m.mm.setOverlay(false)

	setCode := func(code uint16) {
		m.mm.Poke(dsErrCodeAddress, uint8(code>>8))
		m.mm.Poke(dsErrCodeAddress+1, uint8(code))
		m.watchSwitchOff()
	}

	setCode(40) // the greeting, at every boot
	if m.IsReadyToSwitchOff() {
		t.Errorf("the greeting was taken for the shut down alert")
	}

	setCode(shutDownAlert)
	if !m.IsReadyToSwitchOff() {
		t.Errorf("the shut down alert was not noticed")
	}

	// Restart, and the memory test writing over low memory
	setCode(0xffff)
	if m.IsReadyToSwitchOff() {
		t.Errorf("the machine was still taken as shut down after it restarted")
	}
}

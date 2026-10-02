package izmac

import (
	"os"
	"testing"
)

/*
Shut Down chosen from the Finder, end to end, on System 6 and on System 7. The
two put up different words on the alert, and the same code behind them, which
is what says the machine is ready to be switched off. Restart, from the alert,
takes it back.
*/

// shutDownFromTheFinder opens the Special menu at the given place on the menu
// bar and lets go on Shut Down
func shutDownFromTheFinder(t *testing.T, m *Mac, menuH int16, itemH int16, itemV int16) {
	t.Helper()

	moveMouseTo(t, m, menuH, 10)
	m.SetMouseButton(true)
	m.RunFrames(30)
	moveMouseTo(t, m, itemH, itemV)
	m.RunFrames(10)
	m.SetMouseButton(false)
}

// waitForSwitchOff runs the machine until it says it can be switched off
func waitForSwitchOff(t *testing.T, m *Mac) {
	t.Helper()

	for poll := 0; poll < 30; poll++ {
		m.RunFrames(100)
		if m.IsReadyToSwitchOff() {
			return
		}
	}
	t.Fatalf("the machine was never ready to be switched off")
}

func TestShutDownOnSystemSix(t *testing.T) {
	m := bootedMac(t)
	if m.IsReadyToSwitchOff() {
		t.Fatalf("the machine is ready to be switched off before anyone asked")
	}

	// Special is the fourth menu, and Shut Down the last item in it
	shutDownFromTheFinder(t, m, 185, 200, 123)
	waitForSwitchOff(t, m)

	// Restart, the button on the right of the alert
	moveMouseTo(t, m, 432, 163)
	clickMouse(m)
	m.RunFrames(60)
	if m.IsReadyToSwitchOff() {
		t.Errorf("the machine is still ready to be switched off after restarting")
	}
}

func TestShutDownOnSystemSeven(t *testing.T) {
	config := realConfig(t)
	if _, err := os.Stat(systemSevenDisk); err != nil {
		t.Skipf("%v is not here, this test needs it", systemSevenDisk)
	}
	config.DiskFiles = []string{systemSevenDisk}
	config.RamSizeKb = 4096
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}

	m, err := NewMac(config)
	if err != nil {
		t.Fatal(err)
	}
	m.RunFrames(systemSevenBootFrames)

	// System 7 has a Label menu before Special
	shutDownFromTheFinder(t, m, 215, 240, 139)
	waitForSwitchOff(t, m)
}

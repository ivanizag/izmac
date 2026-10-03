package izmac

import (
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
	t.Parallel()
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
	t.Parallel()
	m := systemSevenMac(t)

	// System 7 has a Label menu before Special
	shutDownFromTheFinder(t, m, 215, 240, 139)
	waitForSwitchOff(t, m)
}

/*
The MacPaint diskette, with the System 2.0 of 1985 on it, where Shut Down is the
Finder ejecting the diskette and executing RESET. The machine starts again with
no disk and waits for one with the flashing question mark, and stays there:
there is no alert to say it can be switched off, and izmac does not close. The
diskette is the one izmac fetches when nothing is named.
*/
func TestShutDownOnSystemTwo(t *testing.T) {
	t.Parallel()
	config := testConfig(t)
	config.Diskettes = []string{testImage(t, testPaintDiskette)}
	m := buildTestMac(t, config)
	m.RunFrames(bootFrames)

	// Special is where it is on System 6, and Shut Down its last item
	shutDownFromTheFinder(t, m, 185, 200, 123)
	m.RunFrames(1500)

	if drive := m.GetDiskette(DriveInternal); drive.Image != "" {
		t.Errorf("the diskette is still in the drive after Shut Down")
	}

	/*
		Had the machine not started again, the Finder would have carried on
		and asked for the diskette it had just ejected, with the disk switch
		alert, whose code is 30
	*/
	const diskSwitchAlert = 30
	code := uint16(m.mm.Peek(dsErrCodeAddress))<<8 | uint16(m.mm.Peek(dsErrCodeAddress+1))
	if code == diskSwitchAlert {
		t.Errorf("the machine asks for the diskette back instead of starting again")
	}

	if m.IsReadyToSwitchOff() {
		t.Errorf("the machine waiting for a disk was taken for it switched off")
	}
}

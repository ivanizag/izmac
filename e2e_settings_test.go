package izmac

import (
	"testing"
)

/*
A setting made on the machine is there the next time it starts: the speaker
volume of the Control Panel, which the Macintosh keeps in the parameter RAM of
the clock, and izmac in the file it is given for it. The ROM puts it in
SPVolCtl, $0208, as it starts, the volume in the bits 0 to 2.
*/
func TestASettingIsThereTheNextTime(t *testing.T) {
	t.Parallel()
	const spVolCtl, wanted = 0x0208, 6
	volume := func(m *Mac) uint8 { return m.mm.Peek(spVolCtl) & 7 }

	config := realConfig(t)
	m := buildTestMac(t, config)
	waitForApplication(t, m, "Finder", 60)
	m.RunFrames(300)
	if volume(m) == wanted {
		t.Fatalf("the volume is %v before it was set", wanted)
	}

	// The Control Panel, from the Apple menu, and its volume slider dragged
	// from where it is to the 6
	moveMouseTo(t, m, 16, 10)
	m.SetMouseButton(true)
	m.RunFrames(30)
	moveMouseTo(t, m, 60, 107)
	m.RunFrames(10)
	m.SetMouseButton(false)
	m.RunFrames(900)
	moveMouseTo(t, m, 396, 212)
	m.SetMouseButton(true)
	m.RunFrames(20)
	moveMouseTo(t, m, 396, 190)
	moveMouseTo(t, m, 396, 175)
	m.RunFrames(20)
	m.SetMouseButton(false)
	m.RunFrames(120)
	if volume(m) != wanted {
		t.Fatalf("the slider left the volume at %v, wanted %v", volume(m), wanted)
	}

	// Another machine on the same parameter RAM file
	again := testConfig(t)
	again.DiskFiles = []string{testImage(t, testSystemSixDisk)}
	again.PramFile = config.PramFile
	n := buildTestMac(t, again)
	waitForApplication(t, n, "Finder", 60)
	if got := volume(n); got != wanted {
		t.Errorf("the next time the volume is %v, wanted the %v set", got, wanted)
	}
}

package izmac

import "testing"

// The soft switches, as offsets from the IWM base
const (
	iwmBaseAddress = 0xdf_e1ff

	iwmSwCa0L   = 0
	iwmSwCa0H   = 1
	iwmSwCa1L   = 2
	iwmSwCa1H   = 3
	iwmSwCa2L   = 4
	iwmSwCa2H   = 5
	iwmSwLstrbL = 6
	iwmSwLstrbH = 7
	iwmSwEnblL  = 8
	iwmSwEnblH  = 9
	iwmSwSelL   = 10
	iwmSwSelH   = 11
	iwmSwQ6L    = 12
	iwmSwQ6H    = 13
	iwmSwQ7L    = 14
	iwmSwQ7H    = 15
)

func iwmAddress(reg uint32) uint32 {
	return iwmBaseAddress + reg*0x200
}

func TestTheIwmSwitchesAreEvery512Bytes(t *testing.T) {
	for reg := uint32(0); reg < 16; reg++ {
		if got := iwmRegister(iwmAddress(reg)); uint32(got) != reg {
			t.Errorf("$%06x reached the switch %v, wanted %v",
				iwmAddress(reg), got, reg)
		}
	}
}

func TestTheIwmIsMirroredOverItsRegion(t *testing.T) {
	// The base itself is the first switch, and the ROM reaches the enable
	// one by adding $1000 to it
	if iwmRegister(iwmBaseAddress) != iwmSwCa0L {
		t.Error("the IWM base is not the first switch")
	}
	if iwmRegister(iwmBaseAddress+0x1000) != iwmSwEnblL {
		t.Error("the offset the ROM uses does not reach the enable switch")
	}

	// Only the address lines 9 to 12 are decoded, so the block repeats
	if iwmRegister(0xdf_f1ff) != iwmRegister(0xdf_d1ff) {
		t.Error("the IWM does not mirror over its region")
	}
}

func TestTheSwitchesSetAndClearTheLines(t *testing.T) {
	d := newIwm(false)

	d.peek(iwmAddress(iwmSwEnblH))
	if !d.enable {
		t.Error("the odd address did not set the enable line")
	}
	d.peek(iwmAddress(iwmSwEnblL))
	if d.enable {
		t.Error("the even address did not clear the enable line")
	}

	// A write touches the switch just as a read does
	d.poke(iwmAddress(iwmSwQ7H), 0)
	if !d.q7 {
		t.Error("a write did not reach the switch")
	}
}

/*
The sequence the ROM runs to check that the chip is there. Getting this wrong
leaves the machine looping at $400104 forever, which is what a stub answering
$ff to everything did.
*/
func TestTheRomPresenceHandshake(t *testing.T) {
	d := newIwm(false)
	const wantedMode = 0x1f

	// First pass: the drive off, Q6 high, then read the status through the
	// Q7 low switch
	d.peek(iwmAddress(iwmSwEnblL))
	d.peek(iwmAddress(iwmSwQ6H))
	status := d.peek(iwmAddress(iwmSwQ7L))

	if status&iwmStatusEnable != 0 {
		t.Fatal("the status reports the drive enabled after the enable was cleared")
	}
	if status&iwmModeMask == wantedMode {
		t.Fatal("the mode read back before it was written")
	}

	// The ROM writes the mode it wants through the Q7 high switch
	d.poke(iwmAddress(iwmSwQ7H), wantedMode)
	d.peek(iwmAddress(iwmSwQ7L))

	// Second pass: it has to read back
	d.peek(iwmAddress(iwmSwEnblL))
	d.peek(iwmAddress(iwmSwQ6H))
	status = d.peek(iwmAddress(iwmSwQ7L))

	if status&iwmStatusEnable != 0 {
		t.Error("the status reports the drive enabled")
	}
	if status&iwmModeMask != wantedMode {
		t.Errorf("the mode read back as $%02x, wanted $%02x",
			status&iwmModeMask, wantedMode)
	}
}

func TestTheModeIsOnlyWritableWithTheDriveOff(t *testing.T) {
	d := newIwm(false)

	d.peek(iwmAddress(iwmSwEnblH))
	d.peek(iwmAddress(iwmSwQ6H))
	d.poke(iwmAddress(iwmSwQ7H), 0x1f)

	if d.mode != 0 {
		t.Error("the mode register was written while the drive was enabled")
	}
}

func TestTheWriteHandshakeIsReadyWithNoDisk(t *testing.T) {
	d := newIwm(false)

	// Q7 high and Q6 low reads the write handshake. With no disk to write
	// to it has to say the buffer is empty, or a write would wait forever.
	d.peek(iwmAddress(iwmSwQ6L))
	got := d.peek(iwmAddress(iwmSwQ7H))

	if wanted := iwmHandshakeReady | iwmHandshakeUnderrun; got != wanted {
		t.Errorf("the write handshake reads $%02x, wanted $%02x", got, wanted)
	}
}

func TestThereIsNoDiskToRead(t *testing.T) {
	d := newIwm(false)

	// Q6 and Q7 low reads the data register, and nothing is turning
	d.peek(iwmAddress(iwmSwQ6L))
	if got := d.peek(iwmAddress(iwmSwQ7L)); got != 0 {
		t.Errorf("the data register reads $%02x with no disk, wanted 0", got)
	}
}

// The drive presence line is what gets a drive into the drive queue, without
// which the ROM hangs at $4006e8. Everything else stays negated, which says
// there is a drive with no disk in it.
func TestADriveIsReportedPresentWithNoDisk(t *testing.T) {
	d := newIwm(false)

	setSelector := func(selector uint8) {
		d.ca2 = selector&(1<<3) != 0
		d.ca1 = selector&(1<<2) != 0
		d.ca0 = selector&(1<<1) != 0
		d.headSelect = selector&1 != 0
	}

	sense := func(selector uint8) bool {
		setSelector(selector)
		return d.selected().sense(d.senseSelector())
	}

	// The line is active low, so a drive answers by pulling it down
	if sense(senseDriveInstalled) {
		t.Error("no drive is reported connected, none would register")
	}

	// The disk in place line has to stay negated, there is no disk
	if !sense(senseDiskInPlace) {
		t.Error("a disk is reported in place")
	}

	// And a double sided drive, which is what the Plus has
	if !sense(senseSides) {
		t.Error("the drive is reported single sided")
	}
}

/*
The interface line is the one that tells the driver which of the two drives
the Macintosh ever shipped with it is talking to, and it is not active low.
Reporting an 800K drive is what keeps the driver away from the speed
calibration of the 400K one, where it would set a pulse width through the
sound buffer and time the tachometer to see what came of it.
*/
func TestAnEightHundredKDriveIsReported(t *testing.T) {
	d := newIwm(false)

	d.ca2, d.ca1, d.ca0 = true, true, true
	d.setHeadSelect(true)

	if d.senseSelector() != senseNewInterface {
		t.Fatalf("the selector is %v, wanted %v", d.senseSelector(), senseNewInterface)
	}
	if !d.selected().sense(senseNewInterface) {
		t.Error("the drive reports the old 400K interface")
	}
}

func TestTheHeadSelectIsPartOfTheSelector(t *testing.T) {
	d := newIwm(false)

	d.ca2, d.ca1, d.ca0 = true, true, true

	d.setHeadSelect(false)
	if d.senseSelector() != 14 {
		t.Errorf("the selector is %v with the head select low, wanted 14", d.senseSelector())
	}

	d.setHeadSelect(true)
	if d.senseSelector() != 15 {
		t.Errorf("the selector is %v with the head select high, wanted 15", d.senseSelector())
	}
}

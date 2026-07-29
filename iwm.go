package izmac

/*
The Integrated Woz Machine is the floppy controller. Emulating the drives is
the long tail of this project and is not the way into it, but the ROM talks
to the chip on the power on path long before it looks for a disk, so it needs
something here that answers the way the hardware would.

What the ROM does first is check that the chip is there:

	TST.B   ($1000,A0)      the enable line low, the drive off
	TST.B   ($1a00,A0)      Q6 high
	MOVE.B  ($1c00,A0),D2   Q7 low, so this reads the status register
	BTST.L  #5,D2           wait for the enable to be seen low
	BNE     ...
	AND.B   D0,D2           does the mode read back as the $1f written?
	CMP.B   D0,D2
	BEQ     ...
	MOVE.B  D0,($1e00,A0)   Q7 high, write the mode register

So the mode register has to be writable and has to show up on the low five
bits of the status, and the enable has to be reported. Everything else here
is what an unconnected drive would look like.

The sixteen addresses are soft switches rather than registers, each one
setting or clearing a line, and they are 512 bytes apart as the VIA ones are.
Touching one takes effect whether the access is a read or a write.
*/
type iwm struct {
	ca0   bool
	ca1   bool
	ca2   bool
	lstrb bool

	// enable is the drive motor and select line
	enable bool
	// sel picks between the two drives
	sel bool
	// headSelect is the SEL line, driven by the VIA port A bit 5. It is
	// part of the selector of the drive status lines.
	headSelect bool

	// q6 and q7 select what a read or a write reaches
	q6 bool
	q7 bool

	mode uint8
}

const (
	// iwmModeMask is the part of the mode register that reads back on the
	// status
	iwmModeMask uint8 = 0x1f

	// iwmStatusEnable is the drive enable, the bit the ROM waits on
	iwmStatusEnable uint8 = 1 << 5
	// iwmStatusSense is the selected status line of the drive. With no
	// drive connected the line is not pulled down.
	iwmStatusSense uint8 = 1 << 7

	// iwmHandshakeReady says the write buffer is empty and that there was
	// no underrun, so that a write never waits forever
	iwmHandshakeReady uint8 = 0xc0
)

func newIwm() *iwm {
	return &iwm{}
}

// iwmRegister returns the soft switch an address touches
func iwmRegister(address uint32) uint8 {
	return uint8((address >> 9) & 0x0f)
}

func (d *iwm) peek(address uint32) uint8 {
	d.applySwitch(iwmRegister(address))
	return d.read()
}

func (d *iwm) poke(address uint32, value uint8) {
	d.applySwitch(iwmRegister(address))
	d.write(value)
}

// applySwitch sets or clears the line an address is wired to
func (d *iwm) applySwitch(reg uint8) {
	on := reg&1 != 0

	switch reg >> 1 {
	case 0:
		d.ca0 = on
	case 1:
		d.ca1 = on
	case 2:
		d.ca2 = on
	case 3:
		d.lstrb = on
	case 4:
		d.enable = on
	case 5:
		d.sel = on
	case 6:
		d.q6 = on
	case 7:
		d.q7 = on
	}
}

// read returns the register selected by Q6 and Q7
func (d *iwm) read() uint8 {
	switch {
	case !d.q6 && !d.q7:
		// The data register. With no disk turning there is nothing to
		// read, and a zero is not a valid encoded byte.
		return 0
	case d.q6 && !d.q7:
		return d.status()
	case !d.q6 && d.q7:
		return iwmHandshakeReady
	default:
		return 0xff
	}
}

// write reaches the mode register only, and only while the drive is off
func (d *iwm) write(value uint8) {
	if d.q6 && d.q7 && !d.enable {
		d.mode = value & iwmModeMask
	}
}

// status is the mode register on the low bits, the state of the enable, and
// the status line selected on the drive
func (d *iwm) status() uint8 {
	status := d.mode & iwmModeMask

	if d.enable {
		status |= iwmStatusEnable
	}
	if d.sense() {
		status |= iwmStatusSense
	}

	return status
}

// setHeadSelect drives the SEL line, which on the Macintosh comes from the
// VIA port A bit 5 and not from the IWM. It is the fourth bit of the status
// line selector.
func (d *iwm) setHeadSelect(level bool) {
	d.headSelect = level
}

// senseSelector is the status line the drive is being asked about, made of
// CA2, CA1, CA0 and SEL
func (d *iwm) senseSelector() uint8 {
	var selector uint8
	if d.ca2 {
		selector |= 1 << 3
	}
	if d.ca1 {
		selector |= 1 << 2
	}
	if d.ca0 {
		selector |= 1 << 1
	}
	if d.headSelect {
		selector |= 1
	}
	return selector
}

/*
The drive status lines, selected by CA2, CA1, CA0 and SEL in that order from
the high bit. From the table in Inside Macintosh volume III, page III-35.

	 0  DIRTN     head step direction
	 1  CSTIN     disk in place
	 2  STEP      head stepping
	 3  WRTPRT    disk locked
	 4  MOTORON   motor running
	 5  TKO       head at track 0
	 7  TACH      tachometer
	 8  RDDATA0   read data, lower head
	 9  RDDATA1   read data, upper head
	12  SIDES     single or double sided
	15  DRVIN     drive installed

They are active low with two exceptions worth remembering: SIDES is 1 on a
double sided drive, and DRVIN is 0 when a drive is connected and floats to 1
when none is.
*/
const (
	senseCstin = 1
	senseSides = 12
	senseDrvin = 15

	// The Plus ROM polls 14 rather than the 15 the book gives for DRVIN,
	// and polls it far more often than anything else, which is what a
	// presence check looks like. The book documents the 400K drive of the
	// 128K and 512K machines and the Plus ships an 800K one, so the two
	// are probably the same line on different drives. Both are asserted:
	// answering only 15 leaves the ROM hanging on an empty drive queue,
	// and answering only 14 disagrees with the book for no reason.
	senseDrvinPlus = 14
)

/*
sense reports the drive status line selected, active low, so a false means the
condition the line names is true.

A drive is reported present with no disk in it, which is what an empty floppy
drive looks like. It matters more than it sounds: the ROM walks the drive
queue at DrvQHdr, $0308, and hangs on purpose at $4006e8 when it is empty, so
a Macintosh with no drive at all never finishes booting.

SIDES is left high, which says a double sided drive, the one the Plus has.
*/
func (d *iwm) sense() bool {
	switch d.senseSelector() {
	case senseDrvin, senseDrvinPlus:
		return false // A drive is connected
	default:
		return true // Everything else negated, including no disk in place
	}
}

func (d *iwm) reset() {
	*d = iwm{}
}

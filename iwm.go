package izmac

/*
The Integrated Woz Machine is the floppy controller, and the two Sony drives
of the Macintosh Plus hang off it.

The sixteen addresses it answers at are soft switches rather than registers,
each one setting or clearing a line, and they are 512 bytes apart as the VIA
ones are. Touching one takes effect whether the access is a read or a write.

	 0,1  CA0     the phase lines, which pick which of the sixteen status
	 2,3  CA1     lines of the drive is being asked about and which of the
	 4,5  CA2     four control lines is about to be written
	 6,7  LSTRB   the strobe that latches a control line
	 8,9  ENABLE  powers the drive up
	10,11 SELECT  which of the two drives, the internal one or the external
	12,13 Q6      together, what a read or a write reaches
	14,15 Q7

The fourth bit of the status line selector is not here at all: it is the SEL
line, which comes from the bit 5 of the VIA port A.

What Q6 and Q7 reach:

	Q6 low,  Q7 low   the data going to and from the disk
	Q6 high, Q7 low   the status, with the selected drive line on the top bit
	Q6 low,  Q7 high  the handshake, which says a byte can be written
	Q6 high, Q7 high  the mode register with the drive off, data with it on

The ROM checks the chip is there before anything else works, long before it
looks for a disk:

	TST.B   ($1000,A0)      the enable line low, the drive off
	TST.B   ($1a00,A0)      Q6 high
	MOVE.B  ($1c00,A0),D2   Q7 low, so this reads the status register
	BTST.L  #5,D2           wait for the enable to be seen low
	BNE     ...
	AND.B   D0,D2           does the mode read back as the $1f written?
	CMP.B   D0,D2
	BEQ     ...
	MOVE.B  D0,($1e00,A0)   Q7 high, write the mode register
*/
type iwm struct {
	ca0   bool
	ca1   bool
	ca2   bool
	lstrb bool

	// enable is the drive power line
	enable bool
	// sel picks between the two drives, low for the internal one
	sel bool
	// headSelect is the SEL line, driven by the VIA port A bit 5. It is
	// part of the selector of the drive status lines.
	headSelect bool

	// q6 and q7 select what a read or a write reaches
	q6 bool
	q7 bool

	mode uint8

	// The two drives of the machine, the internal one and the external
	drives [driveCount]*drive
}

const (
	// driveCount is the drives the Macintosh Plus can have: the one inside
	// and one on the port at the back
	driveCount = 2

	driveInternal = 0
	driveExternal = 1
)

const (
	// iwmModeMask is the part of the mode register that reads back on the
	// status
	iwmModeMask uint8 = 0x1f

	// iwmStatusEnable is the drive enable, the bit the ROM waits on
	iwmStatusEnable uint8 = 1 << 5
	// iwmStatusSense is the selected status line of the drive. The lines
	// are active low, so a line pulled down is a zero here.
	iwmStatusSense uint8 = 1 << 7

	// iwmHandshakeUnderrun says the last byte written went out in time.
	// Nothing here can be late, so it is always set.
	iwmHandshakeUnderrun uint8 = 1 << 6
	// iwmHandshakeReady says the write buffer is empty and the next byte
	// can be handed over
	iwmHandshakeReady uint8 = 1 << 7
)

func newIwm(trace bool) *iwm {
	return &iwm{drives: [driveCount]*drive{
		newDrive("internal", trace),
		newDrive("external", trace),
	}}
}

// selected is the drive the SELECT line is pointing at
func (d *iwm) selected() *drive {
	if d.sel {
		return d.drives[driveExternal]
	}
	return d.drives[driveInternal]
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
		d.setStrobe(on)
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

/*
setStrobe drives LSTRB, which is how the machine writes to the drive rather
than reading from it. The three lines CA1, CA0 and SEL pick which of the four
control lines is meant and CA2 carries the value, and the drive takes it as
the strobe goes up.
*/
func (d *iwm) setStrobe(on bool) {
	rising := on && !d.lstrb
	d.lstrb = on

	if rising {
		d.selected().setControl(d.controlSelector(), d.ca2)
	}
}

// controlSelector is the control line being written, CA1, CA0 and SEL
func (d *iwm) controlSelector() uint8 {
	var selector uint8
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
applyHeadSelect picks which of the two heads of the drive reads. There is no
line of its own for it: the drive routes one head or the other to its read
data line according to which of the two the phase lines are addressing, and
the driver points them at one just before it starts moving bytes.

It is worked out here, as a byte is read or written, and not when a phase line
moves. Sony_AdrDisk sets the four lines one at a time, so on the way from one
register to another it passes through others, and a couple of those are the
read data pair: latching the head as the lines went by would take the head
from a state the driver was only passing through.
*/
func (d *iwm) applyHeadSelect() {
	switch d.senseSelector() {
	case senseReadData0:
		d.selected().setSide(0)
	case senseReadData1:
		d.selected().setSide(1)
	}
}

// read returns the register selected by Q6 and Q7
func (d *iwm) read() uint8 {
	switch {
	case !d.q6 && !d.q7:
		d.applyHeadSelect()
		return d.selected().readByte()
	case d.q6 && !d.q7:
		return d.status()
	case !d.q6 && d.q7:
		return d.handshake()
	default:
		return 0xff
	}
}

/*
write reaches the mode register while the drive is off and the disk while it
is on, both at Q6 and Q7 high. Which of the two is meant is the enable line,
as it is on the chip: the mode register can only be changed with the drive
powered down.
*/
func (d *iwm) write(value uint8) {
	if !d.q6 || !d.q7 {
		return
	}

	if d.enable {
		d.applyHeadSelect()
		d.selected().writeByte(value)
		return
	}

	d.mode = value & iwmModeMask
}

// status is the mode register on the low bits, the state of the enable, and
// the status line selected on the drive
func (d *iwm) status() uint8 {
	status := d.mode & iwmModeMask

	if d.enable {
		status |= iwmStatusEnable
	}
	if d.selected().sense(d.senseSelector()) {
		status |= iwmStatusSense
	}

	return status
}

/*
handshake says whether the next byte to write can be handed over. The top bit
is the one the driver polls and it comes from the disk, one byte at a time,
which is what paces a write to the speed the disk turns at.

With nothing to write to, the answer is that the buffer is free. A write that
waited for a drive that is not there would never finish.
*/
func (d *iwm) handshake() uint8 {
	drive := d.selected()

	if drive.canWrite() && !drive.writeReady() {
		return iwmHandshakeUnderrun
	}

	return iwmHandshakeReady | iwmHandshakeUnderrun
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

// tick turns the disks in both drives, whether or not they are the one
// selected: the machine can leave one spinning while it talks to the other
func (d *iwm) tick(cycles uint64) {
	for _, drive := range d.drives {
		drive.tick(cycles)
	}
}

// flush writes back everything the machine has left on the disks
func (d *iwm) flush() error {
	var firstErr error
	for _, drive := range d.drives {
		if err := drive.flush(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (d *iwm) reset() {
	d.ca0 = false
	d.ca1 = false
	d.ca2 = false
	d.lstrb = false
	d.enable = false
	d.sel = false
	d.headSelect = false
	d.q6 = false
	d.q7 = false
	d.mode = 0

	for _, drive := range d.drives {
		drive.reset()
	}
}

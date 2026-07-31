package izmac

import (
	"fmt"
	"sync/atomic"

	"github.com/ivanizag/izmac/storage"
)

/*
One of the two diskette drives, the mechanism rather than the controller: the
motor, the head and where it is, and the bytes going past it.

The Macintosh drive turns slower the further out the head is, in five bands of
sixteen tracks, so that a bit takes up the same length of track wherever it is
written and an outer track holds more of them. That is why a track is twelve
sectors near the middle and eight at the edge.

None of that has to be worked out here. A bit lasts the same time everywhere,
which is the whole point of the arrangement, so a byte does too, and a track
of twelve sectors simply takes half as long again to go round as one of eight.
Building a track out of a fixed number of bytes per sector gives the right
rotation for every band without the speeds appearing anywhere.

The number that does appear is how long a byte lasts, and it comes out even:
the disk carries 489.6 kilobits a second, which is 61200 bytes, and the
processor runs at 7.8336MHz, which is exactly 128 cycles for each of them.
*/

const (
	// cyclesPerDiskByte is how long a byte takes to go past the head
	cyclesPerDiskByte = 128

	// tachPulsesPerTurn is what the drive reports on its tachometer. The
	// 800K drive regulates its own speed and the Macintosh only watches,
	// but the line is polled and has to move.
	tachPulsesPerTurn = 60
)

// drive is one diskette drive
type drive struct {
	name string

	disk *storage.FloppyDisk

	// motorOn is the drive spinning, which the machine asks for and only
	// happens with a disk in place
	motorOn bool

	// stepToTrack0 is the direction a step takes the head, as the DIRTN
	// line was last latched: high steps back towards the track 0
	stepToTrack0 bool

	// track is where the head is, and side which of the two heads the
	// phase lines last selected
	track int
	side  int

	/*
		The track under the head, as bytes going round, and where it came
		from. It is read off the image when the head arrives and written
		back when it leaves, so that the machine rewriting one sector of it
		leaves the rest as it was.
	*/
	trackData   []uint8
	trackNumber int
	trackSide   int
	trackDirty  bool

	/*
		spinCycles is how long the disk has been turning, in processor
		cycles. The byte under the head and the tachometer both come out of
		it, so there is one thing to advance and nothing to keep in step.
	*/
	spinCycles uint64

	// byteReady says a byte has come round since the last one was taken.
	// It is what the machine polls, on the top bit of the data register
	// while reading and of the handshake while writing.
	byteReady bool

	/*
		published is what is in the drive, kept where a frontend can read
		it. The emulation runs on its own goroutine and a menu asking what
		to offer runs on another, so the answer is replaced whole as the
		drive changes rather than being read out of the drive itself.

		Nil is an empty drive.
	*/
	published atomic.Pointer[mountedDiskette]

	trace bool
}

// mountedDiskette is the published answer, replaced rather than changed
type mountedDiskette struct {
	image    string
	readOnly bool
}

func newDrive(name string, trace bool) *drive {
	return &drive{name: name, trace: trace}
}

// mounted is what a frontend sees in the drive
func (d *drive) mounted() (string, bool) {
	if m := d.published.Load(); m != nil {
		return m.image, m.readOnly
	}
	return "", false
}

// publish makes the state of the drive visible to a frontend
func (d *drive) publish() {
	if d.disk == nil {
		d.published.Store(nil)
		return
	}

	d.published.Store(&mountedDiskette{
		image:    d.disk.Name(),
		readOnly: d.disk.IsReadOnly(),
	})
}

// insert puts a diskette in the drive, taking out whatever was there
func (d *drive) insert(disk *storage.FloppyDisk) error {
	if err := d.eject(); err != nil {
		return err
	}

	d.disk = disk

	/*
		The head is left where it is, as it would be. The driver does not
		trust it either: it marks the track unknown when it sees a disk go
		in and recalibrates to the track 0 before reading anything.
	*/
	d.trackDirty = false
	d.trackData = nil
	d.publish()

	if d.trace {
		locked := ""
		if disk.IsReadOnly() {
			locked = ", locked"
		}
		fmt.Printf("Floppy %v: %v inserted, %v sides%v\n",
			d.name, disk.Name(), disk.Sides(), locked)
	}

	return nil
}

// eject writes back whatever is pending and takes the diskette out
func (d *drive) eject() error {
	if d.disk == nil {
		return nil
	}

	err := d.flush()

	if d.trace {
		fmt.Printf("Floppy %v: %v ejected\n", d.name, d.disk.Name())
	}

	d.disk = nil
	d.trackData = nil
	d.trackDirty = false
	d.motorOn = false
	d.publish()

	return err
}

// hasDisk tells whether there is a diskette in the drive
func (d *drive) hasDisk() bool {
	return d.disk != nil
}

// spinning tells whether the disk is turning, which needs a disk to turn
func (d *drive) spinning() bool {
	return d.motorOn && d.disk != nil
}

/*
tick advances the disk under the head. Everything that depends on where the
disk has got to is worked out from the one counter, so there is nothing here
to keep in step with anything else.
*/
func (d *drive) tick(cycles uint64) {
	if !d.spinning() {
		return
	}

	before := d.spinCycles / cyclesPerDiskByte
	d.spinCycles += cycles

	if d.spinCycles/cyclesPerDiskByte != before {
		d.byteReady = true
	}
}

// headPosition is the byte of the track under the head
func (d *drive) headPosition() int {
	if len(d.trackData) == 0 {
		return 0
	}
	return int(d.spinCycles / cyclesPerDiskByte % uint64(len(d.trackData)))
}

/*
readByte hands over the byte under the head, or zero when the next one has not
come round yet.

Zero is not a byte the encoding can produce, and the driver relies on that: it
reads the data register over and over and carries on only when the top bit is
set, which is how it waits without a clock of its own.
*/
func (d *drive) readByte() uint8 {
	d.loadTrack()

	if !d.byteReady || len(d.trackData) == 0 {
		return 0
	}
	d.byteReady = false

	return d.trackData[d.headPosition()]
}

// writeReady is the top bit of the handshake register, which says the last
// byte written has gone out and the next one can be handed over
func (d *drive) writeReady() bool {
	return d.byteReady && d.canWrite()
}

// canWrite tells whether there is anywhere for a byte to go
func (d *drive) canWrite() bool {
	return d.spinning() && !d.disk.IsReadOnly()
}

// writeByte puts a byte down under the head
func (d *drive) writeByte(value uint8) {
	d.loadTrack()

	if !d.canWrite() || len(d.trackData) == 0 {
		return
	}

	d.byteReady = false
	d.trackData[d.headPosition()] = value
	d.trackDirty = true
}

/*
loadTrack reads the track the head is over off the image, writing back the one
it was over before. It is called when a byte is asked for rather than when the
head moves, so that a seek across the disk costs one track read and not one
for every track it passed over.
*/
func (d *drive) loadTrack() {
	if d.disk == nil {
		return
	}
	if len(d.trackData) != 0 && d.trackNumber == d.track && d.trackSide == d.side {
		return
	}

	if err := d.flushTrack(); err != nil && d.trace {
		fmt.Printf("Floppy %v: %v\n", d.name, err)
	}

	d.trackData = nil

	if d.side >= d.disk.Sides() {
		// The far side of a single sided diskette, where there is nothing
		// written and nothing to read
		return
	}

	data, err := d.disk.ReadTrack(d.track, d.side)
	if err != nil {
		if d.trace {
			fmt.Printf("Floppy %v: %v\n", d.name, err)
		}
		return
	}

	d.trackData = data
	d.trackNumber = d.track
	d.trackSide = d.side

	if d.trace {
		fmt.Printf("Floppy %v: track %v side %v under the head, %v bytes\n",
			d.name, d.trackNumber, d.trackSide, len(d.trackData))
	}
}

/*
flushTrack takes what is on the track back apart into sectors and stores them
in the image. Whatever does not decode is left alone: the machine rewrites one
sector of a track at a time and the rest of what comes back is what izmac put
there in the first place.
*/
func (d *drive) flushTrack() error {
	if !d.trackDirty || d.disk == nil {
		return nil
	}
	d.trackDirty = false

	stored, err := d.disk.WriteTrack(d.trackNumber, d.trackSide, d.trackData)
	if err != nil {
		return err
	}

	if d.trace {
		fmt.Printf("Floppy %v: track %v side %v, %v sectors written\n",
			d.name, d.trackNumber, d.trackSide, stored)
	}

	return nil
}

// flush stores everything pending, on the track and in the image, which is
// what has to happen before the machine could be turned off
func (d *drive) flush() error {
	if d.disk == nil {
		return nil
	}

	if err := d.flushTrack(); err != nil {
		return err
	}

	return d.disk.Flush()
}

/*
The lines the drive reports, selected by CA2, CA1, CA0 and SEL in that order
from the high bit. They are active low, so a false is the condition the name
describes being true, with the two exceptions the hardware makes: SIDES is
high on a double sided drive and the new interface line is high on the 800K
one.

The names and the numbers are from SonyEqu.a of the ROM sources, which gives
them in its own order, as CA1, CA0, SEL and CA2. Converted, they land on the
table Inside Macintosh gives at III-35, which documents the 400K drive of the
earlier machines. The Plus has the 800K one and its driver asks about three
lines the book does not list: 13, 14 and 15 below.
*/
const (
	senseDirection    = 0
	senseDiskInPlace  = 1
	senseStepping     = 2
	senseWriteProtect = 3
	senseMotorOn      = 4
	senseTrack0       = 5
	senseTach         = 7
	senseReadData0    = 8
	senseReadData1    = 9
	senseSides        = 12

	// senseReady is ReadyAdr, low once the drive is up to speed. Sony_Seek
	// polls it a thousand times with a millisecond in between, so a drive
	// that never reports ready costs a second before the driver gives up.
	senseReady = 13

	// senseDriveInstalled is DrvExstAdr. The book numbers the drive present
	// line 15 for the 400K drive; the driver of the Plus asks at 14, and
	// asks at 15 for something else entirely.
	senseDriveInstalled = 14

	/*
		senseNewInterface is NewIntfAdr, and it is high on the 800K drive.
		The driver reads it once at startup, with SMI.B, and takes it as
		meaning the drive regulates its own speed.

		It decides everything that follows. Reported low, Sony_MakeSpdTbl
		calibrates the motor: it sets a pulse width through the low bytes of
		the sound buffer, times fifteen tachometer edges four times over at
		two settings, and interpolates a table for the five bands. Reported
		high, the whole routine is an 800ms wait and Sony_SetSpeed forces
		the pulse width to zero. One bit is the difference between having to
		emulate a speed servo and not.
	*/
	senseNewInterface = 15
)

/*
sense answers one of the lines. There is always a drive, whether or not there
is a diskette in it: the ROM walks the drive queue at DrvQHdr and stops on
purpose when it is empty, so a Macintosh with no drive never finishes booting.
*/
func (d *drive) sense(selector uint8) bool {
	switch selector {
	case senseDriveInstalled:
		return false
	case senseNewInterface:
		return true
	case senseSides:
		return true

	case senseDiskInPlace:
		return !d.hasDisk()
	case senseWriteProtect:
		return !(d.hasDisk() && d.disk.IsReadOnly())
	case senseMotorOn:
		return !d.spinning()
	case senseTrack0:
		return d.track != 0
	case senseDirection:
		// The direction as it was latched, which is not inverted: the
		// driver writes a one to send the head back towards the track 0
		return d.stepToTrack0

	case senseStepping:
		// A step is done as soon as it is asked for, so the line is never
		// found busy. The driver reads it before stepping to see that the
		// head is free and after to see that it arrived.
		return true

	case senseReady:
		// The one line where the driver waits a whole second before
		// giving up, so it matters that it comes up
		return !d.spinning()

	case senseTach:
		return d.tach()

	case senseReadData0, senseReadData1:
		// The head has been selected by the phase lines reaching here, and
		// the level itself says nothing
		return true
	}

	return true
}

/*
tach is the tachometer, sixty pulses a turn. A turn is the track going past
once, so its length in bytes is how long it takes, and the pulse falls out of
the same counter as everything else.
*/
func (d *drive) tach() bool {
	if !d.spinning() || len(d.trackData) == 0 {
		return true
	}

	cyclesPerTurn := uint64(len(d.trackData)) * cyclesPerDiskByte
	cyclesPerHalfPulse := cyclesPerTurn / (2 * tachPulsesPerTurn)
	if cyclesPerHalfPulse == 0 {
		return true
	}

	return d.spinCycles/cyclesPerHalfPulse%2 == 0
}

/*
The lines the machine drives, selected by CA1, CA0 and SEL, with CA2 carrying
the value. They are latched when the strobe goes up.

The numbering is the one in SonyEqu.a with the value bit dropped: MtrOnAdr is
8 and MtrOffAdr 9, so the motor is the register 4 and is turned on by a zero.
*/
const (
	controlDirection = 0
	controlStep      = 2
	controlMotor     = 4
	controlEject     = 6
)

// setControl latches one of the lines the machine drives
func (d *drive) setControl(selector uint8, value bool) {
	switch selector {
	case controlDirection:
		// A zero steps outwards, towards the higher numbered tracks
		d.stepToTrack0 = value

	case controlStep:
		if !value {
			d.step()
		}

	case controlMotor:
		d.setMotor(!value)

	case controlEject:
		// The only one that is not active low: the disk comes out when
		// the line is driven high
		if value {
			if err := d.eject(); err != nil && d.trace {
				fmt.Printf("Floppy %v: %v\n", d.name, err)
			}
		}
	}
}

// step moves the head one track. The stepper can not be driven past either
// end, which is what stops the driver's recalibration.
func (d *drive) step() {
	if d.stepToTrack0 {
		if d.track > 0 {
			d.track--
		}
		return
	}

	if d.track < storage.TracksPerSide-1 {
		d.track++
	}
}

/*
setMotor starts or stops the disk. Stopping it writes back whatever the
machine left on the track, which is the moment to do it: the driver turns the
motor off a few seconds after it has finished with the disk, so the image on
the host follows what the Macintosh believes it has saved.
*/
func (d *drive) setMotor(on bool) {
	if d.motorOn == on {
		return
	}
	d.motorOn = on

	if on {
		d.byteReady = false
		return
	}

	if err := d.flush(); err != nil && d.trace {
		fmt.Printf("Floppy %v: %v\n", d.name, err)
	}
}

// setSide picks which of the two heads reads, which the phase lines do when
// they select one of the two read data lines
func (d *drive) setSide(side int) {
	d.side = side
}

func (d *drive) reset() {
	d.setMotor(false)
	d.stepToTrack0 = false
	d.side = 0
	d.byteReady = false

	// The head stays where it is, as it would over a reset, and the driver
	// recalibrates before trusting it
}

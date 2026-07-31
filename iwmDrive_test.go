package izmac

import (
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/ivanizag/izmac/storage"
)

/*
The drive and the controller together, driven the way the Sony driver of the
ROM drives them: through the soft switches, polling the data register, and
finding the sectors by the marks that announce them rather than by asking
where they are.

The driver below is written from plus/resources/res_drvr_sony.s and knows
nothing about how izmac lays a track out. What it gets back is a stream of
bytes, which is what a real machine gets back.
*/

// The rest of the soft switches this file needs, alongside the ones in
// iwm_test.go
const (
	iwmSwCa0H   = 1
	iwmSwCa1L   = 2
	iwmSwCa2L   = 4
	iwmSwCa2H   = 5
	iwmSwLstrbL = 6
	iwmSwLstrbH = 7
	iwmSwSelH   = 11
)

/*
pollCycles is how long the driver's polling loop takes to come round: a read
of the data register and a branch back, about eighteen cycles. A byte comes
past the head every hundred and twenty eight, so it looks about seven times
before one arrives.
*/
const pollCycles = 18

// The drive registers of SonyEqu.a, in its own CA1, CA0, SEL, CA2 order
const (
	sonyDirectionToZero  = 1
	sonyDirectionOutward = 0
	sonyReadData0        = 1
	sonyReadData1        = 3
	sonyStep             = 4
	sonyMotorOn          = 8
	sonyMotorOff         = 9
	sonyDiskInPlace      = 2
	sonyWriteProtect     = 6
	sonyTrack0           = 10
)

// sonyDriver is the machine's side of the wire, working the controller the
// way the ROM does
type sonyDriver struct {
	t   *testing.T
	iwm *iwm
}

func newSonyDriver(t *testing.T, d *iwm) *sonyDriver {
	t.Helper()
	return &sonyDriver{t: t, iwm: d}
}

// touch works a soft switch, which the machine does by reading the address
func (s *sonyDriver) touch(reg uint32) {
	s.iwm.peek(iwmAddress(reg))
}

/*
address puts the phase lines where a status or control line is selected.
Sony_AdrDisk takes the register in the order CA1, CA0, SEL, CA2, and the SEL
of it is the VIA port A bit 5 rather than a switch of the controller.
*/
func (s *sonyDriver) address(register uint8) {
	if register&8 != 0 {
		s.touch(iwmSwCa1L + 1)
	} else {
		s.touch(iwmSwCa1L)
	}
	if register&4 != 0 {
		s.touch(iwmSwCa0H)
	} else {
		s.touch(iwmSwCa0L)
	}
	s.iwm.setHeadSelect(register&2 != 0)
	if register&1 != 0 {
		s.touch(iwmSwCa2H)
	} else {
		s.touch(iwmSwCa2L)
	}
}

// writeRegister strobes a control line, which is how the motor is started and
// the head stepped
func (s *sonyDriver) writeRegister(register uint8) {
	s.address(register)
	s.touch(iwmSwLstrbH)
	s.touch(iwmSwLstrbL)
}

// readRegister returns a status line, on the top bit of the status register
func (s *sonyDriver) readRegister(register uint8) bool {
	s.address(register)

	s.touch(iwmSwQ6H)
	status := s.iwm.peek(iwmAddress(iwmSwQ7L))
	s.touch(iwmSwQ6L)

	return status&0x80 != 0
}

// selectDrive points the controller at one of the two drives and powers it up
func (s *sonyDriver) selectDrive(drive int) {
	if drive == driveExternal {
		s.touch(iwmSwSelH)
	} else {
		s.touch(iwmSwSelL)
	}
	s.touch(iwmSwEnblH)
}

// start gets the selected drive turning
func (s *sonyDriver) start() {
	s.writeRegister(sonyMotorOn)
}

// stop stops it, which is what writes a changed diskette back to the host
func (s *sonyDriver) stop() {
	s.writeRegister(sonyMotorOff)
}

// recalibrate steps the head back to the track 0, which the driver does
// whenever it does not trust where the head is
func (s *sonyDriver) recalibrate() {
	s.t.Helper()

	s.writeRegister(sonyDirectionToZero)
	for step := 0; step <= storage.TracksPerSide; step++ {
		// The line is active low, so at the track 0 it reads low
		if !s.readRegister(sonyTrack0) {
			return
		}
		s.writeRegister(sonyStep)
	}

	s.t.Fatal("the head never reached the track 0")
}

// seek steps the head out to a track, from wherever it was
func (s *sonyDriver) seek(track int) {
	s.recalibrate()

	s.writeRegister(sonyDirectionOutward)
	for step := 0; step < track; step++ {
		s.writeRegister(sonyStep)
	}
}

// selectSide points the phase lines at one of the two read data lines, which
// is how the head is chosen, and leaves the data register selected
func (s *sonyDriver) selectSide(side int) {
	if side == 0 {
		s.address(sonyReadData0)
	} else {
		s.address(sonyReadData1)
	}
	s.touch(iwmSwQ6L)
	s.touch(iwmSwQ7L)
}

/*
nextByte polls the data register until a byte has come round, which is what
the driver does: it reads over and over and carries on when the top bit is
set. The disk turns while it waits.
*/
func (s *sonyDriver) nextByte() uint8 {
	s.t.Helper()

	for poll := 0; poll < 1000; poll++ {
		s.iwm.tick(pollCycles)
		if value := s.iwm.peek(iwmAddress(iwmSwQ6L)); value&0x80 != 0 {
			return value
		}
	}

	s.t.Fatal("no byte came past the head, the disk is not turning")
	return 0
}

// readBytes takes a run of bytes off the track
func (s *sonyDriver) readBytes(count int) []uint8 {
	s.t.Helper()

	stream := make([]uint8, count)
	for i := range stream {
		stream[i] = s.nextByte()
	}
	return stream
}

/*
writeBytes puts a run of bytes down on the track, the way Sony_WrData does:
Q6 high and Q7 high is the disk while the drive is powered, and between bytes
the handshake at Q6 low is polled until the last one has gone out.

The first byte goes to Q7 high, which is what turns the write on.
*/
func (s *sonyDriver) writeBytes(stream []uint8) {
	s.t.Helper()

	s.touch(iwmSwQ6H)
	for i, value := range stream {
		s.waitForWrite()

		if i == 0 {
			s.iwm.poke(iwmAddress(iwmSwQ7H), value)
		} else {
			s.iwm.poke(iwmAddress(iwmSwQ6H), value)
		}
	}

	// Back to reading, as the driver leaves it
	s.touch(iwmSwQ6L)
	s.touch(iwmSwQ7L)
}

// waitForWrite polls the handshake until the buffer is free
func (s *sonyDriver) waitForWrite() {
	s.t.Helper()

	for poll := 0; poll < 1000; poll++ {
		s.iwm.tick(pollCycles)
		if s.iwm.peek(iwmAddress(iwmSwQ6L))&0x80 != 0 {
			return
		}
	}

	s.t.Fatal("the write handshake never came ready")
}

// turnBytes is a whole turn of a track and one sector more, so that a sector
// straddling the point the head started at is met complete
func turnBytes(track int) int {
	return (storage.SectorsInTrack(track) + 1) * 800
}

// buildDisketteImage writes a diskette image of random bytes, so that a
// sector read from the wrong place is noticed
func buildDisketteImage(t *testing.T, name string, sides int, seed int64) (string, []uint8) {
	t.Helper()

	data := make([]uint8, sides*400*1024)
	random := rand.New(rand.NewSource(seed))
	for i := range data {
		data[i] = uint8(random.Intn(256))
	}

	filename := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(filename, data, 0666); err != nil {
		t.Fatal(err)
	}

	return filename, data
}

// mountDiskette puts a fresh image in one of the drives
func mountDiskette(t *testing.T, d *iwm, drive int, name string, seed int64) []uint8 {
	t.Helper()

	filename, data := buildDisketteImage(t, name, 2, seed)

	disk, err := storage.NewFloppyDisk(filename, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.drives[drive].insert(disk); err != nil {
		t.Fatal(err)
	}

	return data
}

/*
The whole path out: an image on the host, encoded into a track, turned under
the head at the rate the disk turns, polled a byte at a time through the data
register, and decoded back from the stream that came off it.

One track of each of the five bands and both sides, since the sector count and
the side bit of the address field both change across them.
*/
func TestTheSectorsComeOffTheDiskTheWayTheDriverReadsThem(t *testing.T) {
	d := newIwm(false)
	image := mountDiskette(t, d, driveInternal, "read.dsk", 7)

	driver := newSonyDriver(t, d)
	driver.selectDrive(driveInternal)
	driver.start()

	for _, track := range []int{0, 16, 40, 79} {
		driver.seek(track)

		for side := 0; side < 2; side++ {
			driver.selectSide(side)

			sectors := storage.DecodeTrack(driver.readBytes(turnBytes(track)))
			wanted := storage.SectorsInTrack(track)
			if len(sectors) != wanted {
				t.Fatalf("the track %v side %v gave %v sectors, wanted %v",
					track, side, len(sectors), wanted)
			}

			for sector, got := range sectors {
				block := storage.BlockOf(track, side, sector, 2)
				want := image[block*storage.BlockSize : (block+1)*storage.BlockSize]

				for i := range want {
					if got[storage.TagSize+i] != want[i] {
						t.Fatalf("the track %v side %v sector %v differs at %v, "+
							"$%02x for $%02x", track, side, sector, i,
							got[storage.TagSize+i], want[i])
					}
				}
			}
		}
	}
}

/*
The disk has to turn at the speed a real one does, or the driver's timeouts
and its sector times mean nothing. A byte every 128 cycles over a track of
twelve sectors comes to just under 400 revolutions a minute, which is what the
innermost band of a Macintosh diskette spins at.
*/
func TestTheDiskTurnsAtTheSpeedOfItsBand(t *testing.T) {
	d := newIwm(false)
	mountDiskette(t, d, driveInternal, "speed.dsk", 3)

	driver := newSonyDriver(t, d)
	driver.selectDrive(driveInternal)
	driver.start()

	for _, c := range []struct {
		track int
		rpm   float64
	}{{0, 394}, {16, 429}, {32, 472}, {48, 525}, {64, 590}} {
		driver.seek(c.track)
		driver.selectSide(0)
		driver.nextByte() // Which is what brings the track under the head

		turn := float64(len(d.drives[driveInternal].trackData)) * cyclesPerDiskByte
		rpm := 60.0 * CPUClockMhz * 1_000_000 / turn

		if rpm < c.rpm*0.99 || rpm > c.rpm*1.01 {
			t.Errorf("the track %v turns at %.1f rpm, wanted about %v",
				c.track, rpm, c.rpm)
		}
	}
}

/*
The way back in, and the way a disk copier does it: a track is read off one
diskette and written to the other, a byte at a time through the controller,
and the sectors have to arrive.

Doing it this way rather than encoding a sector in the test is the point. The
bytes written are bytes that came off a drive, so nothing here has an opinion
about what a track should look like, and the two drives and the line that
picks between them are exercised on the way.
*/
func TestATrackWrittenThroughTheControllerReachesTheImage(t *testing.T) {
	const (
		track = 3
		side  = 1
	)

	d := newIwm(false)
	source := mountDiskette(t, d, driveInternal, "source.dsk", 11)
	mountDiskette(t, d, driveExternal, "target.dsk", 12)

	driver := newSonyDriver(t, d)

	// Read the track off the diskette in the internal drive
	driver.selectDrive(driveInternal)
	driver.start()
	driver.seek(track)
	driver.selectSide(side)

	turn := len(d.drives[driveInternal].trackData)
	if turn == 0 {
		t.Fatal("the track of the source diskette never came under the head")
	}
	stream := driver.readBytes(turn)

	// And write it to the one in the external drive
	driver.selectDrive(driveExternal)
	driver.start()
	driver.seek(track)
	driver.selectSide(side)
	driver.nextByte()

	driver.writeBytes(stream)
	driver.stop()

	target := d.drives[driveExternal].disk
	if target.IsModified() {
		t.Error("the diskette was not written back when the motor stopped")
	}

	// What is on the host now has to be what came off the other diskette
	written, err := os.ReadFile(target.Name())
	if err != nil {
		t.Fatal(err)
	}

	for sector := 0; sector < storage.SectorsInTrack(track); sector++ {
		block := storage.BlockOf(track, side, sector, 2)
		from := block * storage.BlockSize

		for i := 0; i < storage.BlockSize; i++ {
			if written[from+i] != source[from+i] {
				t.Fatalf("the sector %v of the copied track differs at %v, "+
					"$%02x for $%02x", sector, i, written[from+i], source[from+i])
			}
		}
	}

	// And no other track moved, which a write that landed in the wrong
	// place would show up as
	otherBlock := storage.BlockOf(track+1, side, 0, 2) * storage.BlockSize
	unchanged := mountedImageOf(t, target.Name())
	for i := 0; i < storage.BlockSize; i++ {
		if written[otherBlock+i] != unchanged[otherBlock+i] {
			t.Fatalf("the next track was disturbed at %v", i)
		}
	}
}

// mountedImageOf reads an image back off the host
func mountedImageOf(t *testing.T, filename string) []uint8 {
	t.Helper()

	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

/*
A locked diskette is one the machine can see and can not change. The write
protect line says so, and the drive refuses the bytes as well: the driver is
entitled to ignore the line and try, and a file the host will not let izmac
write is not something to find out about halfway through a track.
*/
func TestALockedDisketteRefusesWrites(t *testing.T) {
	filename, _ := buildDisketteImage(t, "locked.dsk", 2, 5)

	disk, err := storage.NewFloppyDisk(filename, true)
	if err != nil {
		t.Fatal(err)
	}

	d := newIwm(false)
	if err := d.drives[driveInternal].insert(disk); err != nil {
		t.Fatal(err)
	}

	driver := newSonyDriver(t, d)
	driver.selectDrive(driveInternal)
	driver.start()
	driver.selectSide(0)
	driver.nextByte()

	// The line is active low, so a locked diskette pulls it down
	if driver.readRegister(sonyWriteProtect) {
		t.Error("a locked diskette does not report the write protect line")
	}

	drive := d.drives[driveInternal]
	before := append([]uint8{}, drive.trackData...)

	driver.selectSide(0)
	driver.writeBytes([]uint8{0x96, 0x97, 0x9a})

	for i := range before {
		if drive.trackData[i] != before[i] {
			t.Fatalf("a byte reached the track of a locked diskette at %v", i)
		}
	}
}

/*
The disk in place line is what the driver polls to notice a diskette going in
or coming out, and the eject the machine asks for has to take it out.
*/
func TestTheDriveReportsWhatIsInIt(t *testing.T) {
	d := newIwm(false)
	driver := newSonyDriver(t, d)
	driver.selectDrive(driveInternal)

	// Active low, so an empty drive leaves it high
	if !driver.readRegister(sonyDiskInPlace) {
		t.Error("an empty drive reports a diskette in place")
	}

	mountDiskette(t, d, driveInternal, "inout.dsk", 9)
	if driver.readRegister(sonyDiskInPlace) {
		t.Error("a diskette in the drive is not reported in place")
	}

	// The eject line is the one control line that is not active low
	driver.writeRegister(13)
	if !driver.readRegister(sonyDiskInPlace) {
		t.Error("the diskette is still in place after being ejected")
	}
	if d.drives[driveInternal].hasDisk() {
		t.Error("the drive still holds a diskette after the eject")
	}
}

package component

import (
	"fmt"
	"os"
	"time"
)

/*
AppleRTC is the real time clock of the Macintosh, reached through three bits
of the VIA port B. Inside Macintosh calls it a custom chip; the ROM
disassembly names it a 58321A: rTCData on 0, rTCClk on 1 and rTCEnb on 2. It holds a four byte
counter of the seconds and twenty bytes of parameter RAM, and it is not
optional: the ROM reads the parameter RAM to find out what to boot from, and
without an answer it never looks at the SCSI bus at all.

The protocol, from Inside Macintosh volume III, pages III-37 and III-38. The
enable line is pulled low for the whole transaction and raising it aborts
whatever was in progress. Bytes go over the data line high bit first. The
processor always sends a command byte first; its top bit is 1 to read and 0
to write, and its low two bits are always 01:

	z0000001, z0000101, z0001001, z0001101   the four seconds bytes, low first
	00110001                                 test register, write only
	00110101                                 write protect register, write only
	z010aa01                                 parameter RAM $10 to $13
	z1aaaa01                                 parameter RAM $00 to $0f

A write command is followed by eight more bits from the processor, clocked in
on the rising edge of the clock line. A read command is answered by the chip,
which puts each bit on the data line as the clock falls.
*/
type AppleRTC struct {
	pram    [pramSize]uint8
	seconds uint32

	/*
		wallClock takes the counter from the host on every read instead of
		from the seconds register, so a machine that has been running for a
		while, or accelerated, or paused, still reads the right time.

		It costs what the register bought: the counter can no longer be
		set, so the date in the Control Panel does not take. The parameter
		RAM is untouched by this and still saves.
	*/
	wallClock bool

	// writeProtected blocks every write, including the parameter RAM
	writeProtected bool

	// The state of the three lines
	enabled bool
	clock   bool

	// The byte being shifted in and how many bits of it have arrived
	shiftIn  uint8
	bitCount int

	// command is the byte that started the transaction, once it is whole
	command    uint8
	hasCommand bool

	// shiftOut is what the chip is answering, sending while it has bits
	shiftOut uint8
	sending  bool
	outBit   bool

	pramFile string
}

const (
	// pramSize is the parameter RAM of the Macintosh Plus, twenty bytes
	pramSize = 20

	// spConfigBothSerial is the SPConfig byte of the parameter RAM with both
	// ports carrying a serial device, port A on the high nibble and port B
	// on the low one
	spConfigBothSerial uint8 = 0x22

	rtcCommandRead uint8 = 1 << 7

	/*
		A command names a register on its middle bits, with 01 always at
		the bottom. The bit 6 picks the sixteen bytes of parameter RAM,
		and below that the bits 5 and 4 pick the group and the bits 3 and
		2 the register inside it.
	*/
	rtcCommandPramLow uint8 = 0x40 // The bit 6, the sixteen byte block
	rtcCommandGroup   uint8 = 0x30 // The bits 5 and 4
	rtcCommandIndex   uint8 = 0x0c // The bits 3 and 2

	// The low two bits are 01 on every command there is
	rtcCommandTailMask uint8 = 0x03
	rtcCommandTail     uint8 = 0x01

	rtcGroupSeconds      uint8 = 0x00
	rtcGroupSecondsAgain uint8 = 0x10
	rtcGroupPramHigh     uint8 = 0x20
	rtcGroupControl      uint8 = 0x30

	rtcIndexTest    uint8 = 0x00
	rtcIndexProtect uint8 = 0x04

	rtcCommandSeconds  uint8 = 0x01
	rtcCommandPramHigh uint8 = 0x21
	rtcCommandTest     uint8 = 0x31
	rtcCommandProtect  uint8 = 0x35

	// rtcProtectEnable is the bit of the write protect register that locks
	// the chip
	rtcProtectEnable uint8 = 1 << 7
)

// The kinds of register a command can reach
type rtcTarget int

const (
	rtcTargetNone rtcTarget = iota
	rtcTargetSeconds
	rtcTargetPram
	rtcTargetTest
	rtcTargetProtect
)

/*
The command byte builders. A command names a register and says whether it is
being read or written, and its shape is fiddly enough that spelling it out in
hex at every call is a good way to reach the wrong register.
*/

// secondsCommand reaches one of the four bytes of the counter, the low order
// one being the index 0
func secondsCommand(index int, read bool) uint8 {
	return withDirection(uint8(index&0x03)<<2|rtcCommandSeconds, read)
}

// pramCommand reaches a byte of the parameter RAM. The sixteen low ones and
// the four above them are addressed differently.
func pramCommand(address int, read bool) uint8 {
	if address >= 0x10 {
		return withDirection(uint8(address&0x03)<<2|rtcCommandPramHigh, read)
	}
	return withDirection(uint8(address&0x0f)<<2|rtcCommandPramLow|0x01, read)
}

func withDirection(command uint8, read bool) uint8 {
	if read {
		return command | rtcCommandRead
	}
	return command
}

/*
NewAppleRTC returns a clock started at the time of the host. With wallClock it
goes back to the host on every read instead of keeping its own count, which
never drifts but can not be set.
*/
func NewAppleRTC(pramFile string, wallClock bool) *AppleRTC {
	r := &AppleRTC{
		pram:      defaultPram(),
		pramFile:  pramFile,
		wallClock: wallClock,
		seconds:   macSeconds(time.Now()),
	}
	r.loadPram()
	return r
}

/*
macSeconds returns a time as the Macintosh counts it, the seconds since
midnight of the first of January 1904, local time.
*/
func macSeconds(t time.Time) uint32 {
	epoch := time.Date(1904, 1, 1, 0, 0, 0, 0, time.Local)
	return uint32(t.Sub(epoch) / time.Second)
}

// TickSecond advances the counter, called once a second of emulated time. A
// clock that reads the host has nothing to advance.
func (r *AppleRTC) TickSecond() {
	if r.wallClock {
		return
	}
	r.seconds++
}

/*
counter is the four byte seconds register the chip answers reads with.

Taking it from the host on every read is what a real chip does too: its
counter ticks on its own while the processor is reading it, and the ROM knows
it. The ROM reads the four bytes twice, through the two groups the counter
answers to, and compares the halves to catch a tick landing in the middle of
the read; when they differ it reads again.
*/
func (r *AppleRTC) counter() uint32 {
	if r.wallClock {
		return macSeconds(time.Now())
	}
	return r.seconds
}

/*
SetLines takes the state of the three lines after every write to the port B.
The processor drives the data line while it is sending, which the data
direction register says, and lets go of it to read the answer.
*/
func (r *AppleRTC) SetLines(enabled bool, clock bool, data bool, driving bool) {
	if !enabled {
		if r.enabled {
			r.endTransaction()
		}
		r.enabled = false
		r.clock = clock
		return
	}

	if !r.enabled {
		r.beginTransaction()
	}
	r.enabled = true

	if clock == r.clock {
		return
	}
	rising := clock && !r.clock
	r.clock = clock

	if r.sending {
		// The chip puts the next bit out as the clock falls
		if !rising {
			r.shiftOutBit()
		}
		return
	}

	if rising && driving {
		r.shiftInBit(data)
	}
}

// DataOut is the level the chip holds the data line at. It only drives the
// line while answering, and an undriven line reads high.
func (r *AppleRTC) DataOut() bool {
	if !r.sending {
		return true
	}
	return r.outBit
}

func (r *AppleRTC) beginTransaction() {
	r.shiftIn = 0
	r.bitCount = 0
	r.hasCommand = false
	r.sending = false
	r.outBit = true
}

func (r *AppleRTC) endTransaction() {
	r.sending = false
	r.hasCommand = false
	r.bitCount = 0
}

// shiftInBit takes one bit from the processor, completing either the command
// or the byte that follows a write command
func (r *AppleRTC) shiftInBit(bit bool) {
	r.shiftIn <<= 1
	if bit {
		r.shiftIn |= 1
	}
	r.bitCount++

	if r.bitCount < 8 {
		return
	}

	value := r.shiftIn
	r.shiftIn = 0
	r.bitCount = 0

	if !r.hasCommand {
		r.command = value
		r.hasCommand = true
		r.startCommand()
		return
	}

	r.writeRegister(value)
	r.hasCommand = false
}

// shiftOutBit presents the next bit of the answer, high bit first. The
// processor reads the line after the clock falls, so the chip has to keep
// holding the last bit until an edge past the eighth: letting go in the same
// call that presents it loses that bit to the undriven line.
func (r *AppleRTC) shiftOutBit() {
	if r.bitCount >= 8 {
		r.sending = false
		r.hasCommand = false
		r.bitCount = 0
		return
	}

	r.outBit = r.shiftOut&0x80 != 0
	r.shiftOut <<= 1
	r.bitCount++
}

// startCommand either loads the answer to a read or waits for the byte of a
// write
func (r *AppleRTC) startCommand() {
	if r.command&rtcCommandRead == 0 {
		// A write, the value is the next byte in
		return
	}

	r.shiftOut = r.readRegister()
	r.sending = true
	r.bitCount = 0
	r.outBit = r.shiftOut&0x80 != 0
}

/*
decodeCommand says which register a command reaches. The parameter RAM is
split in two: sixteen bytes reached with the bit 6 set and four more with a
group of their own.

The seconds answer to two groups rather than one, because the bit 4 is not
decoded for them. That is not a curiosity: the ROM reads the counter by
stepping down from $9d, which takes it through the four registers with that
bit set and then the four with it clear, and it compares the two halves to be
sure it did not catch the counter in the middle of a tick. A chip that
answers only one of the groups returns four zeros for the other half, the
comparison fails, and after one retry the ROM gives up with a clock read
error and leaves the time at the epoch.
*/
func (r *AppleRTC) decodeCommand() (rtcTarget, int) {
	command := r.command &^ rtcCommandRead

	if command&rtcCommandTailMask != rtcCommandTail {
		return rtcTargetNone, 0
	}

	if command&rtcCommandPramLow != 0 {
		return rtcTargetPram, int(command>>2) & 0x0f
	}

	index := int(command&rtcCommandIndex) >> 2

	switch command & rtcCommandGroup {
	case rtcGroupSeconds, rtcGroupSecondsAgain:
		return rtcTargetSeconds, index
	case rtcGroupPramHigh:
		return rtcTargetPram, 0x10 + index
	case rtcGroupControl:
		switch command & rtcCommandIndex {
		case rtcIndexTest:
			return rtcTargetTest, 0
		case rtcIndexProtect:
			return rtcTargetProtect, 0
		}
	}

	return rtcTargetNone, 0
}

func (r *AppleRTC) readRegister() uint8 {
	target, index := r.decodeCommand()

	switch target {
	case rtcTargetSeconds:
		// The low order byte is the register 0
		return uint8(r.counter() >> (8 * index))
	case rtcTargetPram:
		if index < len(r.pram) {
			return r.pram[index]
		}
	}

	// The test and write protect registers are write only, and the book
	// says not to read them
	return 0
}

func (r *AppleRTC) writeRegister(value uint8) {
	target, index := r.decodeCommand()

	if target == rtcTargetProtect {
		r.writeProtected = value&rtcProtectEnable != 0
		return
	}

	if r.writeProtected {
		return
	}

	switch target {
	case rtcTargetSeconds:
		if r.wallClock {
			// The host owns the time, there is nothing to set
			return
		}
		shift := 8 * index
		r.seconds = r.seconds&^(0xff<<shift) | uint32(value)<<shift
	case rtcTargetPram:
		if index < len(r.pram) && r.pram[index] != value {
			r.pram[index] = value
			r.savePram()
		}
	case rtcTargetTest:
		// Nothing to do, the test bits only matter to the hardware
	}
}

/*
defaultPram is what a machine with no saved parameter RAM starts from. It is
byte for byte what the ROM writes for itself when it finds the contents
invalid, with one difference: the two serial ports are marked as in use for a
serial device instead of free.

That difference keeps AppleTalk off. A System that finds a free port takes it
for AppleTalk, and the LocalTalk driver then programs the SCC for a frame and
waits, without a timeout, for the interrupt that says the frame went out. The
SCC here is the asynchronous side of the chip and no more, so that interrupt
never comes and the startup hangs: System 7.5 stops on its Starting up
screen. Leaving the ports spoken for is what a Macintosh with AppleTalk
turned off in the Chooser looks like, and it boots.

Marking them for a serial device rather than free is also what lets a printer
work: the serial driver takes a port already spoken for as one it may open,
which is the same thing a Macintosh with a printer on it does.

	$00 SPValid   $a8, the byte that says the rest is worth reading
	$01 SPATalkA  the AppleTalk node hints, of no use with it off
	$02 SPATalkB
	$03 SPConfig  the port use, port A on the high nibble and port B on
	              the low one: 0 free, 1 AppleTalk, 2 a serial device
	$04 SPPortA   the two port configurations, 9600 baud and 8N1
	$06 SPPortB
	$08 SPAlarm   the alarm time, never
	$0c SPFont    the application font
	$0e SPKbd     the keyboard repeat rate and threshold
	$0f SPPrint   the printer
	$10 SPVolCtl  the speaker volume
	$11 SPClikCaret  the double click and caret blink times
	$12 SPMisc1
	$13 SPMisc2   the mouse scaling and the disk to start from
*/
func defaultPram() [pramSize]uint8 {
	return [pramSize]uint8{
		0xa8, 0x00, 0x00, spConfigBothSerial,
		0xcc, 0x0a, 0xcc, 0x0a,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x02, 0x63, 0x00,
		0x03, 0x88, 0x00, 0x4c,
	}
}

// loadPram reads the parameter RAM saved by a previous run, leaving the
// defaults in place when there is nothing to read. A missing or unusable file
// is not an error: it is a machine that has not run before, or one whose
// battery went flat.
func (r *AppleRTC) loadPram() {
	if r.pramFile == "" {
		return
	}

	data, err := os.ReadFile(r.pramFile)
	if err != nil || len(data) != pramSize {
		return
	}
	copy(r.pram[:], data)
}

func (r *AppleRTC) savePram() {
	if r.pramFile == "" {
		return
	}

	err := os.WriteFile(r.pramFile, r.pram[:], 0o600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: can not save the parameter RAM: %v\n", err)
		r.pramFile = ""
	}
}

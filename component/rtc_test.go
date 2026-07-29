package component

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

/*
The tests drive the three lines the way the ROM does, which is the only way
to know the state machine is right: pull the enable low, clock the command
out a bit at a time high bit first, then either clock a byte in or let go of
the data line and clock the answer out.
*/

// sendByte clocks a byte from the processor into the chip
func sendByte(r *AppleRTC, value uint8) {
	for bit := 7; bit >= 0; bit-- {
		data := value&(1<<bit) != 0
		r.SetLines(true, false, data, true)
		r.SetLines(true, true, data, true)
	}
}

// receiveByte lets go of the data line and clocks the answer out
func receiveByte(r *AppleRTC) uint8 {
	var value uint8
	for bit := 0; bit < 8; bit++ {
		r.SetLines(true, false, true, false)
		value <<= 1
		if r.DataOut() {
			value |= 1
		}
		r.SetLines(true, true, true, false)
	}
	return value
}

func startTransaction(r *AppleRTC) {
	r.SetLines(false, true, true, false)
	r.SetLines(true, true, true, true)
}

func endTransaction(r *AppleRTC) {
	r.SetLines(false, true, true, false)
}

// writeRtc runs a whole write transaction
func writeRtc(r *AppleRTC, command uint8, value uint8) {
	startTransaction(r)
	sendByte(r, command)
	sendByte(r, value)
	endTransaction(r)
}

// readRtc runs a whole read transaction
func readRtc(r *AppleRTC, command uint8) uint8 {
	startTransaction(r)
	sendByte(r, command)
	value := receiveByte(r)
	endTransaction(r)
	return value
}

func TestTheParameterRamHoldsWhatIsWritten(t *testing.T) {
	r := NewAppleRTC("")

	// The sixteen bytes reached with the bit 6 set, $00 to $0f
	for address := 0; address < 16; address++ {
		command := pramCommand(address, false)
		writeRtc(r, command, uint8(0xa0+address))
	}
	for address := 0; address < 16; address++ {
		command := pramCommand(address, true)
		if got := readRtc(r, command); got != uint8(0xa0+address) {
			t.Errorf("the parameter RAM $%02x reads $%02x, wanted $%02x",
				address, got, 0xa0+address)
		}
	}

	// And the four with a group of their own, $10 to $13
	for address := 0; address < 4; address++ {
		command := pramCommand(0x10+address, false)
		writeRtc(r, command, uint8(0x50+address))
	}
	for address := 0; address < 4; address++ {
		command := pramCommand(0x10+address, true)
		if got := readRtc(r, command); got != uint8(0x50+address) {
			t.Errorf("the parameter RAM $%02x reads $%02x, wanted $%02x",
				0x10+address, got, 0x50+address)
		}
	}
}

func TestTheSecondsAreReadLowByteFirst(t *testing.T) {
	r := NewAppleRTC("")
	r.seconds = 0x12345678

	for index, wanted := range []uint8{0x78, 0x56, 0x34, 0x12} {
		command := secondsCommand(index, true)
		if got := readRtc(r, command); got != wanted {
			t.Errorf("the seconds register %v reads $%02x, wanted $%02x",
				index, got, wanted)
		}
	}
}

func TestTheSecondsCanBeSet(t *testing.T) {
	r := NewAppleRTC("")

	for index, value := range []uint8{0x11, 0x22, 0x33, 0x44} {
		writeRtc(r, secondsCommand(index, false), value)
	}

	if r.seconds != 0x44332211 {
		t.Errorf("the counter reads $%08x, wanted $44332211", r.seconds)
	}
}

func TestTheCounterAdvances(t *testing.T) {
	r := NewAppleRTC("")
	before := r.seconds

	r.TickSecond()
	if r.seconds != before+1 {
		t.Error("the counter did not advance a second")
	}
}

// The Macintosh counts from midnight of the first of January 1904
func TestTheEpoch(t *testing.T) {
	epoch := time.Date(1904, 1, 1, 0, 0, 0, 0, time.Local)

	if got := macSeconds(epoch); got != 0 {
		t.Errorf("the epoch itself is %v seconds, wanted 0", got)
	}
	if got := macSeconds(epoch.Add(time.Hour)); got != 3600 {
		t.Errorf("an hour after the epoch is %v seconds, wanted 3600", got)
	}

	// The epoch is 1904 and not the Unix one. The exact number depends on
	// what the zone was doing in 1904, so this only pins the era: sixty six
	// years of seconds, give or take a day.
	const yearsToUnix = 24107 * 86400
	unix := macSeconds(time.Date(1970, 1, 1, 0, 0, 0, 0, time.Local))
	if unix < yearsToUnix-86400 || unix > yearsToUnix+86400 {
		t.Errorf("the Unix epoch reads %v Macintosh seconds, wanted about %v",
			unix, yearsToUnix)
	}
}

func TestTheWriteProtectBlocksWrites(t *testing.T) {
	r := NewAppleRTC("")
	command := pramCommand(0, false)

	writeRtc(r, command, 0x5a)
	writeRtc(r, rtcCommandProtect, rtcProtectEnable)
	writeRtc(r, command, 0xff)

	if got := readRtc(r, command|rtcCommandRead); got != 0x5a {
		t.Errorf("a write got through the write protect, the byte reads $%02x", got)
	}

	// And clearing it lets writes through again
	writeRtc(r, rtcCommandProtect, 0)
	writeRtc(r, command, 0xff)
	if got := readRtc(r, command|rtcCommandRead); got != 0xff {
		t.Errorf("the write protect stayed on, the byte reads $%02x", got)
	}
}

// Raising the enable line abandons whatever was in progress, so a command
// interrupted half way must not be mistaken for the next one
func TestRaisingTheEnableAbortsTheTransaction(t *testing.T) {
	r := NewAppleRTC("")
	command := pramCommand(0, false)

	writeRtc(r, command, 0x33)

	// Start a write and give up after the command
	startTransaction(r)
	sendByte(r, command)
	endTransaction(r)

	// The byte that would have been the data must not be taken as one
	startTransaction(r)
	sendByte(r, 0xff)
	endTransaction(r)

	if got := readRtc(r, command|rtcCommandRead); got != 0x33 {
		t.Errorf("an abandoned transaction wrote anyway, the byte reads $%02x", got)
	}
}

func TestTheChipOnlyDrivesTheDataLineWhileAnswering(t *testing.T) {
	r := NewAppleRTC("")

	// An undriven line reads high
	if !r.DataOut() {
		t.Error("the chip is holding the data line down with nothing to say")
	}

	startTransaction(r)
	sendByte(r, pramCommand(0, true))
	if !r.sending {
		t.Fatal("a read command did not put the chip in answering state")
	}

	// It holds the last bit until the transaction ends, rather than letting
	// go in the same edge that presents it, which would lose that bit to
	// the undriven line
	receiveByte(r)
	if !r.sending {
		t.Error("the chip let go of the last bit before it could be read")
	}

	endTransaction(r)
	if r.sending {
		t.Error("the chip kept answering after the enable went high")
	}
	if !r.DataOut() {
		t.Error("the chip did not let go of the data line")
	}
}

func TestAnUnknownCommandReadsAsZero(t *testing.T) {
	r := NewAppleRTC("")

	// The low two bits of a command are always 01, so this is not one
	if got := readRtc(r, 0x82); got != 0 {
		t.Errorf("an invalid command answered $%02x, wanted 0", got)
	}
}

func TestTheParameterRamSurvivesARestart(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "pram.bin")

	r := NewAppleRTC(filename)
	for address := 0; address < 16; address++ {
		writeRtc(r, pramCommand(address, false), uint8(address*3))
	}

	// A new machine reads back what the first one wrote
	again := NewAppleRTC(filename)
	for address := 0; address < 16; address++ {
		command := pramCommand(address, true)
		if got := readRtc(again, command); got != uint8(address*3) {
			t.Errorf("after a restart the parameter RAM $%02x reads $%02x, wanted $%02x",
				address, got, address*3)
		}
	}
}

// A missing or damaged file is not an error. The ROM notices that the
// contents make no sense and writes its defaults, as it does for a Macintosh
// with a flat battery.
func TestAnUnusableParameterRamFileIsIgnored(t *testing.T) {
	dir := t.TempDir()

	missing := filepath.Join(dir, "missing.bin")
	if r := NewAppleRTC(missing); r.pram != [pramSize]uint8{} {
		t.Error("a missing file did not leave the parameter RAM empty")
	}

	short := filepath.Join(dir, "short.bin")
	if err := os.WriteFile(short, []uint8{1, 2, 3}, 0o600); err != nil {
		t.Fatal(err)
	}
	if r := NewAppleRTC(short); r.pram != [pramSize]uint8{} {
		t.Error("a file of the wrong size was loaded anyway")
	}
}

/*
The sequence the ROM uses to read the clock, from INITUTIL_CONT of the ROM
disassembly. It starts at $9d and steps down by four, which takes it through
the four registers of the counter with the bit 4 set and then the same four
with it clear, and it compares the two halves to be sure it did not catch the
counter in the middle of a tick.

A chip that answers only one of the two groups gives four zeros for the other
half. The ROM retries once, gives up with a clock read error, and the machine
sits at the epoch with no sign of why.
*/
func TestTheRomReadsTheClockTwiceAndComparesTheHalves(t *testing.T) {
	r := NewAppleRTC("")
	r.seconds = 0x12345678

	var read [8]uint8
	command := uint8(0x9d)
	for i := range read {
		read[i] = readRtc(r, command)
		command -= 4
	}

	// The first four are the counter, most significant byte first
	wanted := [4]uint8{0x12, 0x34, 0x56, 0x78}
	for i, b := range wanted {
		if read[i] != b {
			t.Errorf("the byte %v of the counter reads $%02x, wanted $%02x",
				i, read[i], b)
		}
	}

	// And the second four have to match them, or the ROM decides the read
	// went wrong
	for i := 0; i < 4; i++ {
		if read[i+4] != read[i] {
			t.Errorf("the halves differ at the byte %v, $%02x against $%02x",
				i, read[i], read[i+4])
		}
	}
}

// The low two bits of a command are 01 on every one there is, so a byte
// without them reaches nothing
func TestACommandWithoutItsTailReachesNothing(t *testing.T) {
	r := NewAppleRTC("")
	r.seconds = 0x11223344

	for _, command := range []uint8{0x80, 0x82, 0x83, 0x9c} {
		if got := readRtc(r, command); got != 0 {
			t.Errorf("the command $%02x answered $%02x, wanted nothing", command, got)
		}
	}
}

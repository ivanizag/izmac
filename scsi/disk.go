package scsi

import (
	"fmt"

	"github.com/ivanizag/izmac/storage"
)

/*
Disk is a direct access device on the bus, the hard disk the Macintosh boots
from. It answers the commands the ROM and the disk driver use and nothing
else, which is a short list: everything the boot needs is an inquiry, a
capacity, and reads.

The target owns the phase because that is how SCSI works. The initiator asks
for a byte and the target decides what comes next: command, then data one way
or the other, then the status and a message, then the bus is free again.
*/

// The SCSI phases, as the target moves through them
type phase int

const (
	phaseBusFree phase = iota
	phaseSelection
	phaseCommand
	phaseDataIn
	phaseDataOut
	phaseStatus
	phaseMessageIn
)

func (p phase) String() string {
	switch p {
	case phaseBusFree:
		return "bus free"
	case phaseSelection:
		return "selection"
	case phaseCommand:
		return "command"
	case phaseDataIn:
		return "data in"
	case phaseDataOut:
		return "data out"
	case phaseStatus:
		return "status"
	case phaseMessageIn:
		return "message in"
	}
	return "unknown"
}

// The commands answered. The rest are rejected with a sense of invalid
// command, which is what a real device does and what the driver expects.
const (
	cmdTestUnitReady  = 0x00
	cmdRequestSense   = 0x03
	cmdFormatUnit     = 0x04
	cmdRead6          = 0x08
	cmdWrite6         = 0x0a
	cmdInquiry        = 0x12
	cmdModeSelect6    = 0x15
	cmdModeSense6     = 0x1a
	cmdStartStopUnit  = 0x1b
	cmdPreventRemoval = 0x1e
	cmdReadCapacity   = 0x25
	cmdRead10         = 0x28
	cmdWrite10        = 0x2a
)

// The status bytes
const (
	statusGood           uint8 = 0x00
	statusCheckCondition uint8 = 0x02
)

// The sense keys
const (
	senseNoSense        uint8 = 0x00
	senseNotReady       uint8 = 0x02
	senseMediumError    uint8 = 0x03
	senseIllegalRequest uint8 = 0x05
	senseUnitAttention  uint8 = 0x06
	senseDataProtect    uint8 = 0x07
)

// Disk is one device on the bus
type Disk struct {
	id   uint8
	disk storage.BlockDisk

	phase phase

	// command is the descriptor block being gathered
	command []uint8
	// commandLength is how long it will be, from its group code
	commandLength int

	// data is the buffer moving in either direction
	data  []uint8
	index int

	status  uint8
	message uint8

	senseKey  uint8
	senseInfo uint32

	trace bool
}

func NewDisk(id uint8, disk storage.BlockDisk, trace bool) *Disk {
	return &Disk{
		id:    id,
		disk:  disk,
		phase: phaseBusFree,
		trace: trace,

		// A device answers a unit attention after a reset. The ROM
		// revision izmac targets, 'Loud Harmonicas', is the one that
		// handles it, which is why it is the revision to use.
		senseKey: senseUnitAttention,
	}
}

// busReset takes a device back to where it was at power on, which is what a
// reset of the bus does to it
func (t *Disk) busReset() {
	t.phase = phaseBusFree
	t.command = t.command[:0]
	t.commandLength = 0
	t.data = nil
	t.index = 0
	t.senseKey = senseUnitAttention
}

// select starts a command. The initiator has put the target id on the bus.
func (t *Disk) startSelection() {
	t.phase = phaseCommand
	t.command = t.command[:0]
	t.commandLength = 0
	t.data = nil
	t.index = 0
	t.status = statusGood
	t.message = 0
}

// commandLengthOf returns the size of a descriptor block from its group code,
// the top three bits of the operation code
func commandLengthOf(opcode uint8) int {
	switch opcode >> 5 {
	case 0:
		return 6
	case 1, 2:
		return 10
	case 5:
		return 12
	}
	return 6
}

// putByte takes a byte from the initiator, in the command or data out phases
func (t *Disk) putByte(value uint8) {
	switch t.phase {
	case phaseCommand:
		t.command = append(t.command, value)
		if t.commandLength == 0 {
			t.commandLength = commandLengthOf(value)
		}
		if len(t.command) >= t.commandLength {
			t.execute()
		}

	case phaseDataOut:
		if t.index < len(t.data) {
			t.data[t.index] = value
			t.index++
		}
		if t.index >= len(t.data) {
			t.completeWrite()
		}
	}
}

// getByte hands a byte to the initiator and advances the phase when the
// buffer runs out
func (t *Disk) getByte() uint8 {
	switch t.phase {
	case phaseDataIn:
		var value uint8
		if t.index < len(t.data) {
			value = t.data[t.index]
			t.index++
		}
		if t.index >= len(t.data) {
			t.phase = phaseStatus
		}
		return value

	case phaseStatus:
		t.phase = phaseMessageIn
		return t.status

	case phaseMessageIn:
		t.phase = phaseBusFree
		return t.message
	}

	return 0
}

// execute runs the command once the whole descriptor block has arrived
func (t *Disk) execute() {
	opcode := t.command[0]

	if t.trace {
		fmt.Printf("SCSI %v: %v\n", t.id, describeCommand(t.command))
	}

	switch opcode {
	case cmdTestUnitReady:
		t.finishGood()

	case cmdRequestSense:
		t.sendData(t.senseData())

	case cmdInquiry:
		t.sendData(t.inquiryData(int(t.command[4])))

	case cmdReadCapacity:
		t.sendData(t.capacityData())

	case cmdModeSense6:
		t.sendData(t.modeSenseData())

	case cmdRead6, cmdRead10:
		t.read()

	case cmdWrite6, cmdWrite10:
		t.write()

	case cmdFormatUnit, cmdModeSelect6, cmdStartStopUnit, cmdPreventRemoval:
		// Accepted and ignored, there is nothing to do to a file
		t.finishGood()

	default:
		t.fail(senseIllegalRequest)
	}
}

// blockAndCount pulls the address and the length out of a six or ten byte
// descriptor block
func (t *Disk) blockAndCount() (uint32, uint32) {
	if t.command[0]>>5 == 0 {
		// The six byte form has a 21 bit address and a byte count, with
		// zero meaning 256 blocks
		block := uint32(t.command[1]&0x1f)<<16 |
			uint32(t.command[2])<<8 | uint32(t.command[3])
		count := uint32(t.command[4])
		if count == 0 {
			count = 256
		}
		return block, count
	}

	block := uint32(t.command[2])<<24 | uint32(t.command[3])<<16 |
		uint32(t.command[4])<<8 | uint32(t.command[5])
	count := uint32(t.command[7])<<8 | uint32(t.command[8])
	return block, count
}

func (t *Disk) read() {
	block, count := t.blockAndCount()

	data := make([]uint8, 0, count*storage.BlockSize)
	for i := uint32(0); i < count; i++ {
		b, err := t.disk.Read(block + i)
		if err != nil {
			t.senseInfo = block + i
			t.fail(senseMediumError)
			return
		}
		data = append(data, b...)
	}

	t.senseKey = senseNoSense
	t.sendData(data)
}

func (t *Disk) write() {
	if t.disk.IsReadOnly() {
		t.fail(senseDataProtect)
		return
	}

	_, count := t.blockAndCount()
	t.data = make([]uint8, count*storage.BlockSize)
	t.index = 0
	t.phase = phaseDataOut
}

func (t *Disk) completeWrite() {
	block, count := t.blockAndCount()

	for i := uint32(0); i < count; i++ {
		from := i * storage.BlockSize
		err := t.disk.Write(block+i, t.data[from:from+storage.BlockSize])
		if err != nil {
			t.senseInfo = block + i
			t.fail(senseMediumError)
			return
		}
	}

	t.senseKey = senseNoSense
	t.finishGood()
}

// sendData moves to the data in phase with the buffer to hand over
func (t *Disk) sendData(data []uint8) {
	t.data = data
	t.index = 0
	t.status = statusGood

	if len(data) == 0 {
		t.phase = phaseStatus
		return
	}
	t.phase = phaseDataIn
}

func (t *Disk) finishGood() {
	t.status = statusGood
	t.data = nil
	t.phase = phaseStatus
}

func (t *Disk) fail(key uint8) {
	t.senseKey = key
	t.status = statusCheckCondition
	t.data = nil
	t.phase = phaseStatus
}

// senseData is the fixed format sense the driver asks for after a failure
func (t *Disk) senseData() []uint8 {
	data := make([]uint8, 18)
	data[0] = 0x70 // Current error, fixed format
	data[2] = t.senseKey
	data[3] = uint8(t.senseInfo >> 24)
	data[4] = uint8(t.senseInfo >> 16)
	data[5] = uint8(t.senseInfo >> 8)
	data[6] = uint8(t.senseInfo)
	data[7] = 10 // Additional length

	// The sense is cleared once it has been read
	t.senseKey = senseNoSense
	t.senseInfo = 0

	return data
}

// inquiryData describes the device. The driver reads the type from the first
// byte and refuses anything that is not a direct access device.
func (t *Disk) inquiryData(allocation int) []uint8 {
	data := make([]uint8, 36)
	data[0] = 0x00 // Direct access device
	data[1] = 0x00 // Not removable
	data[2] = 0x02 // Complies with SCSI-2
	data[3] = 0x02 // Response data format
	data[4] = 31   // Additional length

	copy(data[8:], []uint8("IZMAC   "))
	copy(data[16:], []uint8("izmac disk      "))
	copy(data[32:], []uint8("1.0 "))

	if allocation > 0 && allocation < len(data) {
		data = data[:allocation]
	}
	return data
}

// capacityData is the address of the last block and the block size
func (t *Disk) capacityData() []uint8 {
	last := t.disk.Blocks() - 1

	return []uint8{
		uint8(last >> 24), uint8(last >> 16), uint8(last >> 8), uint8(last),
		0, 0, uint8(storage.BlockSize >> 8), uint8(storage.BlockSize & 0xff),
	}
}

// modeSenseData is the shortest answer that says the device is not write
// protected and has no block descriptors worth reading
func (t *Disk) modeSenseData() []uint8 {
	deviceSpecific := uint8(0)
	if t.disk.IsReadOnly() {
		deviceSpecific = 0x80 // Write protected
	}

	return []uint8{3, 0, deviceSpecific, 0}
}

// describeCommand names a descriptor block for the trace
func describeCommand(command []uint8) string {
	names := map[uint8]string{
		cmdTestUnitReady:  "TEST UNIT READY",
		cmdRequestSense:   "REQUEST SENSE",
		cmdFormatUnit:     "FORMAT UNIT",
		cmdRead6:          "READ(6)",
		cmdWrite6:         "WRITE(6)",
		cmdInquiry:        "INQUIRY",
		cmdModeSelect6:    "MODE SELECT(6)",
		cmdModeSense6:     "MODE SENSE(6)",
		cmdStartStopUnit:  "START STOP UNIT",
		cmdPreventRemoval: "PREVENT ALLOW REMOVAL",
		cmdReadCapacity:   "READ CAPACITY",
		cmdRead10:         "READ(10)",
		cmdWrite10:        "WRITE(10)",
	}

	name, known := names[command[0]]
	if !known {
		name = fmt.Sprintf("command $%02x", command[0])
	}

	return fmt.Sprintf("%-22s %x", name, command)
}

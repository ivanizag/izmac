// Package scsi is the bus of the Macintosh and the disks on it.
package scsi

/*
Bus is the NCR 5380 of the Macintosh Plus, the initiator side.

The chip is at $580000 and the addresses have the shape $580drn: r is the
register, n is 0 to read and 1 to write, and d is the DACK line used by the
pseudo DMA. So the registers are sixteen bytes apart, a read and a write of
the same register are at an even and an odd address, and the DACK variants
are $200 higher.

	0  Current SCSI Data      / Output Data
	1  Initiator Command      / Initiator Command
	2  Mode                   / Mode
	3  Target Command         / Target Command
	4  Current SCSI Bus Status/ Select Enable
	5  Bus and Status         / Start DMA Send
	6  Input Data             / Start DMA Target Receive
	7  Reset Parity Interrupt / Start DMA Initiator Receive

The 5380 does not run the bus by itself, the driver walks it through every
phase. What is emulated here is the register file and the handshake, with the
target holding the phase.
*/
type Bus struct {
	// targets holds the devices by their id. The initiator keeps the id 7
	// for itself, so a bus takes seven of them.
	targets [TargetCount]*Disk

	// selected is the target holding the bus, nil while it is free
	selected *Disk

	outputData uint8
	inputData  uint8

	initiatorCommand uint8
	mode             uint8
	targetCommand    uint8
	selectEnable     uint8
}

const (
	regCurrentData  = 0
	regInitiatorCmd = 1
	regMode         = 2
	regTargetCmd    = 3
	regBusStatus    = 4
	regBusAndStatus = 5
	regInputData    = 6
	regResetParity  = 7

	// The mode register
	modeArbitrate uint8 = 1 << 0

	// The initiator command register. The bits 5 and 6 are the assert
	// differential and test mode on a write, and report the arbitration on
	// a read, which is what the driver waits on.
	icrLostArbitration       uint8 = 1 << 5
	icrArbitrationInProgress uint8 = 1 << 6

	icrAssertData uint8 = 1 << 0
	icrAssertAtn  uint8 = 1 << 1
	icrAssertSel  uint8 = 1 << 2
	icrAssertBsy  uint8 = 1 << 3
	icrAssertAck  uint8 = 1 << 4
	icrAssertRst  uint8 = 1 << 7

	// The current SCSI bus status register
	busStatusDbp uint8 = 1 << 0
	busStatusSel uint8 = 1 << 1
	busStatusIo  uint8 = 1 << 2
	busStatusCd  uint8 = 1 << 3
	busStatusMsg uint8 = 1 << 4
	busStatusReq uint8 = 1 << 5
	busStatusBsy uint8 = 1 << 6
	busStatusRst uint8 = 1 << 7

	// The bus and status register
	basAck         uint8 = 1 << 0
	basAtn         uint8 = 1 << 1
	basBusyError   uint8 = 1 << 2
	basPhaseMatch  uint8 = 1 << 3
	basInterrupt   uint8 = 1 << 4
	basParityError uint8 = 1 << 5
	basDmaRequest  uint8 = 1 << 6
	basEndOfDma    uint8 = 1 << 7
)

// TargetCount is how many devices a bus can hold, the eight ids less the
// one the initiator answers to
const TargetCount = 7

// initiatorId is the id the Macintosh gives itself
const initiatorId = 7

func NewBus() *Bus {
	return &Bus{}
}

// Attach puts a disk on the bus at its own id
func (s *Bus) Attach(d *Disk) {
	if int(d.id) < len(s.targets) {
		s.targets[d.id] = d
	}
}

/*
Attachment describes a disk on the bus, for a caller that wants to say what
is there without being handed the disk itself.
*/
type Attachment struct {
	Id     int
	Name   string
	Blocks uint32
}

// Attached describes the disks on the bus, in the order of their ids
func (s *Bus) Attached() []Attachment {
	attached := make([]Attachment, 0, len(s.targets))

	for id, d := range s.targets {
		if d == nil {
			continue
		}
		attached = append(attached, Attachment{
			Id:     id,
			Name:   d.disk.Name(),
			Blocks: d.disk.Blocks(),
		})
	}
	return attached
}

// phase is the phase of whichever target holds the bus, or bus free when
// none does
func (s *Bus) currentPhase() phase {
	if s.selected == nil {
		return phaseBusFree
	}
	return s.selected.phase
}

// registerOf returns the register an address reaches
func registerOf(address uint32) uint8 {
	return uint8((address >> 4) & 0x07)
}

// isDack tells if the address is one of the pseudo DMA ones, which
// handshake by themselves
func isDack(address uint32) bool {
	return address&0x200 != 0
}

func (s *Bus) Peek(address uint32) uint8 {
	switch registerOf(address) {
	case regCurrentData:
		return s.readData(isDack(address))
	case regInitiatorCmd:
		return s.readInitiatorCommand()
	case regMode:
		return s.mode
	case regTargetCmd:
		return s.targetCommand
	case regBusStatus:
		return s.busStatus()
	case regBusAndStatus:
		return s.busAndStatus()
	case regInputData:
		return s.readData(isDack(address))
	case regResetParity:
		return 0
	}
	return 0
}

func (s *Bus) Poke(address uint32, value uint8) {
	switch registerOf(address) {
	case regCurrentData:
		s.outputData = value
		if isDack(address) && s.selected != nil {
			s.selected.putByte(value)
		}
		s.trySelect()
	case regInitiatorCmd:
		s.writeInitiatorCommand(value)
	case regMode:
		s.mode = value
		s.trySelect()
	case regTargetCmd:
		s.targetCommand = value
	case regBusStatus:
		s.selectEnable = value
	case regBusAndStatus, regInputData, regResetParity:
		// Starting a DMA transfer. There is nothing to set up, the
		// handshake happens on the DACK addresses.
	}
}

/*
readInitiatorCommand reports the arbitration on the two bits that mean
something else on a write. The driver puts its own id on the data bus, sets
the arbitrate bit of the mode register and waits here for the arbitration to
be in progress and for the lost arbitration bit to stay clear.

Nothing else ever wants the bus, so the arbitration is won as soon as it is
asked for and it is never lost.
*/
func (s *Bus) readInitiatorCommand() uint8 {
	value := s.initiatorCommand &^ (icrArbitrationInProgress | icrLostArbitration)

	if s.arbitrating() {
		value |= icrArbitrationInProgress
	}

	return value
}

// arbitrating tells if the driver has asked for the bus and not yet finished
// selecting a target with it
func (s *Bus) arbitrating() bool {
	return s.mode&modeArbitrate != 0
}

/*
writeInitiatorCommand is where the bus is driven. Two transitions matter: the
select line going up with the target id on the data bus starts a command, and
the acknowledge going up moves one byte.
*/
func (s *Bus) writeInitiatorCommand(value uint8) {
	previous := s.initiatorCommand
	s.initiatorCommand = value

	if value&icrAssertRst != 0 {
		s.reset()
		return
	}

	s.trySelect()

	/*
		The handshake on the rising edge of the acknowledge. Which way the
		byte goes depends on the phase, and not every transfer uses the
		same port: the bulk of the data moves through the pseudo DMA
		addresses, which acknowledge by themselves, but the status, the
		message and any byte the driver is throwing away are read from
		the data register directly and taken by this acknowledge.

		Data in is on this list because the driver reads fewer bytes than
		it asked for and then drains the rest. Probing a disk it asks for
		a whole block and takes only the first 256 bytes of it, because
		that is all of the driver descriptor map it wants, and then
		SCSIComplete finds the bus still in the data phase and bit buckets
		a byte at a time until the target reaches the status phase. A
		target that does not move on those acknowledges never gets there,
		and the driver gives up and resets the bus.
	*/
	if value&icrAssertAck != 0 && previous&icrAssertAck == 0 && s.selected != nil {
		switch s.selected.phase {
		case phaseCommand, phaseDataOut:
			s.selected.putByte(s.outputData)
		case phaseDataIn, phaseStatus, phaseMessageIn:
			s.selected.getByte()
		}
	}

	s.releaseIfFree()
}

// releaseIfFree lets go of a target that has finished with the bus
func (s *Bus) releaseIfFree() {
	if s.selected != nil && s.selected.phase == phaseBusFree {
		s.selected = nil
	}
}

/*
trySelect answers a selection aimed at this target. It is checked after every
change to the bus rather than on the edge of the select line, because the
driver asserts select first and only then puts the target on the data bus:
arbitrate for the bus, win it, assert select, write the initiator and the
target as a bit each, and assert the data bus. Looking at the data when
select goes up sees only the initiator's own id and never selects anything.
*/
func (s *Bus) trySelect() {
	if s.selected != nil {
		return
	}
	if s.initiatorCommand&icrAssertSel == 0 {
		return
	}
	if s.initiatorCommand&icrAssertData == 0 {
		return
	}

	// The data bus carries the initiator and the target as a bit each, so
	// what is left after taking the initiator out names the device wanted
	wanted := s.outputData &^ (1 << initiatorId)
	for _, t := range s.targets {
		if t != nil && wanted&(1<<t.id) != 0 {
			s.selected = t
			t.startSelection()
			return
		}
	}
}

// readData hands over a byte, acknowledging it when the address used is one
// of the pseudo DMA ones
func (s *Bus) readData(dack bool) uint8 {
	if s.selected == nil {
		return 0
	}
	if !dack {
		// Reading without the handshake peeks at what the target offers
		return s.peekByte()
	}

	return s.selected.getByte()
}

// peekByte looks at the byte the target has ready without taking it
func (s *Bus) peekByte() uint8 {
	t := s.selected

	switch t.phase {
	case phaseDataIn:
		if t.index < len(t.data) {
			return t.data[t.index]
		}
	case phaseStatus:
		return t.status
	case phaseMessageIn:
		return t.message
	}
	return 0
}

/*
busStatus reports the lines the target is driving. The phase is carried by
the message, command/data and input/output lines, which is what the driver
reads to know what to do next.
*/
func (s *Bus) busStatus() uint8 {
	var status uint8

	if s.initiatorCommand&icrAssertSel != 0 {
		status |= busStatusSel
	}
	if s.initiatorCommand&icrAssertRst != 0 {
		status |= busStatusRst
	}

	if s.currentPhase() == phaseBusFree {
		if s.arbitrating() {
			// The chip drives the bus busy while it holds it
			status |= busStatusBsy
		}
		return status
	}

	// A target that has been selected holds the bus busy
	status |= busStatusBsy | busStatusDbp

	/*
		The request and acknowledge lines interlock, which is the whole
		of the SCSI handshake: the target asks for a byte with request,
		the initiator answers with acknowledge, the target drops request,
		and only then does the initiator drop acknowledge and the next
		byte begin. A request that stays up forever leaves the driver
		waiting for it to fall, and every acknowledge it sends meanwhile
		hands over the same stale byte again.
	*/
	if s.initiatorCommand&icrAssertAck == 0 {
		status |= busStatusReq
	}

	msg, cd, io := phaseLines(s.currentPhase())
	if msg {
		status |= busStatusMsg
	}
	if cd {
		status |= busStatusCd
	}
	if io {
		status |= busStatusIo
	}

	return status
}

// phaseLines returns the message, command/data and input/output lines of a
// phase, which is how a phase is named on the bus
func phaseLines(phase phase) (msg bool, cd bool, io bool) {
	switch phase {
	case phaseCommand:
		return false, true, false
	case phaseDataIn:
		return false, false, true
	case phaseDataOut:
		return false, false, false
	case phaseStatus:
		return false, true, true
	case phaseMessageIn:
		return true, true, true
	}
	return false, false, false
}

// busAndStatus reports the handshake state. The phase match bit says that
// the target agrees with the phase the driver set on the target command
// register, and the driver waits on it before every transfer.
func (s *Bus) busAndStatus() uint8 {
	var status uint8

	if s.initiatorCommand&icrAssertAck != 0 {
		status |= basAck
	}
	if s.initiatorCommand&icrAssertAtn != 0 {
		status |= basAtn
	}

	if s.phaseMatches() {
		status |= basPhaseMatch
	}
	if s.currentPhase() != phaseBusFree {
		status |= basDmaRequest
	}

	return status
}

// phaseMatches compares the phase the driver expects, on the low three bits
// of the target command register, with the one the target is in
func (s *Bus) phaseMatches() bool {
	if s.currentPhase() == phaseBusFree {
		return false
	}

	msg, cd, io := phaseLines(s.currentPhase())

	var wanted uint8
	if io {
		wanted |= 1 << 0
	}
	if cd {
		wanted |= 1 << 1
	}
	if msg {
		wanted |= 1 << 2
	}

	return s.targetCommand&0x07 == wanted
}

func (s *Bus) reset() {
	s.outputData = 0
	s.inputData = 0
	s.mode = 0
	s.targetCommand = 0

	for _, t := range s.targets {
		if t != nil {
			t.busReset()
		}
	}
	s.selected = nil
}

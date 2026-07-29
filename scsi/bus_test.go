package scsi

import (
	"testing"

	"github.com/ivanizag/izmac/storage"
)

const (
	scsiBaseAddress = 0x58_0000

	// testDiskId is where the tests put their only disk
	testDiskId = 0
)

func scsiAddress(reg uint32, write bool, dack bool) uint32 {
	address := uint32(scsiBaseAddress) + reg*0x10
	if write {
		address++
	}
	if dack {
		address += 0x200
	}
	return address
}

func newTestScsi(t *testing.T, blocks uint32) (*Bus, storage.BlockDisk) {
	t.Helper()

	disk := storage.NewBlockDiskMemory(blocks)
	s := NewBus()
	s.Attach(NewDisk(testDiskId, disk, false))
	return s, disk
}

// theTarget is the only device the tests put on the bus
func (s *Bus) theTarget() *Disk {
	return s.targets[testDiskId]
}

/*
selectTarget drives the bus the way the driver does. The order matters and
getting it wrong is what kept the ROM from ever selecting anything: arbitrate
for the bus with the initiator's own id, assert select, and only then put the
target on the data bus and assert it.
*/
func selectTarget(s *Bus, id uint8) {
	s.Poke(scsiAddress(regCurrentData, true, false), 1<<7)
	s.Poke(scsiAddress(regMode, true, false), modeArbitrate)
	s.Poke(scsiAddress(regInitiatorCmd, true, false), icrAssertSel)
	s.Poke(scsiAddress(regCurrentData, true, false), 1<<7|1<<id)
	s.Poke(scsiAddress(regInitiatorCmd, true, false), icrAssertSel|icrAssertData)
	s.Poke(scsiAddress(regMode, true, false), 0)
	s.Poke(scsiAddress(regInitiatorCmd, true, false), icrAssertData)
}

// runCommand selects the target, hands over the descriptor block a byte at a
// time, then takes whatever comes back until the bus is free again. It
// returns the data and the status.
func runCommand(s *Bus, command []uint8) ([]uint8, uint8) {
	selectTarget(s, s.theTarget().id)

	for _, b := range command {
		s.Poke(scsiAddress(regCurrentData, true, false), b)
		s.Poke(scsiAddress(regInitiatorCmd, true, false), icrAssertAck)
		s.Poke(scsiAddress(regInitiatorCmd, true, false), 0)
	}

	data := make([]uint8, 0)
	status := uint8(0)

	for i := 0; i < 1<<20; i++ {
		switch s.currentPhase() {
		case phaseDataIn:
			data = append(data, s.Peek(scsiAddress(regInputData, false, true)))
		case phaseStatus:
			status = s.Peek(scsiAddress(regInputData, false, true))
		case phaseMessageIn:
			s.Peek(scsiAddress(regInputData, false, true))
		default:
			return data, status
		}
	}

	return data, status
}

func TestTheScsiRegistersAre16BytesApart(t *testing.T) {
	for reg := uint32(0); reg < 8; reg++ {
		for _, write := range []bool{false, true} {
			for _, dack := range []bool{false, true} {
				address := scsiAddress(reg, write, dack)
				if got := registerOf(address); uint32(got) != reg {
					t.Errorf("$%06x reached the register %v, wanted %v",
						address, got, reg)
				}
				if isDack(address) != dack {
					t.Errorf("$%06x got the DACK line wrong", address)
				}
			}
		}
	}
}

func TestTheBusIsFreeUntilSomethingIsSelected(t *testing.T) {
	s, _ := newTestScsi(t, 16)

	if s.busStatus()&busStatusBsy != 0 {
		t.Error("the bus reports busy with nothing selected")
	}
}

func TestSelectingAnAbsentTargetLeavesTheBusFree(t *testing.T) {
	s, _ := newTestScsi(t, 16)

	// An id that is not the one of the disk
	selectTarget(s, 4)
	s.Poke(scsiAddress(regInitiatorCmd, true, false), 0)

	if s.currentPhase() != phaseBusFree {
		t.Errorf("selecting an absent target left the bus on %v", s.currentPhase())
	}
}

func TestInquiryDescribesADirectAccessDevice(t *testing.T) {
	s, _ := newTestScsi(t, 16)

	data, status := runCommand(s, []uint8{cmdInquiry, 0, 0, 0, 36, 0})

	if status != statusGood {
		t.Fatalf("the inquiry answered the status $%02x", status)
	}
	if len(data) != 36 {
		t.Fatalf("the inquiry returned %v bytes, wanted 36", len(data))
	}
	if data[0] != 0x00 {
		t.Errorf("the device type is $%02x, wanted $00 for a direct access device", data[0])
	}
	if string(data[8:16]) != "IZMAC   " {
		t.Errorf("the vendor reads %q", string(data[8:16]))
	}
}

func TestTheCapacityIsTheLastBlockAndTheBlockSize(t *testing.T) {
	s, _ := newTestScsi(t, 100)

	data, status := runCommand(s, []uint8{cmdReadCapacity, 0, 0, 0, 0, 0, 0, 0, 0, 0})

	if status != statusGood {
		t.Fatalf("the capacity answered the status $%02x", status)
	}
	if len(data) != 8 {
		t.Fatalf("the capacity returned %v bytes, wanted 8", len(data))
	}

	last := uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])
	if last != 99 {
		t.Errorf("the last block is %v, wanted 99 on a disk of 100", last)
	}

	size := uint32(data[4])<<24 | uint32(data[5])<<16 | uint32(data[6])<<8 | uint32(data[7])
	if size != storage.BlockSize {
		t.Errorf("the block size is %v, wanted %v", size, storage.BlockSize)
	}
}

func TestReadingABlock(t *testing.T) {
	s, disk := newTestScsi(t, 16)

	block := make([]uint8, storage.BlockSize)
	for i := range block {
		block[i] = uint8(i)
	}
	if err := disk.Write(3, block); err != nil {
		t.Fatal(err)
	}

	data, status := runCommand(s, []uint8{cmdRead6, 0, 0, 3, 1, 0})

	if status != statusGood {
		t.Fatalf("the read answered the status $%02x", status)
	}
	if len(data) != storage.BlockSize {
		t.Fatalf("the read returned %v bytes, wanted %v", len(data), storage.BlockSize)
	}
	for i, v := range data {
		if v != uint8(i) {
			t.Fatalf("the byte %v of the block reads $%02x, wanted $%02x", i, v, uint8(i))
		}
	}
}

func TestReadingSeveralBlocksAtOnce(t *testing.T) {
	s, disk := newTestScsi(t, 16)

	for b := uint32(0); b < 3; b++ {
		block := make([]uint8, storage.BlockSize)
		block[0] = uint8(0xa0 + b)
		if err := disk.Write(b, block); err != nil {
			t.Fatal(err)
		}
	}

	data, _ := runCommand(s, []uint8{cmdRead6, 0, 0, 0, 3, 0})

	if len(data) != 3*storage.BlockSize {
		t.Fatalf("the read returned %v bytes, wanted three blocks", len(data))
	}
	for b := 0; b < 3; b++ {
		if got := data[b*storage.BlockSize]; got != uint8(0xa0+b) {
			t.Errorf("the block %v starts with $%02x, wanted $%02x", b, got, 0xa0+b)
		}
	}
}

func TestReadingPastTheEndFails(t *testing.T) {
	s, _ := newTestScsi(t, 16)

	_, status := runCommand(s, []uint8{cmdRead6, 0, 0, 200, 1, 0})

	if status != statusCheckCondition {
		t.Errorf("a read past the end answered the status $%02x", status)
	}
}

func TestTheTenByteReadIsDecodedToo(t *testing.T) {
	s, disk := newTestScsi(t, 16)

	block := make([]uint8, storage.BlockSize)
	block[0] = 0x5a
	if err := disk.Write(5, block); err != nil {
		t.Fatal(err)
	}

	data, status := runCommand(s, []uint8{cmdRead10, 0, 0, 0, 0, 5, 0, 0, 1, 0})

	if status != statusGood {
		t.Fatalf("the ten byte read answered the status $%02x", status)
	}
	if len(data) != storage.BlockSize || data[0] != 0x5a {
		t.Error("the ten byte read did not return the block asked for")
	}
}

// The device answers a unit attention after a reset, and the ROM revision
// izmac targets is the one that copes with it. It has to be reported once and
// then cleared.
func TestTheUnitAttentionIsReportedOnce(t *testing.T) {
	s, _ := newTestScsi(t, 16)

	data, status := runCommand(s, []uint8{cmdRequestSense, 0, 0, 0, 18, 0})
	if status != statusGood {
		t.Fatalf("the request sense answered the status $%02x", status)
	}
	if data[2] != senseUnitAttention {
		t.Errorf("the first sense key is $%02x, wanted the unit attention $%02x",
			data[2], senseUnitAttention)
	}

	data, _ = runCommand(s, []uint8{cmdRequestSense, 0, 0, 0, 18, 0})
	if data[2] != senseNoSense {
		t.Errorf("the sense was not cleared after being read, it reads $%02x", data[2])
	}
}

func TestAnUnknownCommandIsRejected(t *testing.T) {
	s, _ := newTestScsi(t, 16)

	_, status := runCommand(s, []uint8{0xdd, 0, 0, 0, 0, 0})
	if status != statusCheckCondition {
		t.Fatalf("an unknown command answered the status $%02x", status)
	}

	data, _ := runCommand(s, []uint8{cmdRequestSense, 0, 0, 0, 18, 0})
	if data[2] != senseIllegalRequest {
		t.Errorf("the sense key after an unknown command is $%02x, wanted $%02x",
			data[2], senseIllegalRequest)
	}
}

func TestWritingABlock(t *testing.T) {
	s, disk := newTestScsi(t, 16)

	// The command, then the data out phase a byte at a time
	selectTarget(s, s.theTarget().id)

	for _, b := range []uint8{cmdWrite6, 0, 0, 7, 1, 0} {
		s.Poke(scsiAddress(regCurrentData, true, false), b)
		s.Poke(scsiAddress(regInitiatorCmd, true, false), icrAssertAck)
		s.Poke(scsiAddress(regInitiatorCmd, true, false), 0)
	}

	if s.currentPhase() != phaseDataOut {
		t.Fatalf("the write did not ask for data, it is on %v", s.currentPhase())
	}

	for i := 0; i < storage.BlockSize; i++ {
		s.Poke(scsiAddress(regCurrentData, true, true), uint8(i))
	}

	block, err := disk.Read(7)
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range block {
		if v != uint8(i) {
			t.Fatalf("the byte %v of the block written reads $%02x", i, v)
		}
	}
}

func TestThePhaseMatchFollowsTheTargetCommandRegister(t *testing.T) {
	s, _ := newTestScsi(t, 16)

	selectTarget(s, s.theTarget().id)

	// The target asks for a command, which is command/data asserted and
	// input/output clear, so the driver puts a 2 on the target command
	s.Poke(scsiAddress(regTargetCmd, true, false), 0x02)
	if s.busAndStatus()&basPhaseMatch == 0 {
		t.Error("the phase does not match with the command phase selected")
	}

	s.Poke(scsiAddress(regTargetCmd, true, false), 0x01)
	if s.busAndStatus()&basPhaseMatch != 0 {
		t.Error("the phase matches a phase the target is not in")
	}
}

func TestTheBusStatusCarriesThePhase(t *testing.T) {
	s, _ := newTestScsi(t, 16)

	selectTarget(s, s.theTarget().id)

	status := s.busStatus()
	if status&busStatusBsy == 0 {
		t.Error("the target did not hold the bus busy after being selected")
	}
	if status&busStatusReq == 0 {
		t.Error("the target is not asking for a byte")
	}
	if status&busStatusCd == 0 {
		t.Error("the command phase does not assert command/data")
	}
	if status&busStatusIo != 0 {
		t.Error("the command phase asserts input/output")
	}
}

func TestAResetLeavesTheBusFree(t *testing.T) {
	s, _ := newTestScsi(t, 16)

	selectTarget(s, s.theTarget().id)

	s.Poke(scsiAddress(regInitiatorCmd, true, false), icrAssertRst)

	if s.currentPhase() != phaseBusFree {
		t.Errorf("the bus is on %v after a reset", s.currentPhase())
	}
}

// The driver waits for the arbitration to be reported in progress before it
// selects anything, on a bit that means something else when written
func TestTheArbitrationIsWonAsSoonAsItIsAsked(t *testing.T) {
	s, _ := newTestScsi(t, 16)

	if s.Peek(scsiAddress(regInitiatorCmd, false, false))&icrArbitrationInProgress != 0 {
		t.Error("the arbitration is in progress before it was asked for")
	}

	s.Poke(scsiAddress(regCurrentData, true, false), 1<<7)
	s.Poke(scsiAddress(regMode, true, false), modeArbitrate)

	icr := s.Peek(scsiAddress(regInitiatorCmd, false, false))
	if icr&icrArbitrationInProgress == 0 {
		t.Error("the arbitration was asked for and is not in progress")
	}
	if icr&icrLostArbitration != 0 {
		t.Error("the arbitration was lost with nothing else on the bus")
	}
	if s.busStatus()&busStatusBsy == 0 {
		t.Error("the bus is not held busy while arbitrating")
	}
}

// Asserting select with only the initiator on the data bus, which is what the
// driver does first, must not select anything
func TestSelectNeedsTheTargetOnTheDataBus(t *testing.T) {
	s, _ := newTestScsi(t, 16)

	s.Poke(scsiAddress(regCurrentData, true, false), 1<<7)
	s.Poke(scsiAddress(regMode, true, false), modeArbitrate)
	s.Poke(scsiAddress(regInitiatorCmd, true, false), icrAssertSel)

	if s.currentPhase() != phaseBusFree {
		t.Fatalf("the target answered a selection that named nobody, it is on %v",
			s.currentPhase())
	}

	// Now the driver puts the target on the bus and asserts it
	s.Poke(scsiAddress(regCurrentData, true, false), 1<<7|1<<s.theTarget().id)
	s.Poke(scsiAddress(regInitiatorCmd, true, false), icrAssertSel|icrAssertData)

	if s.currentPhase() != phaseCommand {
		t.Errorf("the target did not answer the selection, it is on %v", s.currentPhase())
	}
}

/*
The request and acknowledge lines interlock. The driver waits for request to
fall after it raises acknowledge, so a target that holds request up forever
hangs it, and the acknowledges it keeps sending meanwhile deliver the same
byte over and over.
*/
func TestRequestFallsWhileAcknowledgeIsUp(t *testing.T) {
	s, _ := newTestScsi(t, 16)
	selectTarget(s, s.theTarget().id)

	if s.busStatus()&busStatusReq == 0 {
		t.Fatal("the target is not asking for the first byte")
	}

	s.Poke(scsiAddress(regCurrentData, true, false), cmdTestUnitReady)
	s.Poke(scsiAddress(regInitiatorCmd, true, false), icrAssertData|icrAssertAck)

	if s.busStatus()&busStatusReq != 0 {
		t.Error("the request stayed up while the acknowledge was up")
	}

	s.Poke(scsiAddress(regInitiatorCmd, true, false), icrAssertData)
	if s.busStatus()&busStatusReq == 0 {
		t.Error("the request did not come back for the next byte")
	}
}

// Handing over a descriptor block the way the driver does, one byte at a
// time with the interlock respected, has to arrive as the bytes sent and not
// as the first one repeated
func TestTheDescriptorBlockArrivesIntact(t *testing.T) {
	s, _ := newTestScsi(t, 16)
	selectTarget(s, s.theTarget().id)

	command := []uint8{cmdRead6, 0x00, 0x00, 0x05, 0x01, 0x00}
	for _, b := range command {
		if s.busStatus()&busStatusReq == 0 {
			t.Fatal("the target is not asking for a byte")
		}
		s.Poke(scsiAddress(regCurrentData, true, false), b)
		s.Poke(scsiAddress(regInitiatorCmd, true, false), icrAssertData|icrAssertAck)
		s.Poke(scsiAddress(regInitiatorCmd, true, false), icrAssertData)
	}

	if s.currentPhase() != phaseDataIn {
		t.Fatalf("the read did not reach the data phase, it is on %v", s.currentPhase())
	}
	block, count := s.theTarget().blockAndCount()
	if block != 5 || count != 1 {
		t.Errorf("the block gathered is %v count %v, wanted 5 and 1", block, count)
	}
}

/*
The status and the message are not read through the pseudo DMA port. The
driver reads the data register directly and then asserts acknowledge, and it
is that acknowledge which takes the byte and moves the target on. A target
that stays in the status phase leaves the driver looking for the message in
the wrong phase, and it resets the bus.
*/
func TestTheStatusAndMessageAreTakenByTheAcknowledge(t *testing.T) {
	s, _ := newTestScsi(t, 16)
	selectTarget(s, s.theTarget().id)

	for _, b := range []uint8{cmdTestUnitReady, 0, 0, 0, 0, 0} {
		s.Poke(scsiAddress(regCurrentData, true, false), b)
		s.Poke(scsiAddress(regInitiatorCmd, true, false), icrAssertData|icrAssertAck)
		s.Poke(scsiAddress(regInitiatorCmd, true, false), icrAssertData)
	}

	if s.currentPhase() != phaseStatus {
		t.Fatalf("the command did not reach the status phase, it is on %v", s.currentPhase())
	}

	// Read the status without the pseudo DMA port, then acknowledge it
	if got := s.Peek(scsiAddress(regCurrentData, false, false)); got != statusGood {
		t.Errorf("the status reads $%02x, wanted $%02x", got, statusGood)
	}
	s.Poke(scsiAddress(regInitiatorCmd, true, false), icrAssertAck)
	s.Poke(scsiAddress(regInitiatorCmd, true, false), 0)

	if s.currentPhase() != phaseMessageIn {
		t.Fatalf("the acknowledge did not move the target to the message, it is on %v",
			s.currentPhase())
	}

	// The phase the driver will ask for has to match
	s.Poke(scsiAddress(regTargetCmd, true, false), 0x07)
	if s.busAndStatus()&basPhaseMatch == 0 {
		t.Error("the message in phase does not match the target command register")
	}

	s.Peek(scsiAddress(regCurrentData, false, false))
	s.Poke(scsiAddress(regInitiatorCmd, true, false), icrAssertAck)
	s.Poke(scsiAddress(regInitiatorCmd, true, false), 0)

	if s.currentPhase() != phaseBusFree {
		t.Errorf("the bus was not freed after the message, it is on %v", s.currentPhase())
	}
}

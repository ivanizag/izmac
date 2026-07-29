package izmac

import (
	"testing"

	"github.com/ivanizag/izmac/storage"
)

const scsiBaseAddress = 0x580000

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

func newTestScsi(t *testing.T, blocks uint32) (*scsi5380, storage.BlockDisk) {
	t.Helper()

	disk := storage.NewBlockDiskMemory(blocks)
	s := newScsi5380()
	s.attach(newScsiTarget(scsiFirstDiskId, disk, false))
	return s, disk
}

// theTarget is the only device the tests put on the bus
func (s *scsi5380) theTarget() *scsiTarget {
	return s.targets[scsiFirstDiskId]
}

/*
selectTarget drives the bus the way the driver does. The order matters and
getting it wrong is what kept the ROM from ever selecting anything: arbitrate
for the bus with the initiator's own id, assert select, and only then put the
target on the data bus and assert it.
*/
func selectTarget(s *scsi5380, id uint8) {
	s.poke(scsiAddress(scsiRegCurrentData, true, false), 1<<7)
	s.poke(scsiAddress(scsiRegMode, true, false), modeArbitrate)
	s.poke(scsiAddress(scsiRegInitiatorCmd, true, false), icrAssertSel)
	s.poke(scsiAddress(scsiRegCurrentData, true, false), 1<<7|1<<id)
	s.poke(scsiAddress(scsiRegInitiatorCmd, true, false), icrAssertSel|icrAssertData)
	s.poke(scsiAddress(scsiRegMode, true, false), 0)
	s.poke(scsiAddress(scsiRegInitiatorCmd, true, false), icrAssertData)
}

// runCommand selects the target, hands over the descriptor block a byte at a
// time, then takes whatever comes back until the bus is free again. It
// returns the data and the status.
func runCommand(s *scsi5380, command []uint8) ([]uint8, uint8) {
	selectTarget(s, s.theTarget().id)

	for _, b := range command {
		s.poke(scsiAddress(scsiRegCurrentData, true, false), b)
		s.poke(scsiAddress(scsiRegInitiatorCmd, true, false), icrAssertAck)
		s.poke(scsiAddress(scsiRegInitiatorCmd, true, false), 0)
	}

	data := make([]uint8, 0)
	status := uint8(0)

	for i := 0; i < 1<<20; i++ {
		switch s.phase() {
		case phaseDataIn:
			data = append(data, s.peek(scsiAddress(scsiRegInputData, false, true)))
		case phaseStatus:
			status = s.peek(scsiAddress(scsiRegInputData, false, true))
		case phaseMessageIn:
			s.peek(scsiAddress(scsiRegInputData, false, true))
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
				if got := scsiRegister(address); uint32(got) != reg {
					t.Errorf("$%06x reached the register %v, wanted %v",
						address, got, reg)
				}
				if scsiIsDack(address) != dack {
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
	s.poke(scsiAddress(scsiRegInitiatorCmd, true, false), 0)

	if s.phase() != phaseBusFree {
		t.Errorf("selecting an absent target left the bus on %v", s.phase())
	}
}

func TestInquiryDescribesADirectAccessDevice(t *testing.T) {
	s, _ := newTestScsi(t, 16)

	data, status := runCommand(s, []uint8{scsiCmdInquiry, 0, 0, 0, 36, 0})

	if status != scsiStatusGood {
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

	data, status := runCommand(s, []uint8{scsiCmdReadCapacity, 0, 0, 0, 0, 0, 0, 0, 0, 0})

	if status != scsiStatusGood {
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

	data, status := runCommand(s, []uint8{scsiCmdRead6, 0, 0, 3, 1, 0})

	if status != scsiStatusGood {
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

	data, _ := runCommand(s, []uint8{scsiCmdRead6, 0, 0, 0, 3, 0})

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

	_, status := runCommand(s, []uint8{scsiCmdRead6, 0, 0, 200, 1, 0})

	if status != scsiStatusCheckCondition {
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

	data, status := runCommand(s, []uint8{scsiCmdRead10, 0, 0, 0, 0, 5, 0, 0, 1, 0})

	if status != scsiStatusGood {
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

	data, status := runCommand(s, []uint8{scsiCmdRequestSense, 0, 0, 0, 18, 0})
	if status != scsiStatusGood {
		t.Fatalf("the request sense answered the status $%02x", status)
	}
	if data[2] != senseUnitAttention {
		t.Errorf("the first sense key is $%02x, wanted the unit attention $%02x",
			data[2], senseUnitAttention)
	}

	data, _ = runCommand(s, []uint8{scsiCmdRequestSense, 0, 0, 0, 18, 0})
	if data[2] != senseNoSense {
		t.Errorf("the sense was not cleared after being read, it reads $%02x", data[2])
	}
}

func TestAnUnknownCommandIsRejected(t *testing.T) {
	s, _ := newTestScsi(t, 16)

	_, status := runCommand(s, []uint8{0xdd, 0, 0, 0, 0, 0})
	if status != scsiStatusCheckCondition {
		t.Fatalf("an unknown command answered the status $%02x", status)
	}

	data, _ := runCommand(s, []uint8{scsiCmdRequestSense, 0, 0, 0, 18, 0})
	if data[2] != senseIllegalRequest {
		t.Errorf("the sense key after an unknown command is $%02x, wanted $%02x",
			data[2], senseIllegalRequest)
	}
}

func TestWritingABlock(t *testing.T) {
	s, disk := newTestScsi(t, 16)

	// The command, then the data out phase a byte at a time
	selectTarget(s, s.theTarget().id)

	for _, b := range []uint8{scsiCmdWrite6, 0, 0, 7, 1, 0} {
		s.poke(scsiAddress(scsiRegCurrentData, true, false), b)
		s.poke(scsiAddress(scsiRegInitiatorCmd, true, false), icrAssertAck)
		s.poke(scsiAddress(scsiRegInitiatorCmd, true, false), 0)
	}

	if s.phase() != phaseDataOut {
		t.Fatalf("the write did not ask for data, it is on %v", s.phase())
	}

	for i := 0; i < storage.BlockSize; i++ {
		s.poke(scsiAddress(scsiRegCurrentData, true, true), uint8(i))
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
	s.poke(scsiAddress(scsiRegTargetCmd, true, false), 0x02)
	if s.busAndStatus()&basPhaseMatch == 0 {
		t.Error("the phase does not match with the command phase selected")
	}

	s.poke(scsiAddress(scsiRegTargetCmd, true, false), 0x01)
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

	s.poke(scsiAddress(scsiRegInitiatorCmd, true, false), icrAssertRst)

	if s.phase() != phaseBusFree {
		t.Errorf("the bus is on %v after a reset", s.phase())
	}
}

// The driver waits for the arbitration to be reported in progress before it
// selects anything, on a bit that means something else when written
func TestTheArbitrationIsWonAsSoonAsItIsAsked(t *testing.T) {
	s, _ := newTestScsi(t, 16)

	if s.peek(scsiAddress(scsiRegInitiatorCmd, false, false))&icrArbitrationInProgress != 0 {
		t.Error("the arbitration is in progress before it was asked for")
	}

	s.poke(scsiAddress(scsiRegCurrentData, true, false), 1<<7)
	s.poke(scsiAddress(scsiRegMode, true, false), modeArbitrate)

	icr := s.peek(scsiAddress(scsiRegInitiatorCmd, false, false))
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

	s.poke(scsiAddress(scsiRegCurrentData, true, false), 1<<7)
	s.poke(scsiAddress(scsiRegMode, true, false), modeArbitrate)
	s.poke(scsiAddress(scsiRegInitiatorCmd, true, false), icrAssertSel)

	if s.phase() != phaseBusFree {
		t.Fatalf("the target answered a selection that named nobody, it is on %v",
			s.phase())
	}

	// Now the driver puts the target on the bus and asserts it
	s.poke(scsiAddress(scsiRegCurrentData, true, false), 1<<7|1<<s.theTarget().id)
	s.poke(scsiAddress(scsiRegInitiatorCmd, true, false), icrAssertSel|icrAssertData)

	if s.phase() != phaseCommand {
		t.Errorf("the target did not answer the selection, it is on %v", s.phase())
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

	s.poke(scsiAddress(scsiRegCurrentData, true, false), scsiCmdTestUnitReady)
	s.poke(scsiAddress(scsiRegInitiatorCmd, true, false), icrAssertData|icrAssertAck)

	if s.busStatus()&busStatusReq != 0 {
		t.Error("the request stayed up while the acknowledge was up")
	}

	s.poke(scsiAddress(scsiRegInitiatorCmd, true, false), icrAssertData)
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

	command := []uint8{scsiCmdRead6, 0x00, 0x00, 0x05, 0x01, 0x00}
	for _, b := range command {
		if s.busStatus()&busStatusReq == 0 {
			t.Fatal("the target is not asking for a byte")
		}
		s.poke(scsiAddress(scsiRegCurrentData, true, false), b)
		s.poke(scsiAddress(scsiRegInitiatorCmd, true, false), icrAssertData|icrAssertAck)
		s.poke(scsiAddress(scsiRegInitiatorCmd, true, false), icrAssertData)
	}

	if s.phase() != phaseDataIn {
		t.Fatalf("the read did not reach the data phase, it is on %v", s.phase())
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

	for _, b := range []uint8{scsiCmdTestUnitReady, 0, 0, 0, 0, 0} {
		s.poke(scsiAddress(scsiRegCurrentData, true, false), b)
		s.poke(scsiAddress(scsiRegInitiatorCmd, true, false), icrAssertData|icrAssertAck)
		s.poke(scsiAddress(scsiRegInitiatorCmd, true, false), icrAssertData)
	}

	if s.phase() != phaseStatus {
		t.Fatalf("the command did not reach the status phase, it is on %v", s.phase())
	}

	// Read the status without the pseudo DMA port, then acknowledge it
	if got := s.peek(scsiAddress(scsiRegCurrentData, false, false)); got != scsiStatusGood {
		t.Errorf("the status reads $%02x, wanted $%02x", got, scsiStatusGood)
	}
	s.poke(scsiAddress(scsiRegInitiatorCmd, true, false), icrAssertAck)
	s.poke(scsiAddress(scsiRegInitiatorCmd, true, false), 0)

	if s.phase() != phaseMessageIn {
		t.Fatalf("the acknowledge did not move the target to the message, it is on %v",
			s.phase())
	}

	// The phase the driver will ask for has to match
	s.poke(scsiAddress(scsiRegTargetCmd, true, false), 0x07)
	if s.busAndStatus()&basPhaseMatch == 0 {
		t.Error("the message in phase does not match the target command register")
	}

	s.peek(scsiAddress(scsiRegCurrentData, false, false))
	s.poke(scsiAddress(scsiRegInitiatorCmd, true, false), icrAssertAck)
	s.poke(scsiAddress(scsiRegInitiatorCmd, true, false), 0)

	if s.phase() != phaseBusFree {
		t.Errorf("the bus was not freed after the message, it is on %v", s.phase())
	}
}

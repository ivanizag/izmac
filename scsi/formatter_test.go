package scsi

import (
	"bytes"
	"testing"
)

/*
What Apple HD SC Setup asks of a drive before it lists it and formats it: the
page of Apple's own, the buffer, the format and geometry pages, and a mode
select and a format that send data. Each answer has to be as long as it asked
for, since it reads exactly that.
*/

// sendCommand hands over a descriptor block and the data out after it, and
// takes the status and the message, which leaves the bus free
func sendCommand(s *Bus, command []uint8, out []uint8) uint8 {
	selectTarget(s, s.theTarget().id)

	for _, b := range command {
		s.Poke(scsiAddress(regCurrentData, true, false), b)
		s.Poke(scsiAddress(regInitiatorCmd, true, false), icrAssertAck)
		s.Poke(scsiAddress(regInitiatorCmd, true, false), 0)
	}
	for _, b := range out {
		if s.currentPhase() != phaseDataOut {
			return 0xff
		}
		s.Poke(scsiAddress(regCurrentData, true, true), b)
	}
	if s.currentPhase() != phaseStatus {
		return 0xff
	}
	status := s.Peek(scsiAddress(regInputData, false, true))
	s.Peek(scsiAddress(regInputData, false, true))
	return status
}

func TestApplesPageCarriesItsWords(t *testing.T) {
	s, _ := newTestScsi(t, 2048)

	data, status := runCommand(s, []uint8{cmdModeSense6, 0, applePage, 0, 0x22, 0})
	if status != statusGood {
		t.Fatalf("the mode sense of Apple's page answered $%02x", status)
	}
	if len(data) != 0x22 {
		t.Fatalf("the mode sense answered %v bytes, HD SC Setup reads %v", len(data), 0x22)
	}
	if !bytes.Contains(data, []uint8("APPLE COMPUTER, INC")) {
		t.Errorf("Apple's page reads %q", data)
	}
}

func TestTheBlockDescriptorHasTheSizeOfTheDisk(t *testing.T) {
	s, _ := newTestScsi(t, 40960)

	data, _ := runCommand(s, []uint8{cmdModeSense6, 0, 0x03, 0, 0x20, 0})
	if data[3] != 8 {
		t.Fatalf("the block descriptor length is %v", data[3])
	}
	blocks := int(data[5])<<16 | int(data[6])<<8 | int(data[7])
	size := int(data[10])<<8 | int(data[11])
	if blocks != 40960 || size != 512 {
		t.Errorf("the block descriptor says %v blocks of %v bytes", blocks, size)
	}
	if len(data) != 0x20 || data[12] != 0x03 {
		t.Errorf("the format page did not follow, %v bytes starting % x", len(data), data[12:])
	}
}

func TestTheBufferKeepsWhatIsWrittenToIt(t *testing.T) {
	s, _ := newTestScsi(t, 16)

	header, _ := runCommand(s, []uint8{cmdReadBuffer, 0, 0, 0, 0, 0, 0, 0, 4, 0})
	if len(header) != 4 || int(header[1])<<16|int(header[2])<<8|int(header[3]) != bufferSize {
		t.Fatalf("the buffer header reads % x", header)
	}

	pattern := []uint8{0xde, 0xad, 0xbe, 0xef, 0x55, 0xaa}
	status := sendCommand(s, []uint8{cmdWriteBuffer, bufferModeData, 0, 0, 0, 0, 0, 0, 6, 0}, pattern)
	if status != statusGood {
		t.Fatalf("the write buffer answered $%02x", status)
	}
	back, _ := runCommand(s, []uint8{cmdReadBuffer, bufferModeData, 0, 0, 0, 0, 0, 0, 6, 0})
	if !bytes.Equal(back, pattern) {
		t.Errorf("the buffer reads back % x after writing % x", back, pattern)
	}
}

func TestModeSelectTakesItsParameters(t *testing.T) {
	s, _ := newTestScsi(t, 16)

	status := sendCommand(s, []uint8{cmdModeSelect6, 1, 0, 0, 0x18, 0}, make([]uint8, 0x18))
	if status != statusGood {
		t.Errorf("the mode select answered $%02x", status)
	}
}

func TestFormatUnitTakesItsDefectList(t *testing.T) {
	s, _ := newTestScsi(t, 16)

	// The header of four bytes says eight bytes of list follow
	list := []uint8{0, 0, 0, 8, 1, 2, 3, 4, 5, 6, 7, 8}
	status := sendCommand(s, []uint8{cmdFormatUnit, 0x10, 0, 0, 0, 0}, list)
	if status != statusGood {
		t.Errorf("the format with a defect list answered $%02x", status)
	}

	status = sendCommand(s, []uint8{cmdFormatUnit, 0, 0, 0, 5, 0}, nil)
	if status != statusGood {
		t.Errorf("the format without one answered $%02x", status)
	}
}

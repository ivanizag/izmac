package component

import (
	"bytes"
	"testing"
)

// frameRecorder is a network that keeps what the chip sends
type frameRecorder struct {
	frames [][]uint8
}

func (r *frameRecorder) SendFrame(frame []uint8) {
	r.frames = append(r.frames, append([]uint8(nil), frame...))
}

// writeRegister sets a register of channel B, the way the driver does
func writeRegister(s *SCC8530, register uint8, value uint8) {
	if register >= 8 {
		s.Write(ChannelB, control, wr0PointHigh|(register-8))
	} else {
		s.Write(ChannelB, control, register)
	}
	s.Write(ChannelB, control, value)
}

// readRegister reads a register of channel B
func readRegister(s *SCC8530, register uint8) uint8 {
	if register >= 8 {
		s.Write(ChannelB, control, wr0PointHigh|(register-8))
	} else if register != 0 {
		s.Write(ChannelB, control, register)
	}
	return s.Read(ChannelB, control)
}

/*
localTalkChip is channel B set up the way the .MPP driver of the ROM sets it
up, with the values of its tables: SDLC, the receiver on with address search
for node 9, and the interrupt on the first character of a frame.
*/
func localTalkChip(cyclesPerSecond uint64) (*SCC8530, *frameRecorder) {
	s := NewSCC8530(cyclesPerSecond)
	recorder := &frameRecorder{}
	s.AttachLink(ChannelB, recorder)

	writeRegister(s, 4, 0x20)
	writeRegister(s, 10, 0xe0)
	writeRegister(s, 6, 9)
	writeRegister(s, 3, 0xdd)
	writeRegister(s, 15, 0x08)
	writeRegister(s, 1, 0x09)
	return s, recorder
}

// sendFrame writes a frame the way the driver does: the transmitter on, the
// bytes, the underrun latch reset after the last one, then waiting for it
func sendFrame(t *testing.T, s *SCC8530, frame []uint8) {
	t.Helper()
	writeRegister(s, 5, 0x6b)
	for _, b := range frame {
		for i := 0; s.Read(ChannelB, control)&rr0TxBufferEmpty == 0; i++ {
			if i > 1000 {
				t.Fatalf("the transmit buffer never emptied")
			}
			s.Tick(10)
		}
		s.Write(ChannelB, data, b)
	}
	s.Write(ChannelB, control, wr0ResetTxUnderrunEom)
	for i := 0; s.Read(ChannelB, control)&rr0TxUnderrunEom == 0; i++ {
		if i > 1000 {
			t.Fatalf("the frame never ended")
		}
		s.Tick(10)
	}
	writeRegister(s, 5, 0x60)
}

func TestTheChipIsInSdlcModeOnlyWhenTold(t *testing.T) {
	s := NewSCC8530(0)
	if s.channels[ChannelB].isSdlc() {
		t.Errorf("a chip just reset is in SDLC mode")
	}

	s, _ = localTalkChip(0)
	if !s.channels[ChannelB].isSdlc() {
		t.Errorf("the register 4 of the driver did not put the channel in SDLC mode")
	}
}

func TestAFrameEndsWhenTheTransmitterRunsDry(t *testing.T) {
	s, recorder := localTalkChip(cyclesPerSecondForTest)

	frame := []uint8{0x20, 0x09, 0x01, 0x00, 0x05, 0x01, 0x02, 0x03}
	sendFrame(t, s, frame)

	if len(recorder.frames) != 1 || !bytes.Equal(recorder.frames[0], frame) {
		t.Fatalf("the network got %x, wanted one frame %x", recorder.frames, frame)
	}
}

func TestAFrameDoesNotEndBeforeTheLatchIsReset(t *testing.T) {
	s, recorder := localTalkChip(cyclesPerSecondForTest)

	writeRegister(s, 5, 0x6b)
	s.Write(ChannelB, data, 0x20)
	s.Tick(10000)

	// The transmitter ran dry, but the driver has not said it is the end
	if len(recorder.frames) != 0 {
		t.Errorf("a frame went out before the driver reset the underrun latch")
	}

	s.Write(ChannelB, control, wr0ResetTxUnderrunEom)
	if len(recorder.frames) != 1 {
		t.Errorf("resetting the latch with the transmitter already empty did not end the frame")
	}
}

func TestAFrameTurnedOffHalfWayIsDropped(t *testing.T) {
	s, recorder := localTalkChip(0)

	writeRegister(s, 5, 0x6b)
	s.Write(ChannelB, data, 0x20)
	s.Write(ChannelB, control, wr0ResetTxUnderrunEom|1) // resets, then points somewhere
	s.Read(ChannelB, control)
	recorder.frames = nil

	writeRegister(s, 5, 0x6b)
	s.Write(ChannelB, data, 0x20)
	writeRegister(s, 5, 0x60)
	s.Write(ChannelB, control, wr0ResetTxUnderrunEom)

	if len(recorder.frames) != 0 {
		t.Errorf("a frame whose transmitter was turned off went out: %x", recorder.frames)
	}
}

func TestTheLineIsBusyWhileAFrameGoesBy(t *testing.T) {
	s, _ := localTalkChip(cyclesPerSecondForTest)

	if s.Read(ChannelB, control)&rr0SyncHunt == 0 {
		t.Fatalf("a quiet line reads as busy")
	}

	s.ReceiveFrame(ChannelB, []uint8{0x09, 0x20, 0x01, 0x00, 0x05})
	s.Tick(10)
	if s.Read(ChannelB, control)&rr0SyncHunt != 0 {
		t.Errorf("the line does not read busy with a frame on it")
	}

	// Seven bytes at 230.4kbit/s, and some more
	s.Tick(7 * 300)
	if s.Read(ChannelB, control)&rr0SyncHunt == 0 {
		t.Errorf("the line still reads busy after the frame")
	}
}

// receive reads a whole frame out of the FIFO the way the driver does, with
// the status of each byte
func receive(t *testing.T, s *SCC8530, cyclesPerByte uint64) ([]uint8, []uint8) {
	t.Helper()
	var values, statuses []uint8
	for i := 0; i < 600; i++ {
		s.Tick(cyclesPerByte)
		for s.Read(ChannelB, control)&rr0RxCharAvailable != 0 {
			statuses = append(statuses, readRegister(s, 1))
			values = append(values, s.Read(ChannelB, data))
			if statuses[len(statuses)-1]&rr1EndOfFrame != 0 {
				return values, statuses
			}
		}
	}
	return values, statuses
}

func TestAFrameArrivesWithItsCrcAndTheEndOnTheLastByte(t *testing.T) {
	s, _ := localTalkChip(cyclesPerSecondForTest)

	frame := []uint8{0x09, 0x20, 0x01, 0x00, 0x05, 0x02, 0x02, 0x04}
	s.ReceiveFrame(ChannelB, frame)
	values, statuses := receive(t, s, 100)

	if len(values) != len(frame)+crcLength || !bytes.Equal(values[:len(frame)], frame) {
		t.Fatalf("the FIFO gave %x, wanted %x and two CRC bytes", values, frame)
	}
	for i, status := range statuses {
		last := i == len(statuses)-1
		if (status&rr1EndOfFrame != 0) != last {
			t.Errorf("byte %v has the end of frame %v, wanted %v", i, status&rr1EndOfFrame != 0, last)
		}
	}
	if statuses[len(statuses)-1]&0x40 != 0 {
		t.Errorf("the frame ends with a CRC error")
	}
}

func TestAddressSearchLetsInTheNodeAndTheBroadcast(t *testing.T) {
	s, _ := localTalkChip(0)

	for _, c := range []struct {
		destination uint8
		wanted      bool
	}{{9, true}, {0xff, true}, {7, false}} {
		s.ReceiveFrame(ChannelB, []uint8{c.destination, 0x20, 0x81})
		values, _ := receive(t, s, 1)
		if got := len(values) != 0; got != c.wanted {
			t.Errorf("a frame for node %v reached the FIFO: %v, wanted %v", c.destination, got, c.wanted)
		}
		writeRegister(s, 0, wr0ErrorReset)
	}
}

func TestTheFirstByteRaisesTheReceiveInterrupt(t *testing.T) {
	s, _ := localTalkChip(0)
	s.Write(ChannelB, control, wr0EnableRxNextChar)

	s.ReceiveFrame(ChannelB, []uint8{0x09, 0x20, 0x85})
	s.Tick(1)

	if !s.InterruptAsserted() || readRegister(s, 2) != vectorBRxAvailable {
		t.Fatalf("the first byte of a frame did not raise the receive interrupt")
	}

	// Reading it answers it, and the rest of the frame is polled for
	s.Read(ChannelB, data)
	if s.channels[ChannelB].sdlc.rxInterrupt {
		t.Errorf("the receive interrupt stayed up after the byte was read")
	}

	// The end of the frame is a special condition until an error reset
	receive(t, s, 1)
	if !s.channels[ChannelB].sdlc.specialInterrupt {
		t.Errorf("the end of the frame did not raise the special condition")
	}
	s.Write(ChannelB, control, wr0ErrorReset)
	if s.InterruptAsserted() {
		t.Errorf("the error reset did not clear the special condition")
	}
}

func TestAFrameWaitsForTheReceiverToListen(t *testing.T) {
	s, _ := localTalkChip(0)
	writeRegister(s, 3, 0xd0) // the receiver off, as while transmitting

	s.ReceiveFrame(ChannelB, []uint8{0x09, 0x20, 0x85})
	for i := 0; i < 20; i++ {
		s.Tick(1)
	}
	if s.Read(ChannelB, control)&rr0SyncHunt == 0 {
		t.Fatalf("a frame went onto the wire with the receiver off")
	}

	writeRegister(s, 3, 0xdd)
	values, _ := receive(t, s, 1)
	if len(values) != 3+crcLength {
		t.Errorf("the frame that waited arrived as %x", values)
	}
}

func TestTheMissingClockStatusReadsClear(t *testing.T) {
	s, _ := localTalkChip(0)

	// The driver writes the register 10 with the FM0 bits set, and takes a
	// missing clock in RR10 for a busy line
	if status := readRegister(s, 10); status != 0 {
		t.Errorf("RR10 reads %02x, wanted no missing clock", status)
	}
}

func TestAResetKeepsTheNetwork(t *testing.T) {
	s, recorder := localTalkChip(0)
	s.Reset()
	writeRegister(s, 4, 0x20)
	sendFrame(t, s, []uint8{0xff, 0x09, 0x01})

	if len(recorder.frames) != 1 {
		t.Errorf("the network was unplugged by a reset of the chip")
	}
}

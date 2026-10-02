package component

/*
The synchronous side of the 8530, as much of it as LocalTalk needs: the SDLC
mode the AppleTalk driver of the Macintosh puts the printer port in.

LocalTalk sends frames, not bytes. A frame is opened and closed by flags, its
bits are FM0 encoded, the chip adds a CRC on the way out and checks it on the
way in, and between frames the line is quiet. None of that is modelled bit by
bit. A frame moves whole: what the driver writes to the transmitter is
gathered and handed to a FrameLink when the frame ends, and a frame given to
the chip goes onto a modelled wire, a byte at a time at the 230.4kbit/s of
LocalTalk, into the receive FIFO the driver reads it from. What the driver
looks at on the way is what is kept:

  - RR0 bit 4, Sync/Hunt, is set while the receiver hunts for a flag and
    cleared while a frame is on the wire. The driver reads it as the line
    being busy and backs off, which is all the carrier sense LocalTalk has.
  - RR0 bit 6, Tx Underrun/EOM, is cleared by the driver after the last byte
    of a frame and set again when the transmitter runs dry, which is the
    chip sending the CRC and the closing flag. That is where a frame ends.
  - RR1 bit 7, End of Frame, comes with the last byte of a received frame,
    which is the second of the two CRC bytes the receiver puts in the FIFO
    after the data. The driver reads both, and checks that the first does
    not end the frame and the second does, with no CRC error.
  - Address search, on the bit 2 of the register 3, drops a frame whose first
    byte is neither the address in the register 6 nor the broadcast $ff,
    without an interrupt; the line is busy while it goes past all the same.
  - The receive interrupt comes on the first byte of a frame, and a special
    condition at its end, which the driver clears with an error reset.

The receiver is turned off while the driver transmits, as LocalTalk is half
duplex, so a frame arriving then waits for it to be turned on again, the way a
station waits for a quiet line. The timing comes from the Plus ROM's .MPP, read
in the disassembly, plus/resources/res_drvr_mpp.s.
*/

/*
FrameLink is what is on the other end of a channel in SDLC mode: a network.
The chip hands it every frame it sends, the bytes between the flags without
the CRC.
*/
type FrameLink interface {
	SendFrame(frame []uint8)
}

const (
	// The bits of the register 4 that pick SDLC: the synchronous modes on
	// the bits 3 and 2, and SDLC among them on the bits 5 and 4
	wr4StopBitsMask uint8 = 0x0c
	wr4SyncModeMask uint8 = 0x30
	wr4SdlcMode     uint8 = 0x20

	wr3RxEnable      uint8 = 1 << 0
	wr3AddressSearch uint8 = 1 << 2

	wr5TxEnable uint8 = 1 << 3

	// The receive interrupt mode on the bits 4 and 3 of the register 1
	wr1RxInterruptMask        uint8 = 3 << 3
	wr1RxInterruptFirstChar   uint8 = 1 << 3
	wr1RxInterruptAllChars    uint8 = 2 << 3
	wr1RxInterruptSpecialOnly uint8 = 3 << 3

	// wr15BreakAbortInterrupt asks for the external status interrupt when
	// the line goes quiet after a frame, an abort to the receiver
	wr15BreakAbortInterrupt uint8 = 1 << 7

	// The commands of the register 0 the receiver and the SDLC side answer
	wr0SendAbort          uint8 = 3 << 3
	wr0EnableRxNextChar   uint8 = 4 << 3
	wr0ErrorReset         uint8 = 6 << 3
	wr0ResetCrcMask       uint8 = 3 << 6
	wr0ResetTxUnderrunEom uint8 = 3 << 6

	rr0RxCharAvailable uint8 = 1 << 0
	rr0SyncHunt        uint8 = 1 << 4
	rr0TxUnderrunEom   uint8 = 1 << 6
	rr0BreakAbort      uint8 = 1 << 7

	rr1EndOfFrame uint8 = 1 << 7
	rr1RxOverrun  uint8 = 1 << 5

	// rr1Residue8Bits is the residue code of a frame of whole 8 bit bytes
	rr1Residue8Bits uint8 = 3 << 1

	// The vectors of the receiver of channel B, beside the ones in
	// scc8530.go
	vectorBRxAvailable uint8 = 0x4
	vectorBSpecial     uint8 = 0x6
	vectorARxAvailable uint8 = 0xc
	vectorASpecial     uint8 = 0xe

	// localTalkBitRate is the speed of LocalTalk, the 3.672MHz clock the
	// Macintosh feeds the chip divided by 16
	localTalkBitRate = 230_400

	// rxFifoSize is the receive FIFO of the chip, three bytes deep
	rxFifoSize = 3

	// crcLength is the two bytes of CRC the receiver puts in the FIFO
	// after the data of a frame
	crcLength = 2

	/*
		quietBytes is the quiet before a frame from another station starts on
		the wire, after one the machine received or sent, in byte times:
		about 2ms. On a real wire every frame to one node is a dialog of its
		own, the line idle for 400µs, a random wait, an RTS and a CTS before
		it, and the station on the other end takes its time to answer. The
		AppleTalk of the machine counts on it: a frame that follows the end
		of the last one at once is lost while the driver is still finishing
		with it, and what was lost is retried seconds later. The answers in
		the same dialog, a CTS or an ACK, are the exception, see AnswerFrame.
	*/
	quietBytes = 60

	// maxQueuedFrames is how many frames wait for a receiver that is not
	// listening before the oldest is dropped, as a busy wire would drop it
	maxQueuedFrames = 32
)

// rxEntry is a byte in the receive FIFO with the status that goes with it
type rxEntry struct {
	value      uint8
	endOfFrame bool
}

// sdlc is the synchronous state of a channel
type sdlc struct {
	link FrameLink

	// txFrame is the frame being written, and txUnderrunEom the latch of
	// RR0 bit 6, set while nothing is being sent
	txFrame       []uint8
	txUnderrunEom bool

	// rxQueue is the frames waiting for the wire, rxWire the one on it with
	// its CRC, rxPosition the next byte of it and rxCycles the time until
	// that byte arrives. rxAccepted says the address search let it in.
	rxQueue    [][]uint8
	rxWire     []uint8
	rxPosition int
	rxCycles   uint64
	rxAccepted bool

	// rxQuiet is the time left of the gap after a frame
	rxQuiet uint64

	rxFifo  [rxFifoSize]rxEntry
	rxCount int

	// The receive interrupts. rxArmed is the first character mode waiting
	// for its character.
	rxArmed          bool
	rxInterrupt      bool
	specialInterrupt bool

	// endOfFrame and overrun are the special conditions of RR1, latched
	// until an error reset
	endOfFrame bool
	overrun    bool

	// lineQuiet is RR0 bit 7, the abort the receiver sees on a quiet line
	lineQuiet bool
}

// resetSdlc puts the synchronous side in its power on state, keeping the
// network it is plugged into
func (c *channel) resetSdlc() {
	c.sdlc = sdlc{
		link:          c.sdlc.link,
		txUnderrunEom: true,
		lineQuiet:     true,
	}
}

// isSdlc tells whether the channel is in the SDLC mode
func (c *channel) isSdlc() bool {
	return c.write[4]&wr4StopBitsMask == 0 && c.write[4]&wr4SyncModeMask == wr4SdlcMode
}

/*
AttachLink puts a network on the other end of a channel, for the frames it
sends in SDLC mode. It survives a reset, as a cable does.
*/
func (s *SCC8530) AttachLink(channel int, link FrameLink) {
	s.channels[channel].sdlc.link = link
}

/*
ReceiveFrame puts a frame arriving from the network on the wire of a channel:
the bytes between the flags, without the CRC. It is ignored by a channel that
is not in SDLC mode, which is a port nobody turned AppleTalk on for.
*/
func (s *SCC8530) ReceiveFrame(channel int, frame []uint8) {
	c := &s.channels[channel]
	if !c.isSdlc() || len(frame) == 0 {
		return
	}

	if len(c.sdlc.rxQueue) >= maxQueuedFrames {
		c.sdlc.rxQueue = c.sdlc.rxQueue[1:]
	}
	c.sdlc.rxQueue = append(c.sdlc.rxQueue, append([]uint8(nil), frame...))
}

/*
AnswerFrame puts a frame on the wire of a channel ahead of everything waiting,
and with no quiet before it: the answer to the frame the machine just sent,
which it waits for in the next 200µs and takes nothing else for. That is the
lapCTS to its lapRTS and the lapACK to its lapENQ, which no other station can
come between on a real wire.
*/
func (s *SCC8530) AnswerFrame(channel int, frame []uint8) {
	c := &s.channels[channel]
	if !c.isSdlc() || len(frame) == 0 {
		return
	}
	c.sdlc.rxQueue = append([][]uint8{append([]uint8(nil), frame...)}, c.sdlc.rxQueue...)
	c.sdlc.rxQuiet = 0
}

// sdlcByteCycles is how long a byte takes on LocalTalk
func (c *channel) sdlcByteCycles() uint64 {
	return c.cyclesPerSecond * 8 / localTalkBitRate
}

/*
transmitSdlc takes a byte written to the data register in SDLC mode. It is part
of the frame while the transmitter is on, and paced like any other byte.
*/
func (c *channel) transmitSdlc(value uint8) {
	if c.write[5]&wr5TxEnable == 0 {
		return
	}
	c.sdlc.txFrame = append(c.sdlc.txFrame, value)
}

/*
transmitterDry is called when the last byte has left and nothing waits behind
it. With the latch reset by the driver that is the end of the frame: the chip
sends the CRC and the closing flag, sets the latch again, and the frame is
out. With the latch still set the chip only sends flags, and the frame is
still open.
*/
func (c *channel) transmitterDry() {
	if c.sdlc.txUnderrunEom {
		return
	}
	c.sdlc.txUnderrunEom = true

	frame := c.sdlc.txFrame
	c.sdlc.txFrame = nil
	if len(frame) != 0 && c.sdlc.link != nil {
		// The others answer after a quiet, as after any frame
		c.sdlc.rxQuiet = max(c.sdlc.rxQuiet, quietBytes*c.sdlcByteCycles())
		c.sdlc.link.SendFrame(frame)
	}
}

// abortFrame drops the frame being written, which is what an abort or the
// transmitter turned off in the middle of one does
func (c *channel) abortFrame() {
	c.sdlc.txFrame = nil
}

/*
tickReceive moves the wire along: a frame waiting starts once the receiver is
listening and the gap after the last one has gone by, and the frame on the
wire gives up a byte every byte time.
*/
func (c *channel) tickReceive(cycles uint64) {
	w := &c.sdlc

	if w.rxWire == nil {
		if w.rxQuiet > cycles {
			w.rxQuiet -= cycles
			return
		}
		w.rxQuiet = 0

		if len(w.rxQueue) == 0 || c.write[3]&wr3RxEnable == 0 || len(w.txFrame) != 0 {
			return
		}

		frame := w.rxQueue[0]
		w.rxQueue = w.rxQueue[1:]
		w.rxWire = append(frame, make([]uint8, crcLength)...)
		w.rxPosition = 0
		w.rxCycles = c.sdlcByteCycles()
		w.lineQuiet = false
	}

	for w.rxWire != nil {
		if w.rxCycles > cycles {
			w.rxCycles -= cycles
			return
		}
		cycles -= w.rxCycles
		c.receiveByte()
		if w.rxWire == nil {
			return
		}
		w.rxCycles = c.sdlcByteCycles()
		if w.rxCycles == 0 {
			// A chip that has not been told how fast it runs takes a
			// byte a tick, which is fast enough for the tests
			return
		}
	}
}

// receiveByte takes the next byte of the frame on the wire off it
func (c *channel) receiveByte() {
	w := &c.sdlc
	value := w.rxWire[w.rxPosition]
	last := w.rxPosition == len(w.rxWire)-1

	if w.rxPosition == 0 {
		search := c.write[3]&wr3AddressSearch != 0
		w.rxAccepted = c.write[3]&wr3RxEnable != 0 &&
			(!search || value == c.write[6] || value == 0xff)
	}

	if w.rxAccepted && c.write[3]&wr3RxEnable != 0 {
		c.pushReceived(rxEntry{value: value, endOfFrame: last})
	}

	w.rxPosition++
	if last {
		w.rxWire = nil
		w.rxQuiet = quietBytes * c.sdlcByteCycles()
		w.lineQuiet = true

		if c.write[15]&wr15BreakAbortInterrupt != 0 && c.write[1]&wr1ExternalInterrupt != 0 {
			c.extInterrupt = true
		}
	}
}

// pushReceived puts a byte in the FIFO and raises what it raises
func (c *channel) pushReceived(entry rxEntry) {
	w := &c.sdlc
	if w.rxCount == rxFifoSize {
		w.overrun = true
	} else {
		w.rxFifo[w.rxCount] = entry
		w.rxCount++
	}

	switch c.write[1] & wr1RxInterruptMask {
	case wr1RxInterruptFirstChar:
		if w.rxArmed {
			w.rxInterrupt = true
			w.rxArmed = false
		}
	case wr1RxInterruptAllChars:
		w.rxInterrupt = true
	}

	if (entry.endOfFrame || w.overrun) && c.write[1]&wr1RxInterruptMask != 0 {
		w.specialInterrupt = true
	}
}

// readReceived pops the oldest byte of the FIFO, which is what a read of the
// data register is
func (c *channel) readReceived() uint8 {
	w := &c.sdlc
	if w.rxCount == 0 {
		return 0
	}

	entry := w.rxFifo[0]
	copy(w.rxFifo[:], w.rxFifo[1:])
	w.rxCount--

	if entry.endOfFrame {
		w.endOfFrame = true
	}
	if c.write[1]&wr1RxInterruptMask == wr1RxInterruptFirstChar || w.rxCount == 0 {
		w.rxInterrupt = false
	}
	return entry.value
}

// sdlcStatus is what the synchronous side adds to RR0
func (c *channel) sdlcStatus() uint8 {
	w := &c.sdlc
	var status uint8
	if w.rxCount != 0 {
		status |= rr0RxCharAvailable
	}
	if w.txUnderrunEom {
		status |= rr0TxUnderrunEom
	}
	if c.isSdlc() {
		if w.rxWire == nil {
			status |= rr0SyncHunt
		}
		if w.lineQuiet {
			status |= rr0BreakAbort
		}
	}
	return status
}

/*
receiveStatus is the receiver's half of RR1: the end of frame of the byte at
the head of the FIFO, or latched from the one read before it, and an overrun.
The residue code says the frame was whole bytes, which it always is here.
*/
func (c *channel) receiveStatus() uint8 {
	w := &c.sdlc
	var status uint8
	if w.endOfFrame || (w.rxCount != 0 && w.rxFifo[0].endOfFrame) {
		status |= rr1EndOfFrame | rr1Residue8Bits
	}
	if w.overrun {
		status |= rr1RxOverrun
	}
	return status
}

// sdlcCommand runs the commands of a write to the register 0 that belong to
// the receiver and the SDLC side
func (c *channel) sdlcCommand(value uint8) {
	w := &c.sdlc
	switch value & wr0CommandMask {
	case wr0SendAbort:
		c.abortFrame()
	case wr0EnableRxNextChar:
		w.rxArmed = true
	case wr0ErrorReset:
		w.specialInterrupt = false
		w.endOfFrame = false
		w.overrun = false
	}

	if value&wr0ResetCrcMask == wr0ResetTxUnderrunEom {
		w.txUnderrunEom = false

		// A transmitter already empty underruns at once, and the frame
		// ends there
		if c.txCycles == 0 && !c.txBuffered && len(w.txFrame) != 0 {
			c.transmitterDry()
		}
	}
}

// sdlcRegisterWritten follows a write to one of the registers whose bits
// change what the synchronous side does
func (c *channel) sdlcRegisterWritten(register uint8, previous uint8) {
	switch register {
	case 1:
		// Choosing the first character mode arms it, as the chip does
		if c.write[1]&wr1RxInterruptMask == wr1RxInterruptFirstChar &&
			previous&wr1RxInterruptMask != wr1RxInterruptFirstChar {
			c.sdlc.rxArmed = true
		}
	case 5:
		// The transmitter turned off in the middle of a frame drops it.
		// A frame that ended has gone already and left nothing behind.
		if previous&wr5TxEnable != 0 && c.write[5]&wr5TxEnable == 0 {
			c.abortFrame()
		}
	}
}

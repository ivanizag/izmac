package izmac

import "testing"

/*
The clock reached the way the machine reaches it, over the three low bits of
the VIA port B. What is checked here is the wiring and not the chip: that the
data, the clock and the enable go out on the right pins, and that what the
chip drives back arrives on the data one. The protocol itself is the chip's
business and is tested with the chip.

The two command bytes are written out rather than built, because the builder
is the chip's own and this test is on the other side of that boundary. A
command carries the register on its middle bits with 01 at the bottom, and
the top bit set to read.
*/
const (
	rtcWritePramZero = 0x41 // 0100 0001, write the parameter RAM $00
	rtcReadPramZero  = 0xc1 // 1100 0001, read it back
)

// rtcLines drives the three pins of the clock through the VIA
type rtcLines struct {
	v *via
}

func (l *rtcLines) set(enable bool, clock bool, data bool) {
	var value uint8
	if !enable {
		value |= viaPortBRtcEnable // The enable is active low
	}
	if clock {
		value |= viaPortBRtcClock
	}
	if data {
		value |= viaPortBRtcData
	}
	l.v.poke(viaAddress(viaRegPortB), value)
}

// sending points the data line outwards, receiving lets go of it so that the
// chip can drive it
func (l *rtcLines) sending()   { l.v.poke(viaAddress(viaRegDdrB), 0x07) }
func (l *rtcLines) receiving() { l.v.poke(viaAddress(viaRegDdrB), 0x06) }

func (l *rtcLines) start() {
	l.set(false, true, true)
	l.set(true, true, true)
}

func (l *rtcLines) end() {
	l.set(false, true, true)
}

// sendByte clocks a byte out, high bit first
func (l *rtcLines) sendByte(value uint8) {
	for bit := 7; bit >= 0; bit-- {
		data := value&(1<<bit) != 0
		l.set(true, false, data)
		l.set(true, true, data)
	}
}

// receiveByte clocks a byte in, reading the data line off the port
func (l *rtcLines) receiveByte() uint8 {
	var value uint8
	for bit := 0; bit < 8; bit++ {
		l.set(true, false, true)
		value <<= 1
		if l.v.peek(viaAddress(viaRegPortB))&viaPortBRtcData != 0 {
			value |= 1
		}
		l.set(true, true, true)
	}
	return value
}

func TestTheClockIsReachedThroughTheVia(t *testing.T) {
	v, _, _ := newTestVia(t)
	lines := &rtcLines{v: v}

	// A byte written to the parameter RAM
	lines.sending()
	lines.start()
	lines.sendByte(rtcWritePramZero)
	lines.sendByte(0xa5)
	lines.end()

	// And read back, which only works if both directions are wired
	lines.sending()
	lines.start()
	lines.sendByte(rtcReadPramZero)
	lines.receiving()
	answer := lines.receiveByte()
	lines.end()

	if answer != 0xa5 {
		t.Errorf("the clock answered $%02x through the VIA, wanted $a5", answer)
	}
}

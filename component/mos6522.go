package component

/*
MOS 6522 Versatile Interface Adapter (VIA)
See:

	http://archive.6502.org/datasheets/mos_6522_preliminary_nov_1977.pdf
	http://archive.6502.org/datasheets/rockwell_r6522_via.pdf

Taken from izapple2, where it drives the AY-3-8913 sound generators of the
Mockingboard card. On the Macintosh it carries most of the machine: see
via.go for what each port bit is wired to.

Implemented: ports A and B, timers T1 and T2, the IFR/IER interrupt logic,
the CA/CB control lines as inputs, and the shift register as a byte at a
time rather than bit by bit.
Not implemented: the CA/CB lines as outputs, handshaking and the T2 pulse
counting mode.

Registers:

	0: ORB/IRB   1: ORA/IRA   2: DDRB   3: DDRA
	4: T1C-L     5: T1C-H     6: T1L-L  7: T1L-H
	8: T2C-L     9: T2C-H    10: SR    11: ACR
	12: PCR     13: IFR      14: IER   15: ORA no handshake

The timers advance with Tick(elapsedCycles). Catching up with several
cycles at a time is supported, including multiple T1 underflows in free
running mode.
*/
type MOS6522 struct {
	ora, orb   uint8 // Output registers
	ira, irb   uint8 // Input registers, the values on the input pins
	ddra, ddrb uint8 // Data direction registers, 1 is output

	t1counter int64 // Signed to detect underflows when catching up
	t1latch   uint16
	t1fired   bool
	t2counter int64
	t2latchL  uint8
	t2fired   bool

	sr, acr, pcr uint8
	ifr, ier     uint8

	// The levels on the control pins, to spot the transitions
	ca1, ca2, cb1, cb2 bool

	// shiftedOut is set when the processor has written a byte to the shift
	// register, for the owner of the other end to pick up
	shiftedOut bool
}

const (
	mos6522IntCA2 uint8 = 1 << 0
	mos6522IntCA1 uint8 = 1 << 1
	mos6522IntSR  uint8 = 1 << 2
	mos6522IntCB2 uint8 = 1 << 3
	mos6522IntCB1 uint8 = 1 << 4
	mos6522IntT2  uint8 = 1 << 5
	mos6522IntT1  uint8 = 1 << 6

	mos6522AcrT1FreeRunning uint8 = 1 << 6

	/*
		With this set, an underflow of the timer 1 inverts the top bit of
		the port B instead of only raising the interrupt. The Macintosh
		uses it to make a tone out of a buffer of one repeated value: the
		bit it inverts is the one that enables the sound, so the level is
		switched on and off at the rate of the timer.
	*/
	mos6522AcrT1OnPortB uint8 = 1 << 7

	mos6522PortB7 uint8 = 1 << 7

	// The peripheral control register: the low nibble is for the port A
	// lines and the high one for the port B ones. Bit 0 and bit 4 select
	// the active edge of CA1 and CB1. CA2 and CB2 are outputs when the
	// top bit of their field is set, and their edge is the middle bit.
	mos6522PcrCA1PositiveEdge uint8 = 1 << 0
	mos6522PcrCA2Output       uint8 = 1 << 3
	mos6522PcrCA2PositiveEdge uint8 = 1 << 2
	mos6522PcrCB1PositiveEdge uint8 = 1 << 4
	mos6522PcrCB2Output       uint8 = 1 << 7
	mos6522PcrCB2PositiveEdge uint8 = 1 << 6
)

// Read returns the value of a register
func (v *MOS6522) Read(reg uint8) uint8 {
	switch reg & 0x0f {
	case 0:
		v.ifr &^= mos6522IntCB1 | mos6522IntCB2
		return (v.irb &^ v.ddrb) | (v.orb & v.ddrb)
	case 1:
		// Reaching the port A through the register 1 clears the flags of
		// its control lines. The register 15 is the same port without
		// that side effect, which is why the Macintosh uses it.
		v.ifr &^= mos6522IntCA1 | mos6522IntCA2
		return (v.ira &^ v.ddra) | (v.ora & v.ddra)
	case 15:
		return (v.ira &^ v.ddra) | (v.ora & v.ddra)
	case 2:
		return v.ddrb
	case 3:
		return v.ddra
	case 4:
		v.ifr &^= mos6522IntT1
		return uint8(uint16(v.t1counter))
	case 5:
		return uint8(uint16(v.t1counter) >> 8)
	case 6:
		return uint8(v.t1latch)
	case 7:
		return uint8(v.t1latch >> 8)
	case 8:
		v.ifr &^= mos6522IntT2
		return uint8(uint16(v.t2counter))
	case 9:
		return uint8(uint16(v.t2counter) >> 8)
	case 10:
		v.ifr &^= mos6522IntSR
		return v.sr
	case 11:
		return v.acr
	case 12:
		return v.pcr
	case 13:
		ifr := v.ifr & 0x7f
		if ifr&v.ier != 0 {
			ifr |= 0x80
		}
		return ifr
	case 14:
		return v.ier | 0x80
	}
	return 0
}

// Write sets the value of a register
func (v *MOS6522) Write(reg uint8, value uint8) {
	switch reg & 0x0f {
	case 0:
		v.ifr &^= mos6522IntCB1 | mos6522IntCB2
		v.orb = value
	case 1:
		v.ifr &^= mos6522IntCA1 | mos6522IntCA2
		v.ora = value
	case 15:
		v.ora = value
	case 2:
		v.ddrb = value
	case 3:
		v.ddra = value
	case 4, 6:
		v.t1latch = (v.t1latch & 0xff00) | uint16(value)
	case 5:
		// Load the counter from the latch and start counting
		v.t1latch = (v.t1latch & 0x00ff) | uint16(value)<<8
		v.t1counter = int64(v.t1latch)
		v.t1fired = false
		v.ifr &^= mos6522IntT1
	case 7:
		v.t1latch = (v.t1latch & 0x00ff) | uint16(value)<<8
		v.ifr &^= mos6522IntT1
	case 8:
		v.t2latchL = value
	case 9:
		v.t2counter = int64(value)<<8 | int64(v.t2latchL)
		v.t2fired = false
		v.ifr &^= mos6522IntT2
	case 10:
		v.ifr &^= mos6522IntSR
		v.sr = value
		v.shiftedOut = true
	case 11:
		v.acr = value
	case 12:
		v.pcr = value
	case 13:
		// Writing ones clears the flags
		v.ifr &^= value & 0x7f
	case 14:
		if value&0x80 != 0 {
			v.ier |= value & 0x7f
		} else {
			v.ier &^= value & 0x7f
		}
	}
}

// Tick advances the timers by the elapsed CPU cycles
func (v *MOS6522) Tick(elapsedCycles uint64) {
	v.t1counter -= int64(elapsedCycles)
	if v.t1counter < 0 {
		if v.acr&mos6522AcrT1FreeRunning != 0 {
			// Reload from the latch and interrupt on each underflow
			period := int64(v.t1latch) + 2
			v.t1counter += period * (1 + (-v.t1counter-1)/period)
			v.ifr |= mos6522IntT1
		} else {
			// One shot: interrupt once, the counter rolls over
			if !v.t1fired {
				v.ifr |= mos6522IntT1
				v.t1fired = true
			}
			v.t1counter = int64(uint16(v.t1counter))
		}
	}

	v.t2counter -= int64(elapsedCycles)
	if v.t2counter < 0 {
		// One shot: interrupt once, the counter rolls over
		if !v.t2fired {
			v.ifr |= mos6522IntT2
			v.t2fired = true
		}
		v.t2counter = int64(uint16(v.t2counter))
	}
}

// squareWave inverts the top bit of the port B once for each underflow of
// the timer, which is the tone generator of the Macintosh
func (v *MOS6522) squareWave(underflows int64) {
	if v.acr&mos6522AcrT1OnPortB == 0 {
		return
	}
	if underflows%2 != 0 {
		v.orb ^= mos6522PortB7
	}
}

// InterruptAsserted returns the state of the IRQ output line
func (v *MOS6522) InterruptAsserted() bool {
	return v.ifr&v.ier&0x7f != 0
}

// Reset clears the registers as the RES pin. The timers and the shift
// register are not affected
func (v *MOS6522) Reset() {
	v.ora, v.orb = 0, 0
	v.ddra, v.ddrb = 0, 0
	v.acr, v.pcr = 0, 0
	v.ifr, v.ier = 0, 0
}

// GetPortA returns the values on the port A pins
func (v *MOS6522) GetPortA() uint8 {
	return (v.ora & v.ddra) | (v.ira &^ v.ddra)
}

// GetPortB returns the values on the port B pins
func (v *MOS6522) GetPortB() uint8 {
	return (v.orb & v.ddrb) | (v.irb &^ v.ddrb)
}

// GetDirectionA returns the data direction register of the port A, a 1 bit
// for each pin driven by the chip
func (v *MOS6522) GetDirectionA() uint8 {
	return v.ddra
}

// GetDirectionB returns the data direction register of the port B
func (v *MOS6522) GetDirectionB() uint8 {
	return v.ddrb
}

// SetInputA sets the values on the port A input pins
func (v *MOS6522) SetInputA(value uint8) {
	v.ira = value
}

// SetInputB sets the values on the port B input pins
func (v *MOS6522) SetInputB(value uint8) {
	v.irb = value
}

/*
The shift register, as the Macintosh keyboard uses it. The real chip clocks
the eight bits one at a time against the keyboard clock on CB1, which takes
about three milliseconds a byte. Nothing on this side of it can tell the
difference between that and a byte appearing whole, so the bits are not
emulated: a write hands a byte over, and ShiftIn() hands one back and raises
the interrupt that says a byte has arrived.
*/

// TakeShiftedOut returns the byte the processor last wrote to the shift
// register, and whether there was one waiting
func (v *MOS6522) TakeShiftedOut() (uint8, bool) {
	if !v.shiftedOut {
		return 0, false
	}
	v.shiftedOut = false
	return v.sr, true
}

// ShiftIn puts a byte in the shift register as if it had been clocked in,
// and raises the interrupt for it
func (v *MOS6522) ShiftIn(value uint8) {
	v.sr = value
	v.ifr |= mos6522IntSR
}

// ShiftOutDone raises the shift register interrupt without touching the
// register, which is how the chip reports that the eight bits written to it
// have finished going out
func (v *MOS6522) ShiftOutDone() {
	v.ifr |= mos6522IntSR
}

/*
The control lines as inputs. Each one raises its interrupt flag on the
transition to the level selected by the peripheral control register, so the
caller drives the pin and the chip decides whether that is an active edge.
*/

// SetCA1 drives the CA1 pin. On the Macintosh it is the vertical blanking.
func (v *MOS6522) SetCA1(level bool) {
	v.setControlLine(&v.ca1, level,
		v.pcr&mos6522PcrCA1PositiveEdge != 0, mos6522IntCA1)
}

// SetCA2 drives the CA2 pin, ignored while it is programmed as an output. On
// the Macintosh it is the one second interrupt of the clock.
func (v *MOS6522) SetCA2(level bool) {
	if v.pcr&mos6522PcrCA2Output != 0 {
		v.ca2 = level
		return
	}
	v.setControlLine(&v.ca2, level,
		v.pcr&mos6522PcrCA2PositiveEdge != 0, mos6522IntCA2)
}

// SetCB1 drives the CB1 pin. On the Macintosh it is the keyboard clock.
func (v *MOS6522) SetCB1(level bool) {
	v.setControlLine(&v.cb1, level,
		v.pcr&mos6522PcrCB1PositiveEdge != 0, mos6522IntCB1)
}

// SetCB2 drives the CB2 pin, ignored while it is programmed as an output. On
// the Macintosh it is the keyboard data.
func (v *MOS6522) SetCB2(level bool) {
	if v.pcr&mos6522PcrCB2Output != 0 {
		v.cb2 = level
		return
	}
	v.setControlLine(&v.cb2, level,
		v.pcr&mos6522PcrCB2PositiveEdge != 0, mos6522IntCB2)
}

// setControlLine raises the interrupt flag when the pin moves to the level
// the peripheral control register asks for
func (v *MOS6522) setControlLine(pin *bool, level bool, positiveEdge bool, flag uint8) {
	if *pin == level {
		return
	}
	*pin = level

	if level == positiveEdge {
		v.ifr |= flag
	}
}

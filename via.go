package izmac

import "github.com/ivanizag/izmac/component"

/*
The VIA carries most of the Macintosh Plus. Everything on it raises the
interrupt level 1.

	Port A                        Port B
	  0-2 Sound volume              0 Real time clock data
	    3 Sound buffer select       1 Real time clock clock
	    4 Overlay                   2 Real time clock enable
	    5 Disk head select          3 Mouse switch
	    6 Video page select         4 Mouse X2
	    7 SCC wait/request          5 Mouse Y2
	                                6 Horizontal blanking
	                                7 Sound disable

The control lines are the vertical blanking on CA1, the one second interrupt
of the clock on CA2, and the keyboard clock and data on CB1 and CB2. Those
and the shift register the keyboard uses arrive on the M2 milestone.

The chip is on the last quarter of the address space with only the address
lines 9 to 12 decoded, so a register appears every 512 bytes and the whole
block repeats over the region. The Macintosh reaches the port A through the
register 15, the one without handshaking.
*/
type via struct {
	mos      component.MOS6522
	mm       *memoryManager
	video    *video
	iwm      *iwm
	rtc      *component.AppleRTC
	keyboard *keyboard
	mouse    *mouse
	sound    *sound

	// eClockRemainder accumulates the processor cycles not yet passed to
	// the chip, which is clocked at a tenth of the processor
	eClockRemainder uint64

	// The quadrature lines of the mouse, as the pins see them
	mouseX2 bool
	mouseY2 bool
}

const (
	// viaClockDivider is the ratio between the processor clock and the E
	// clock that drives the VIA, 7.8336MHz over 783.36KHz
	viaClockDivider = 10

	// viaIntShiftRegister is the interrupt flag of the shift register, which
	// the Macintosh uses for the keyboard
	viaIntShiftRegister uint8 = 1 << 2

	viaPortBRtcData   uint8 = 1 << 0
	viaPortBRtcClock  uint8 = 1 << 1
	viaPortBRtcEnable uint8 = 1 << 2

	viaPortBMouseSwitch uint8 = 1 << 3
	viaPortBMouseX2     uint8 = 1 << 4
	viaPortBMouseY2     uint8 = 1 << 5

	viaPortASoundVolume uint8 = 7 << 0
	viaPortASoundPage   uint8 = 1 << 3
	viaPortBSoundEnable uint8 = 1 << 7

	viaPortAOverlay    uint8 = 1 << 4
	viaPortAHeadSelect uint8 = 1 << 5
	viaPortAVideoPage  uint8 = 1 << 6

	// viaPortAResetInputs is what the port A pins read while they are
	// inputs, as they are after a reset. The overlay and the main video
	// page are selected by the pull ups until the ROM drives them.
	viaPortAResetInputs uint8 = 0xff

	// viaPortBResetInputs is the same for the port B. What matters of it
	// so far is that the mouse switch on the bit 3 reads high, which is
	// the button not pressed: the ROM ejects the disk when it is held
	// down at the power on. The clock drives the bit 0 over this.
	viaPortBResetInputs uint8 = 0xff
)

func newVia(mm *memoryManager, v *video, d *iwm, c *component.AppleRTC, k *keyboard, mo *mouse, so *sound) *via {
	via := &via{mm: mm, video: v, iwm: d, rtc: c, keyboard: k, mouse: mo, sound: so}
	via.reset()
	return via
}

func (v *via) reset() {
	v.mos.Reset()
	v.mos.SetInputA(viaPortAResetInputs)
	v.mos.SetInputB(viaPortBResetInputs)
	v.eClockRemainder = 0
	v.applyPortA()
	v.applyPortB()
}

// setVBlank drives CA1, the vertical blanking the whole system runs on. The
// chip decides which transition raises the interrupt from the peripheral
// control register the ROM sets up.
func (v *via) setVBlank(blanking bool) {
	v.mos.SetCA1(blanking)
}

// setOneSecond drives CA2, the one second interrupt of the real time clock
func (v *via) setOneSecond(level bool) {
	v.mos.SetCA2(level)
}

// viaRegister returns the register an address reaches. Only the address
// lines 9 to 12 are decoded.
func viaRegister(address uint32) uint8 {
	return uint8((address >> 9) & 0x0f)
}

func (v *via) peek(address uint32) uint8 {
	reg := viaRegister(address)

	if reg == 0 {
		// The clock may be holding the data line down, refresh it before
		// the processor reads the port
		v.refreshPortBInputs()

		// And this read is the mouse handler taking the quadrature that
		// goes with the edge it was called for
		v.mouse.quadratureRead()
	}

	return v.mos.Read(reg)
}

func (v *via) poke(address uint32, value uint8) {
	reg := viaRegister(address)
	v.mos.Write(reg, value)

	// The output latches and the data direction registers change what the
	// pins drive
	switch reg {
	case 1, 3, 15:
		v.applyPortA()
	case 0, 2:
		v.applyPortB()
	case 10:
		// A byte shifted out is a command for the keyboard
		if command, sent := v.mos.TakeShiftedOut(); sent {
			v.keyboard.command(command)
		}
	}
}

// applyPortA propagates the port A pins to what they are wired to. A pin
// configured as an input keeps the value the hardware puts on it.
func (v *via) applyPortA() {
	portA := v.mos.GetPortA()

	v.mm.setOverlay(portA&viaPortAOverlay != 0)
	v.video.setAlternatePage(portA&viaPortAVideoPage == 0)
	v.iwm.setHeadSelect(portA&viaPortAHeadSelect != 0)

	v.sound.setVolume(portA & viaPortASoundVolume)
	v.sound.setAlternateBuffer(portA&viaPortASoundPage == 0)
}

/*
applyPortB propagates the port B pins. The three low bits are the serial
interface of the real time clock, and the data line is bidirectional: the
processor drives it while sending and lets go of it to read the answer, which
is what the data direction register says.
*/
func (v *via) applyPortB() {
	portB := v.mos.GetPortB()
	driving := v.mos.GetDirectionB()&viaPortBRtcData != 0

	v.rtc.SetLines(
		portB&viaPortBRtcEnable == 0, // The enable is active low
		portB&viaPortBRtcClock != 0,
		portB&viaPortBRtcData != 0,
		driving,
	)

	v.applySoundEnable(portB)
	v.refreshPortBInputs()
}

// applySoundEnable takes the top bit of the port B, which is zero for the
// sound on. It is checked after a write to the port and after the timers run,
// because the timer 1 can invert that bit by itself to make a tone.
func (v *via) applySoundEnable(portB uint8) {
	v.sound.setEnabled(portB&viaPortBSoundEnable == 0)
}

/*
refreshPortBInputs puts what the clock and the mouse are driving on the input
pins. The mouse switch is active low, so a pressed button pulls its bit down.
*/
func (v *via) refreshPortBInputs() {
	inputB := viaPortBResetInputs

	if !v.rtc.DataOut() {
		inputB &^= viaPortBRtcData
	}
	if v.mouse.button {
		inputB &^= viaPortBMouseSwitch
	}
	if !v.mouseX2 {
		inputB &^= viaPortBMouseX2
	}
	if !v.mouseY2 {
		inputB &^= viaPortBMouseY2
	}

	v.mos.SetInputB(inputB)
}

// setMouseQuadrature takes the two lines of the mouse that arrive here
func (v *via) setMouseQuadrature(x2 bool, y2 bool) {
	v.mouseX2 = x2
	v.mouseY2 = y2
	v.refreshPortBInputs()
}

/*
tick advances the timers, converting the processor cycles to the E clock the
chip runs on, and lets the keyboard answer when its byte is due.

The port B is read back afterwards because the timer can invert its top bit
by itself, which is how the machine makes a tone, and that bit is the one
that enables the sound.
*/
func (v *via) tick(cycles uint64) {
	answer, answered, sent := v.keyboard.tick(cycles)
	switch {
	case sent:
		// The command has finished going out, which is what tells the
		// Macintosh to turn the shift register around and listen
		v.mos.ShiftOutDone()
	case answered:
		v.mos.ShiftIn(answer)
	}

	v.eClockRemainder += cycles
	eCycles := v.eClockRemainder / viaClockDivider
	if eCycles != 0 {
		v.eClockRemainder -= eCycles * viaClockDivider
		v.mos.Tick(eCycles)

		v.applySoundEnable(v.mos.GetPortB())
	}
}

// interruptAsserted returns the state of the IRQ line of the chip
func (v *via) interruptAsserted() bool {
	return v.mos.InterruptAsserted()
}

package izmac

/*
The sound of the Macintosh Plus, which is a buffer in RAM read by the
hardware rather than a chip that makes tones.

There are 370 words at the top of the RAM, one for each scan line of a frame,
and the circuit takes one of them every 44.93 microseconds as the beam goes
down the screen. That works out at 22254 samples a second and it costs
nothing here, because the scan line tick that drives the vertical blanking is
already counted out by the run loop.

Only the high byte of each word is the sound. The low byte is the speed the
disk motor should run at, which shares the buffer, and Inside Macintosh warns
against writing anything there for that reason.

	VIA port A bits 0 to 2   the volume, eight steps
	VIA port A bit 3         which of the two buffers is read
	VIA port B bit 7         zero enables the sound

The value is unsigned around a middle of 128, so silence is a buffer full of
$80 and not one full of zeros. A buffer of zeros is the loudest sound the
machine can make in one direction, which is worth knowing when the screen is
blank and the speaker is not.
*/
type sound struct {
	mm   *memoryManager
	sink AudioSink

	// alternate selects the second buffer, from the VIA port A bit 3
	alternate bool

	// volume is the three bits of the VIA port A, 0 to 7
	volume uint8

	// enabled is the VIA port B bit 7, inverted: a zero there is sound on
	enabled bool
}

const (
	// soundSamplesPerFrame is one for each scan line, whether it is drawn
	// or not
	soundSamplesPerFrame = linesPerFrame

	// SoundSampleRateHz is what that works out at, 22254 a second, and what
	// a frontend has to resample from
	SoundSampleRateHz = CPUClockMhz * 1_000_000 / cyclesPerLine

	// soundMidpoint is the value of silence, the samples being unsigned
	soundMidpoint = 128

	// soundMaxVolume is the loudest of the eight steps
	soundMaxVolume = 7
)

/*
AudioSink takes the samples the machine produces, one for every scan line. It
is implemented by the frontends, which resample from the 22254 a second the
machine makes to whatever the sound device of the host wants.
*/
type AudioSink interface {
	PushSample(sample float32)
}

func newSound(mm *memoryManager) *sound {
	return &sound{mm: mm}
}

// SetAudioSink attaches a frontend to the sound of the machine
func (m *Mac) SetAudioSink(sink AudioSink) {
	m.sound.sink = sink
}

// setVolume takes the three bits of the VIA port A
func (s *sound) setVolume(volume uint8) {
	s.volume = volume & soundMaxVolume
}

// setAlternateBuffer picks between the two buffers, from the VIA port A bit 3
func (s *sound) setAlternateBuffer(alternate bool) {
	s.alternate = alternate
}

// setEnabled takes the VIA port B bit 7, which is zero for sound on
func (s *sound) setEnabled(enabled bool) {
	s.enabled = enabled
}

// base is where the buffer in use starts
func (s *sound) base() uint32 {
	if s.alternate {
		return s.mm.ramTop() - soundAlternateOffset
	}
	return s.mm.ramTop() - soundMainOffset
}

/*
tick hands over the sample for one scan line. The line is the index into the
buffer, so the sound follows the beam down the screen exactly as it does on
the hardware.
*/
func (s *sound) tick(line int) {
	if s.sink == nil {
		return
	}
	if !s.enabled || s.volume == 0 {
		s.sink.PushSample(0)
		return
	}
	if line < 0 || line >= soundSamplesPerFrame {
		return
	}

	// The high byte of the word is the sound, the low one the disk speed
	value := s.mm.ram[(s.base()+uint32(line)*2)&s.mm.ramMask]

	level := (float32(value) - soundMidpoint) / soundMidpoint
	s.sink.PushSample(level * float32(s.volume) / soundMaxVolume)
}

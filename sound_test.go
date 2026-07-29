package izmac

import "testing"

// recordingSink keeps what the machine played
type recordingSink struct {
	samples []float32
}

func (r *recordingSink) PushSample(sample float32) {
	r.samples = append(r.samples, sample)
}

func (r *recordingSink) loudest() float32 {
	loudest := float32(0)
	for _, s := range r.samples {
		if s > loudest {
			loudest = s
		} else if -s > loudest {
			loudest = -s
		}
	}
	return loudest
}

func newTestSound(t *testing.T) (*sound, *memoryManager, *recordingSink) {
	t.Helper()

	mm := newTestMemoryManager(1024)
	s := newSound(mm)
	sink := &recordingSink{}

	s.sink = sink
	s.setEnabled(true)
	s.setVolume(soundMaxVolume)

	return s, mm, sink
}

// fillBuffer writes a value into the high byte of every word of the buffer,
// which is the part that is the sound
func fillBuffer(mm *memoryManager, base uint32, value uint8) {
	for line := 0; line < soundSamplesPerFrame; line++ {
		mm.ram[(base+uint32(line)*2)&mm.ramMask] = value
	}
}

func TestSilenceIsTheMiddleAndNotZero(t *testing.T) {
	s, mm, sink := newTestSound(t)

	// A buffer of the midpoint is silence
	fillBuffer(mm, mm.ramTop()-soundMainOffset, soundMidpoint)
	for line := 0; line < soundSamplesPerFrame; line++ {
		s.tick(line)
	}

	if got := sink.loudest(); got != 0 {
		t.Errorf("a buffer of $%02x played at a level of %v, wanted silence",
			soundMidpoint, got)
	}

	// And a buffer of zeros is not
	sink.samples = sink.samples[:0]
	fillBuffer(mm, mm.ramTop()-soundMainOffset, 0)
	for line := 0; line < soundSamplesPerFrame; line++ {
		s.tick(line)
	}

	if sink.loudest() == 0 {
		t.Error("a buffer of zeros played as silence, it is the loudest one way")
	}
}

func TestTheVolumeScalesTheLevel(t *testing.T) {
	s, mm, sink := newTestSound(t)
	fillBuffer(mm, mm.ramTop()-soundMainOffset, 0)

	levels := make(map[uint8]float32)
	for volume := uint8(0); volume <= soundMaxVolume; volume++ {
		sink.samples = sink.samples[:0]
		s.setVolume(volume)
		s.tick(0)
		levels[volume] = sink.loudest()
	}

	if levels[0] != 0 {
		t.Errorf("the volume 0 played at %v, wanted silence", levels[0])
	}
	for volume := uint8(1); volume <= soundMaxVolume; volume++ {
		if levels[volume] <= levels[volume-1] {
			t.Errorf("the volume %v is not louder than %v", volume, volume-1)
		}
	}
}

func TestTheSoundCanBeTurnedOff(t *testing.T) {
	s, mm, sink := newTestSound(t)
	fillBuffer(mm, mm.ramTop()-soundMainOffset, 0)

	s.setEnabled(false)
	for line := 0; line < soundSamplesPerFrame; line++ {
		s.tick(line)
	}

	if got := sink.loudest(); got != 0 {
		t.Errorf("the sound played at %v with the enable off", got)
	}
	if len(sink.samples) != soundSamplesPerFrame {
		t.Error("turning the sound off stopped the samples rather than silencing them")
	}
}

func TestTheAlternateBufferIsPlayed(t *testing.T) {
	s, mm, sink := newTestSound(t)

	fillBuffer(mm, mm.ramTop()-soundMainOffset, soundMidpoint)
	fillBuffer(mm, mm.ramTop()-soundAlternateOffset, 0)

	s.setAlternateBuffer(true)
	s.tick(0)

	if sink.loudest() == 0 {
		t.Error("the main buffer was played with the alternate one selected")
	}
}

/*
Only the high byte of each word is the sound. The low one is the speed of the
disk motor, which shares the buffer, so writing there must not be audible.
*/
func TestTheLowByteOfEachWordIsNotTheSound(t *testing.T) {
	s, mm, sink := newTestSound(t)
	base := mm.ramTop() - soundMainOffset

	fillBuffer(mm, base, soundMidpoint)
	for line := 0; line < soundSamplesPerFrame; line++ {
		mm.ram[(base+uint32(line)*2+1)&mm.ramMask] = 0xff
	}

	for line := 0; line < soundSamplesPerFrame; line++ {
		s.tick(line)
	}

	if got := sink.loudest(); got != 0 {
		t.Errorf("the disk speed bytes played at a level of %v", got)
	}
}

// One sample for each scan line is what makes the rate come out right
func TestOneSamplePerScanLine(t *testing.T) {
	s, mm, sink := newTestSound(t)
	fillBuffer(mm, mm.ramTop()-soundMainOffset, soundMidpoint)

	const frames = 3
	for frame := 0; frame < frames; frame++ {
		for line := 0; line < soundSamplesPerFrame; line++ {
			s.tick(line)
		}
	}

	if len(sink.samples) != frames*soundSamplesPerFrame {
		t.Errorf("%v frames gave %v samples, wanted %v",
			frames, len(sink.samples), frames*soundSamplesPerFrame)
	}

	if rate := soundSampleRateHz; rate < 22250 || rate > 22260 {
		t.Errorf("the rate works out at %v, wanted about 22254", rate)
	}
}

// The VIA carries the volume, the buffer and the enable, so the machine has
// to reach the sound through it
func TestTheViaDrivesTheSound(t *testing.T) {
	v, mm, _ := newTestVia(t)
	sink := &recordingSink{}
	v.sound.sink = sink

	fillBuffer(mm, mm.ramTop()-soundMainOffset, 0)

	// The ports are outputs, then the volume and the enable are set
	v.poke(viaAddress(viaRegDdrA), 0xff)
	v.poke(viaAddress(viaRegDdrB), 0xff)
	v.poke(viaAddress(viaRegPortA), viaPortASoundVolume|viaPortASoundPage)
	v.poke(viaAddress(viaRegPortB), 0)

	if v.sound.volume != soundMaxVolume {
		t.Errorf("the volume reads %v, wanted %v", v.sound.volume, soundMaxVolume)
	}
	if !v.sound.enabled {
		t.Error("the sound is off with the enable bit low")
	}

	v.sound.tick(0)
	if sink.loudest() == 0 {
		t.Error("nothing was played through the VIA")
	}

	// And the top bit of the port B silences it
	sink.samples = sink.samples[:0]
	v.poke(viaAddress(viaRegPortB), viaPortBSoundEnable)
	v.sound.tick(0)
	if sink.loudest() != 0 {
		t.Error("the enable bit did not silence the sound")
	}
}

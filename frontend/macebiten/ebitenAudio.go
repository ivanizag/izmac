package main

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2/audio"

	"github.com/ivanizag/izmac"
	izaudio "github.com/ivanizag/izmac/audio"
)

/*
The sound of the machine handed to the ebiten player.

The machine makes one sample for every scan line, 22254 a second, and the
player wants them at its own rate, so a stream sits between the two and
resamples. The machine puts samples in from the emulation goroutine and the
player takes them out from its own, which the stream is prepared for.
*/
type ebitenAudio struct {
	stream *izaudio.Stream
	player *audio.Player

	// samples is reused between reads rather than allocated each time,
	// because Read is called often and by the sound device
	samples []float32
}

const (
	// audioSampleRate is what the player runs at
	audioSampleRate = 48000

	/*
		audioQueueSamples is how much of the machine's output the stream
		holds. A quarter of a second is enough to ride out the emulation
		stopping for a moment and short enough that the sound does not
		noticeably lag the picture.
	*/
	audioQueueSamples = 22254 / 4

	// bytesPerSample is two channels of a float32 each
	bytesPerSample = 8
)

func newEbitenAudio(m *izmac.Mac) (*ebitenAudio, error) {
	s := &ebitenAudio{
		stream: izaudio.NewStream(
			izmac.SoundSampleRateHz(), audioSampleRate, audioQueueSamples),
	}

	context := audio.NewContext(audioSampleRate)
	player, err := context.NewPlayerF32(s)
	if err != nil {
		return nil, err
	}
	s.player = player

	m.SetAudioSink(s)
	return s, nil
}

// PushSample takes a sample from the machine. It is the izmac.AudioSink.
func (s *ebitenAudio) PushSample(sample float32) {
	s.stream.Push(sample)
}

// Read fills the buffer of the player. It is an io.Reader of pairs of
// float32, one for each channel, as ebiten wants them.
func (s *ebitenAudio) Read(buffer []byte) (int, error) {
	wanted := len(buffer) / bytesPerSample
	if wanted == 0 {
		return 0, nil
	}

	if cap(s.samples) < wanted {
		s.samples = make([]float32, wanted)
	}
	s.samples = s.samples[:wanted]
	s.stream.Read(s.samples)

	for i, sample := range s.samples {
		// The machine has one speaker, so both channels get the same
		putFloat32(buffer, i*2, sample)
		putFloat32(buffer, i*2+1, sample)
	}

	return wanted * bytesPerSample, nil
}

func (s *ebitenAudio) start() {
	s.player.Play()
}

// putFloat32 writes a sample in the little endian order ebiten reads
func putFloat32(buffer []byte, index int, sample float32) {
	bits := math.Float32bits(sample)
	at := index * 4

	buffer[at] = byte(bits)
	buffer[at+1] = byte(bits >> 8)
	buffer[at+2] = byte(bits >> 16)
	buffer[at+3] = byte(bits >> 24)
}

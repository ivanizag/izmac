// Package audio carries the samples the machine generates to a frontend.
package audio

import "sync"

/*
Stream is a queue of samples with a resampler on the way out.

The Macintosh produces one sample per scan line, 22254 a second, and a host
sound device wants some round number like 44100 or 48000, so the two have to
be reconciled. Samples go in at the rate the machine makes them and come out
at the rate the device asks for, interpolated between the two nearest.

The two ends are different goroutines, the emulation putting samples in and
the sound device taking them out, so the queue is locked.

Neither end waits for the other. If the emulation falls behind, the last
sample is held rather than a gap being left, which is quieter than silence.
If it runs ahead, which it does at full speed, the oldest samples are dropped:
the alternative is for the sound to lag further and further behind the
picture.
*/
type Stream struct {
	mutex sync.Mutex

	samples []float32
	first   int
	count   int

	// step is how far to move along the samples for each one asked for
	step float64

	// position is where in the queue the next sample comes from, kept
	// between calls so the interpolation does not jump
	position float64

	// last is held when the queue runs dry
	last float32
}

// NewStream builds a queue that takes samples at one rate and gives them out
// at another. The capacity is how many source samples it holds before the
// oldest are dropped.
func NewStream(sourceRate float64, outputRate float64, capacity int) *Stream {
	if capacity < 2 {
		capacity = 2
	}

	return &Stream{
		samples: make([]float32, capacity),
		step:    sourceRate / outputRate,
	}
}

// Push adds a sample, dropping the oldest when the queue is full
func (s *Stream) Push(sample float32) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.count == len(s.samples) {
		s.dropOldest()
	}

	s.samples[(s.first+s.count)%len(s.samples)] = sample
	s.count++
}

// Read fills a buffer at the output rate
func (s *Stream) Read(out []float32) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	for i := range out {
		out[i] = s.next()
	}
}

// next takes one sample at the output rate, interpolating between the two
// nearest of the source
func (s *Stream) next() float32 {
	index := int(s.position)

	// Two samples are needed to interpolate between
	if index+1 >= s.count {
		return s.last
	}

	fraction := float32(s.position - float64(index))
	a := s.at(index)
	b := s.at(index + 1)
	s.last = a + (b-a)*fraction

	s.position += s.step
	for s.position >= 1 && s.count > 0 {
		s.position--
		s.dropOldest()
	}

	return s.last
}

func (s *Stream) at(index int) float32 {
	return s.samples[(s.first+index)%len(s.samples)]
}

func (s *Stream) dropOldest() {
	s.first = (s.first + 1) % len(s.samples)
	s.count--
}

// Waiting returns how many samples are queued, for a frontend that wants to
// know whether the emulation is keeping up
func (s *Stream) Waiting() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.count
}

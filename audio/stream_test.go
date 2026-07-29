package audio

import "testing"

func TestSamplesComeOutInOrder(t *testing.T) {
	// The same rate in and out, so nothing is interpolated away
	s := NewStream(1000, 1000, 64)

	pushed := []float32{0.1, 0.2, 0.3, 0.4}
	for _, sample := range pushed {
		s.Push(sample)
	}

	out := make([]float32, len(pushed)-1)
	s.Read(out)

	for i, got := range out {
		if got != pushed[i] {
			t.Errorf("the sample %v came out as %v, wanted %v", i, got, pushed[i])
		}
	}
}

// Reading faster than the machine plays takes fewer source samples, and the
// other way round takes more, which is the whole point of the resampling
func TestTheRatesAreReconciled(t *testing.T) {
	for _, c := range []struct {
		name       string
		source     float64
		output     float64
		pushed     int
		wantedLeft int
	}{
		// Reading 20 samples at twice the source rate is ten of the
		// source, and at half the rate it is forty
		{"the device runs twice as fast", 1000, 2000, 40, 30},
		{"the machine runs twice as fast", 2000, 1000, 40, 0},
	} {
		s := NewStream(c.source, c.output, 128)
		for i := 0; i < c.pushed; i++ {
			s.Push(float32(i) / float32(c.pushed))
		}

		out := make([]float32, 20)
		s.Read(out)

		if left := s.waiting(); left < c.wantedLeft-2 || left > c.wantedLeft+2 {
			t.Errorf("%v: %v samples left of %v, wanted about %v",
				c.name, left, c.pushed, c.wantedLeft)
		}
	}
}

func TestTheValuesAreInterpolated(t *testing.T) {
	// Half the output rate of the source, so every other sample lands
	// between two of them
	s := NewStream(1000, 2000, 64)
	s.Push(0)
	s.Push(1)
	s.Push(0)

	out := make([]float32, 3)
	s.Read(out)

	if out[0] != 0 {
		t.Errorf("the first sample is %v, wanted 0", out[0])
	}
	if out[1] <= 0 || out[1] >= 1 {
		t.Errorf("the sample between 0 and 1 is %v, it was not interpolated", out[1])
	}
}

/*
Neither end waits for the other. Running dry holds the last value rather than
leaving a gap, which is quieter than silence, and running over drops the
oldest so that the sound does not fall further and further behind.
*/
func TestRunningDryHoldsTheLastValue(t *testing.T) {
	s := NewStream(1000, 1000, 64)
	s.Push(0.5)
	s.Push(0.5)

	out := make([]float32, 16)
	s.Read(out)

	for i, got := range out {
		if got != 0.5 {
			t.Errorf("the sample %v is %v after the queue ran dry, wanted 0.5", i, got)
		}
	}
}

func TestRunningOverDropsTheOldest(t *testing.T) {
	const capacity = 8
	s := NewStream(1000, 1000, capacity)

	for i := 0; i < capacity*4; i++ {
		s.Push(float32(i))
	}

	if waiting := s.waiting(); waiting != capacity {
		t.Errorf("%v samples are queued, wanted the capacity of %v", waiting, capacity)
	}

	// What is left is the newest, not the oldest
	out := make([]float32, 1)
	s.Read(out)
	if out[0] < float32(capacity*3) {
		t.Errorf("the oldest sample left is %v, the newest ones were dropped", out[0])
	}
}

func TestAnEmptyStreamIsSilent(t *testing.T) {
	s := NewStream(1000, 1000, 8)

	out := make([]float32, 4)
	for i := range out {
		out[i] = 99
	}
	s.Read(out)

	for i, got := range out {
		if got != 0 {
			t.Errorf("the sample %v of an empty stream is %v, wanted silence", i, got)
		}
	}
}

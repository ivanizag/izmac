package screen

import (
	"image/color"
	"strings"
	"testing"
)

// testSource is a frame buffer that can be drawn on directly
type testSource struct {
	buffer []uint8
}

func newTestSource() *testSource {
	return &testSource{buffer: make([]uint8, FrameBufferSize)}
}

func (s *testSource) GetFrameBuffer() []uint8 {
	return s.buffer
}

func (s *testSource) set(x int, y int) {
	s.buffer[y*BytesPerLine+x/8] |= 0x80 >> (x % 8)
}

func TestSnapshotSize(t *testing.T) {
	img := Snapshot(newTestSource())

	if img.Rect.Dx() != Width || img.Rect.Dy() != Height {
		t.Errorf("the snapshot is %vx%v, wanted %vx%v",
			img.Rect.Dx(), img.Rect.Dy(), Width, Height)
	}
}

func TestASetBitIsBlack(t *testing.T) {
	s := newTestSource()
	s.set(0, 0)
	s.set(511, 341)

	img := Snapshot(s)
	black := color.RGBA{0x00, 0x00, 0x00, 0xff}
	white := color.RGBA{0xff, 0xff, 0xff, 0xff}

	if img.RGBAAt(0, 0) != black {
		t.Error("a set bit is not black")
	}
	if img.RGBAAt(511, 341) != black {
		t.Error("the last pixel of the buffer is not the bottom right one")
	}
	if img.RGBAAt(1, 0) != white {
		t.Error("a clear bit is not white")
	}
}

func TestTheMostSignificantBitIsOnTheLeft(t *testing.T) {
	s := newTestSource()
	s.buffer[0] = 0x80

	img := Snapshot(s)
	if img.RGBAAt(0, 0) != (color.RGBA{0x00, 0x00, 0x00, 0xff}) {
		t.Error("the most significant bit is not the leftmost pixel")
	}
}

func TestSnapshotTextShape(t *testing.T) {
	const scale = 8
	text := SnapshotText(newTestSource(), scale)

	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) != Height/scale {
		t.Errorf("the dump has %v lines, wanted %v", len(lines), Height/scale)
	}
	if len(lines[0]) != Width/scale {
		t.Errorf("the dump has %v columns, wanted %v", len(lines[0]), Width/scale)
	}
}

func TestSnapshotTextShadesByCoverage(t *testing.T) {
	empty := SnapshotText(newTestSource(), 8)
	if strings.ContainsAny(empty, "@#%") {
		t.Error("an empty screen dumped dark shades")
	}

	s := newTestSource()
	for i := range s.buffer {
		s.buffer[i] = 0xff
	}
	full := SnapshotText(s, 8)
	if strings.Contains(full, " ") {
		t.Error("a full screen dumped light shades")
	}
}

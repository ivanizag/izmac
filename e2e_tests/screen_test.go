package e2e_tests

import (
	"github.com/ivanizag/izmac"
)

// The screen of the Plus, and the bytes of a line of it, a bit a pixel
const (
	width        = 512
	height       = 342
	bytesPerLine = width / 8
)

/*
screenBytes is the screen as the Plus keeps it, a bit a pixel with the leftmost
the top bit of its byte and set for black, made out of a screenshot. The tests
that compare screens count what changed in bytes of this, which is how much of
the screen a change took on the machine.
*/
func screenBytes(m *izmac.Mac) []uint8 {
	shot := m.Screenshot()
	bits := make([]uint8, height*bytesPerLine)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if shot.RGBAAt(x, y).R < 0x80 {
				bits[y*bytesPerLine+x/8] |= 0x80 >> (x % 8)
			}
		}
	}
	return bits
}

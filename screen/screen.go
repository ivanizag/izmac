// Package screen renders the frame buffer of the Macintosh Plus.
//
// The Plus has no CRTC. The frame buffer is a plain bitmap of 512 by 342
// pixels, one bit per pixel, most significant bit to the left, and a bit set
// means a black pixel.
package screen

import (
	"image"
	"image/color"
	"strings"
)

const (
	// Width and Height of the Macintosh Plus screen in pixels
	Width  = 512
	Height = 342

	// BytesPerLine is the length of a scan line in the frame buffer
	BytesPerLine = Width / 8

	// FrameBufferSize is the length of the frame buffer in bytes
	FrameBufferSize = BytesPerLine * Height
)

// VideoSource is implemented by the emulated machine to give access to the
// frame buffer currently selected
type VideoSource interface {
	// GetFrameBuffer returns the FrameBufferSize bytes of the active page
	GetFrameBuffer() []uint8
}

// Snapshot returns the screen as an image. A set bit is black, as on the
// hardware.
func Snapshot(vs VideoSource) *image.RGBA {
	white := color.RGBA{0xff, 0xff, 0xff, 0xff}
	black := color.RGBA{0x00, 0x00, 0x00, 0xff}

	buffer := vs.GetFrameBuffer()
	img := image.NewRGBA(image.Rect(0, 0, Width, Height))

	for y := 0; y < Height; y++ {
		for xByte := 0; xByte < BytesPerLine; xByte++ {
			bits := buffer[y*BytesPerLine+xByte]
			for bit := 0; bit < 8; bit++ {
				c := white
				if bits&(0x80>>bit) != 0 {
					c = black
				}
				img.Set(xByte*8+bit, y, c)
			}
		}
	}

	return img
}

// SnapshotText renders the screen as ASCII art, scaled down by the given
// factor. It is how the headless frontend shows what the ROM has drawn
// without opening a window.
func SnapshotText(vs VideoSource, scale int) string {
	if scale < 1 {
		scale = 1
	}

	buffer := vs.GetFrameBuffer()
	var sb strings.Builder

	for y := 0; y+scale <= Height; y += scale {
		for x := 0; x+scale <= Width; x += scale {
			sb.WriteByte(shadeOf(coverage(buffer, x, y, scale)))
		}
		sb.WriteByte('\n')
	}

	return sb.String()
}

// coverage returns the ratio of black pixels on a square of the frame buffer
func coverage(buffer []uint8, x int, y int, scale int) float64 {
	set := 0
	for dy := 0; dy < scale; dy++ {
		for dx := 0; dx < scale; dx++ {
			px := x + dx
			bits := buffer[(y+dy)*BytesPerLine+px/8]
			if bits&(0x80>>(px%8)) != 0 {
				set++
			}
		}
	}
	return float64(set) / float64(scale*scale)
}

func shadeOf(ratio float64) byte {
	shades := " .:-=+*#%@"
	i := int(ratio * float64(len(shades)))
	if i >= len(shades) {
		i = len(shades) - 1
	}
	return shades[i]
}

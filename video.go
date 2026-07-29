package izmac

import "image"

/*
The video of the Macintosh Plus, which is as simple as video gets. There is no
CRTC and there are no modes: a plain bitmap of 512 by 342 pixels, one bit per
pixel, most significant bit to the left, and a set bit is black. Drawing it is
one pass over 21888 bytes.

The frame buffer hangs from the top of the RAM at a fixed offset, the same for
every RAM size, and the page in use is the bit 6 of the VIA port A. The sound
buffer hangs from the top the same way, which is why its offsets are here too.
*/
const (
	// The size of the screen in pixels. A frontend takes it from the bounds
	// of the image instead of from here, so these stay in.
	width  = 512
	height = 342

	bytesPerLine    = width / 8
	frameBufferSize = bytesPerLine * height

	videoMainOffset      = 0x5900
	videoAlternateOffset = 0xd900

	soundMainOffset      = 0x0300
	soundAlternateOffset = 0x5f00
)

// video renders the frame buffer the VIA has selected
type video struct {
	mm *memoryManager

	// alternate selects the second page, the VIA PA6 bit
	alternate bool

	// image is drawn into again on every call rather than made afresh,
	// because a frontend asks for it sixty times a second
	image *image.RGBA
}

func newVideo(mm *memoryManager) *video {
	return &video{
		mm:    mm,
		image: image.NewRGBA(image.Rect(0, 0, width, height)),
	}
}

// setAlternatePage selects the video page, from the VIA port A bit 6
func (v *video) setAlternatePage(alternate bool) {
	v.alternate = alternate
}

// frameBuffer returns the active video page
func (v *video) frameBuffer() []uint8 {
	base := v.mm.ramTop() - videoMainOffset
	if v.alternate {
		base = v.mm.ramTop() - videoAlternateOffset
	}
	return v.mm.ram[base : base+frameBufferSize]
}

/*
GetImage returns the screen as it is now. The image belongs to the machine and
is drawn into again by the next call, so a caller that wants to keep one has
to copy it.
*/
func (m *Mac) GetImage() *image.RGBA {
	return m.video.render()
}

/*
render draws the frame buffer into the image. The buffer and the pixels of the
image are both contiguous, in the same order and of the same shape, so this is
one sweep over the pair of them rather than a walk in x and y: there is no
row to work out and no bounds to check for each of the 175104 pixels.
*/
func (v *video) render() *image.RGBA {
	pix := v.image.Pix

	p := 0
	for _, bits := range v.frameBuffer() {
		for mask := uint8(0x80); mask != 0; mask >>= 1 {
			// A set bit is black
			shade := uint8(0xff)
			if bits&mask != 0 {
				shade = 0x00
			}

			pix[p] = shade
			pix[p+1] = shade
			pix[p+2] = shade
			pix[p+3] = 0xff
			p += 4
		}
	}

	return v.image
}

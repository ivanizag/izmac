package izmac

import "github.com/ivanizag/izmac/screen"

/*
The video and sound buffers hang from the top of the RAM at fixed offsets,
the same ones for every RAM size. The page in use is selected by a bit of the
VIA port A, PA6 for the video and PA3 for the sound.
*/
const (
	videoMainOffset      = 0x5900
	videoAlternateOffset = 0xd900

	soundMainOffset      = 0x0300
	soundAlternateOffset = 0x5f00
)

// video gives access to the frame buffer selected by the VIA. It implements
// screen.VideoSource.
type video struct {
	mm *memoryManager

	// alternate selects the second page, the VIA PA6 bit
	alternate bool
}

func newVideo(mm *memoryManager) *video {
	return &video{mm: mm}
}

// setAlternatePage selects the video page, from the VIA port A bit 6
func (v *video) setAlternatePage(alternate bool) {
	v.alternate = alternate
}

// GetFrameBuffer returns the active video page
func (v *video) GetFrameBuffer() []uint8 {
	base := v.mm.ramTop() - videoMainOffset
	if v.alternate {
		base = v.mm.ramTop() - videoAlternateOffset
	}
	return v.mm.ram[base : base+screen.FrameBufferSize]
}

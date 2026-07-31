package izmac

import (
	"image/color"
	"testing"

	"github.com/ivanizag/izmac/storage"
)

func newTestVideo(t *testing.T) (*Mac, *video) {
	t.Helper()

	config := NewConfiguration()
	config.RomFile = "<test>"

	m := ensureNewMac(t, config, storage.RomFromData(make([]uint8, storage.RomSize)), nil, nil)
	return m, m.video
}

// setPixel puts a bit in the frame buffer the way the machine would
func setPixel(v *video, x int, y int) {
	buffer := v.frameBuffer()
	buffer[y*bytesPerLine+x/8] |= 0x80 >> (x % 8)
}

func TestTheImageIsTheSizeOfTheScreen(t *testing.T) {
	m, _ := newTestVideo(t)

	image := m.GetImage()
	if image.Rect.Dx() != width || image.Rect.Dy() != height {
		t.Errorf("the image is %vx%v, wanted %vx%v",
			image.Rect.Dx(), image.Rect.Dy(), width, height)
	}
}

// A set bit is black, which is the opposite of what a bitmap usually means
func TestASetBitIsBlack(t *testing.T) {
	m, v := newTestVideo(t)
	black := color.RGBA{0x00, 0x00, 0x00, 0xff}
	white := color.RGBA{0xff, 0xff, 0xff, 0xff}

	setPixel(v, 0, 0)
	setPixel(v, width-1, height-1)

	image := m.GetImage()
	if image.RGBAAt(0, 0) != black {
		t.Error("a set bit is not black")
	}
	if image.RGBAAt(width-1, height-1) != black {
		t.Error("the last bit of the buffer is not the bottom right pixel")
	}
	if image.RGBAAt(1, 0) != white {
		t.Error("a clear bit is not white")
	}
}

func TestTheMostSignificantBitIsOnTheLeft(t *testing.T) {
	m, v := newTestVideo(t)
	v.frameBuffer()[0] = 0x80

	image := m.GetImage()
	if image.RGBAAt(0, 0) != (color.RGBA{0, 0, 0, 0xff}) {
		t.Error("the most significant bit is not the leftmost pixel")
	}
	if image.RGBAAt(7, 0) != (color.RGBA{0xff, 0xff, 0xff, 0xff}) {
		t.Error("the most significant bit reached the wrong end of the byte")
	}
}

func TestThePageIsChosenByTheVia(t *testing.T) {
	m, v := newTestVideo(t)

	// The alternate page is somewhere else in the RAM
	main := v.mm.ramTop() - videoMainOffset
	alternate := v.mm.ramTop() - videoAlternateOffset
	v.mm.ram[main] = 0xff
	v.mm.ram[alternate] = 0x00

	if m.GetImage().RGBAAt(0, 0) != (color.RGBA{0, 0, 0, 0xff}) {
		t.Error("the main page is not the one drawn")
	}

	v.setAlternatePage(true)
	if m.GetImage().RGBAAt(0, 0) != (color.RGBA{0xff, 0xff, 0xff, 0xff}) {
		t.Error("the alternate page was not drawn once it was selected")
	}
}

// The image is drawn into again rather than made afresh, because a frontend
// asks for it sixty times a second
func TestTheImageIsReused(t *testing.T) {
	m, _ := newTestVideo(t)

	if m.GetImage() != m.GetImage() {
		t.Error("a new image is made on every call")
	}
}

/*
Package activities makes the pictures of izmac's activities, the pages of
doc/activities that walk through what a Macintosh Plus owner did: an Album
writes screenshots and recordings of a machine into a folder, in the frame
they all have, and a Recording is a GIF of the screen taken as an operator
drives the machine.

The activities of izmac's own documentation are in this package's tests, and
are made with:

	IZMAC_ACTIVITY_SCREENSHOTS=1 go test -run TestActivityScreenshots ./activities

Anyone writing their own does it the same way: a machine from izmac.NewMac,
an operator.Operator to drive it, and an Album for what it shows on the way.
*/
package activities

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"

	"github.com/ivanizag/izmac"
)

/*
An Album is the folder of the pictures of one activity. Screenshots are PNG
files and recordings GIF files, named after what they show, in the frame of
Frame. The folder is made with the first picture.
*/
type Album struct {
	folder string
}

// NewAlbum is an album in a folder
func NewAlbum(folder string) *Album {
	return &Album{folder: folder}
}

// Path is where a picture of the album is, by its name and extension
func (a *Album) Path(file string) string {
	return filepath.Join(a.folder, file)
}

// create makes a file of the album, and the folder when it is not there yet
func (a *Album) create(file string) (*os.File, error) {
	if err := os.MkdirAll(a.folder, 0o755); err != nil {
		return nil, err
	}
	return os.Create(a.Path(file))
}

// Screenshot writes the screen of a machine as it is now
func (a *Album) Screenshot(m *izmac.Mac, name string) error {
	return a.Write(m.Screenshot(), name, "")
}

// ScreenshotOf writes the screen of a machine with a label in its frame,
// which says whose it is when there are two
func (a *Album) ScreenshotOf(m *izmac.Mac, name string, label string) error {
	return a.Write(m.Screenshot(), name, label)
}

// Write writes a screen taken before, or put together from several, in its
// frame
func (a *Album) Write(screen image.Image, name string, label string) error {
	f, err := a.create(name + ".png")
	if err != nil {
		return err
	}
	if err := png.Encode(f, Frame(screen, label)); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

/*
KeepPrintedPage keeps a page the ImageWriter printed, a PNG file of izmac's,
as a picture of the album: from its top to half an inch below the last line
printed, since the rest of a page of eleven inches is blank and shown whole
it reads as a picture that did not finish. It is outlined in grey so that it
shows as paper on a white background, the edge where it is cut in dashes.
*/
func (a *Album) KeepPrintedPage(printed string, name string) error {
	f, err := os.Open(printed)
	if err != nil {
		return err
	}
	paper, err := png.Decode(f)
	f.Close()
	if err != nil {
		return err
	}

	// The last row with ink on it, and half an inch more at 144 dots
	bounds := paper.Bounds()
	last := bounds.Min.Y
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if r, _, _, _ := paper.At(x, y).RGBA(); r < 0x8000 {
				last = y
				break
			}
		}
	}
	bottom := min(last+printedMargin, bounds.Max.Y)

	kept := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bottom-bounds.Min.Y))
	draw.Draw(kept, kept.Bounds(), paper, bounds.Min, draw.Src)
	w, h := kept.Bounds().Dx(), kept.Bounds().Dy()
	for x := 0; x < w; x++ {
		kept.Set(x, 0, labelGrey)
		if x/8%2 == 0 {
			kept.Set(x, h-1, labelGrey)
		}
	}
	for y := 0; y < h; y++ {
		kept.Set(0, y, labelGrey)
		kept.Set(w-1, y, labelGrey)
	}

	out, err := a.create(name + ".png")
	if err != nil {
		return err
	}
	if err := png.Encode(out, kept); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// printedMargin is the paper kept below the last line printed, half an inch
// at the 144 dots to the inch of the ImageWriter's pages
const printedMargin = 72

// SaveRecording writes a recording as a GIF, its last picture held for
// longer, by hold hundredths of a second
func (a *Album) SaveRecording(r *Recording, name string, hold int) error {
	f, err := a.create(name + ".gif")
	if err != nil {
		return err
	}
	if err := r.Encode(f, hold); err != nil {
		f.Close()
		return fmt.Errorf("the recording %v: %w", name, err)
	}
	return f.Close()
}

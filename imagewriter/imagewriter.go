/*
Package imagewriter is an Apple ImageWriter II on the end of a serial line.
It knows nothing of the machine that is printing: bytes go in and pages come
out.

The Macintosh prints everything as dot graphics. QuickDraw draws the page
into a bitmap and the driver sends it a strip at a time, eight dots tall,
with the head moved into place before each strip and the paper pulled through
between them:

	ESC N ESC ! ESC > ESC f ESC T16 LF ESC F0007 ESC G0050 <50 bytes> ESC T00 CR

which is the pitch and the direction set, an eighth of an inch of paper, the
head at the dot column 7, and fifty columns of eight dots each. Every byte of
a graphics run is one column, and **the bit 0 is the top dot**, which is the
other way round from the Epson printers everything else of the period spoke
to.

That description is not from a manual. It was read off the wire: izmac's raw
printer mode was written first, a page was printed from a real System with
the real driver, and this was written against what came out. The one thing
worth knowing that no manual would have made obvious is the last line of it:
the driver never sends a form feed. It gets to the end of a page by feeding
paper, so a page is finished here when the paper has gone far enough and not
when anything says so.

Two densities matter. The paper moves in 144ths of an inch, which is what the
ESC T argument counts, and the eight pins of the head are a 72nd of an inch
apart, so a strip of graphics is eight dots down a sixth of an inch whatever
else is going on. Across, the dots are as far apart as the character pitch
says, and the pitch the Macintosh picks for its 72 dot per inch bitmap is the
80 dot per inch pica: that mismatch is the reason a Macintosh page came off
an ImageWriter a tenth narrower than it looked on the screen, and it is
reproduced here rather than corrected.
*/
package imagewriter

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
)

/*
ImageWriter takes the bytes of a print job and writes a PNG for every page.
It is an io.WriteCloser: closing it finishes the page in the machine, which
is what gets the last one out when the job ended without pushing the paper
all the way through.
*/
type ImageWriter struct {
	// prefix is what the page files are named after
	prefix string

	// paper is the page being printed on, made when the first dot lands on
	// it so that a job that prints nothing leaves nothing behind
	paper *image.Gray

	// pages is how many have come out
	pages int

	// OnPage is told the name of every page as it comes out, which is how a
	// caller says so
	OnPage func(name string)

	/*
		emit is what a finished page goes to. It writes a PNG; a test
		replaces it to look at the page instead of at a file.
	*/
	emit func(page *image.Gray, number int) (string, error)

	// The state of the decoder: what is being collected, and for which
	// command
	state   int
	command uint8
	digits  []uint8
	wanted  int

	// graphics is how many bytes of a graphics run are still to come
	graphics int

	/*
		column is where the head is, counted in dots of the current
		horizontal density from the left of the printable area, and row is
		where the paper is, counted in 144ths of an inch from the top of
		the page. The paper is what moves on a real printer, but it is
		simpler to think of the head going down the page and it comes to
		the same thing.
	*/
	column int
	row    int

	// feed is what a line feed advances, in 144ths of an inch, and reverse
	// says the paper is going the other way
	feed    int
	reverse bool

	// density is the dots per inch across, which the pitch commands set
	density int

	// inked says something has been printed on the page in the machine,
	// which is what tells a page that has to come out from a blank one the
	// paper simply went past
	inked bool
}

const (
	// The states of the decoder
	stateData = iota
	stateEscape
	stateDigits
	stateGraphics

	/*
		The paper. Eight inches of printable width on eleven inches of
		page, which is what the printer is set up for out of the box and
		what the driver lays a page out for.
	*/
	pageWidthInches  = 8
	pageLengthInches = 11

	/*
		rasterDpi is what the page is drawn at. The paper moves in 144ths
		of an inch, so at 144 dots per inch a step of the paper is a row of
		the image and nothing has to be rounded on the axis where the
		rounding would show.
	*/
	rasterDpi = 144

	// pinDpi is the spacing of the eight pins of the head, which is a 72nd
	// of an inch whatever the pitch across is
	pinDpi = 72

	// pinsPerStrip is the height of a graphics run, the eight pins
	pinsPerStrip = 8

	// defaultDensity is the pica pitch, which is what the Macintosh driver
	// selects and what a printer comes up in
	defaultDensity = 80

	// defaultFeed is a sixth of an inch, the six lines to the inch a
	// printer comes up in
	defaultFeed = 24
)

// pageWidth and pageLength are the paper in dots of the raster
const (
	pageWidth  = pageWidthInches * rasterDpi
	pageLength = pageLengthInches * rasterDpi
)

// New returns an ImageWriter whose pages are written as PNG files named after
// the prefix: the prefix, the number of the page and the extension
func New(prefix string) *ImageWriter {
	w := &ImageWriter{
		prefix:  prefix,
		density: defaultDensity,
		feed:    defaultFeed,
	}
	w.emit = w.writePng
	return w
}

// Write takes bytes off the serial line
func (w *ImageWriter) Write(data []uint8) (int, error) {
	for _, b := range data {
		if err := w.step(b); err != nil {
			return 0, err
		}
	}
	return len(data), nil
}

/*
Close finishes the page in the machine. A job that ended without feeding the
paper past the end of the page has left one there, and this is what gets it
out.
*/
func (w *ImageWriter) Close() error {
	return w.finishPage()
}

// step takes one byte through the decoder
func (w *ImageWriter) step(b uint8) error {
	switch w.state {
	case stateEscape:
		return w.escape(b)

	case stateDigits:
		w.digits = append(w.digits, b)
		if len(w.digits) < w.wanted {
			return nil
		}
		w.state = stateData
		return w.argument()

	case stateGraphics:
		w.dots(b)
		w.column++
		w.graphics--
		if w.graphics == 0 {
			w.state = stateData
		}
		return nil
	}

	return w.data(b)
}

const (
	charEscape = 0x1b
	charLF     = 0x0a
	charCR     = 0x0d
	charFF     = 0x0c
)

// data handles a byte that is not part of a command
func (w *ImageWriter) data(b uint8) error {
	switch b {
	case charEscape:
		w.state = stateEscape

	case charCR:
		// The carriage back to the left margin. The driver sets the line
		// feed to zero before every one of them, so whether the printer
		// was set up to feed on a return as well does not arise.
		w.column = 0

	case charLF:
		return w.advance(w.feed)

	case charFF:
		return w.formFeed()

	default:
		if b >= 0x20 {
			w.character(b)
		}
	}

	return nil
}

/*
escape dispatches the command after an escape. The ones that take an argument
go on to collect it; the ones that set the pitch land in the table below; and
the rest are the ones a printer answers to and a page does not show, which
are stepped over.

Stepping over an unknown command is right only for the ones that carry no
argument, and there is no way to know that of a command that has not been
seen. What makes it safe enough is that everything the Macintosh driver sends
is either understood here or carries nothing: the whole of a real job decodes
with nothing left over but the seven zeroes and a space it signs off with.
*/
func (w *ImageWriter) escape(b uint8) error {
	w.state = stateData

	if digits, ok := commandArguments()[b]; ok {
		w.state = stateDigits
		w.command = b
		w.wanted = digits
		w.digits = w.digits[:0]
		return nil
	}

	if density, ok := pitchDensities()[b]; ok {
		w.density = density
		return nil
	}

	switch b {
	case 'f':
		w.reverse = false
	case 'r':
		w.reverse = true
	}

	return nil
}

/*
commandArguments is how many digits the commands that take a number are
followed by. They arrive as ASCII digits rather than as bytes, which is what
lets a printer be driven from a terminal.
*/
func commandArguments() map[uint8]int {
	return map[uint8]int{
		'F': 4, // the head to a dot column of the line
		'G': 4, // a run of graphics, and how many bytes of it
		'T': 2, // the line feed, in 144ths of an inch
		'L': 3, // the left margin, in characters
	}
}

/*
pitchDensities is the dots per inch across that each character pitch prints
at. The pitch and the graphics density are the same setting on this printer:
asking for narrower characters is asking for finer dots, which is how the
driver picks a density at all.

The Macintosh uses two of them, the pica for its 72 dot per inch pages and
the proportional elite for the 144 dot per inch ones the best quality draws.
*/
func pitchDensities() map[uint8]int {
	return map[uint8]int{
		'n': 72,  // extended, 9 characters to the inch
		'N': 80,  // pica, 10 to the inch
		'E': 96,  // elite, 12 to the inch
		'e': 107, // semicondensed
		'q': 120, // condensed, 15 to the inch
		'Q': 136, // ultracondensed, 17 to the inch
		'p': 144, // proportional pica
		'P': 160, // proportional elite
	}
}

// argument acts on a command once its digits have arrived
func (w *ImageWriter) argument() error {
	value := 0
	for _, d := range w.digits {
		if d < '0' || d > '9' {
			// Not a number after all, so nothing to do with it. The
			// command is over either way.
			return nil
		}
		value = value*10 + int(d-'0')
	}

	switch w.command {
	case 'F':
		w.column = value
	case 'T':
		w.feed = value
	case 'L':
		w.column = value * w.density / charactersPerInch(w.density)
	case 'G':
		if value == 0 {
			return nil
		}
		w.graphics = value
		w.state = stateGraphics
	}

	return nil
}

/*
dots prints one column of a graphics run, the eight pins of the head with the
bit 0 at the top. A pin is a 72nd of an inch from the next one and the page is
drawn at twice that, so every dot is two rows of the raster tall, which is
also what makes the dots of a strip run into each other on paper the way they
do here.
*/
func (w *ImageWriter) dots(column uint8) {
	for pin := 0; pin < pinsPerStrip; pin++ {
		if column&(1<<pin) == 0 {
			continue
		}
		w.dot(w.column, w.row+pin*rasterDpi/pinDpi)
	}
}

/*
dot puts one dot on the paper. The column is in dots of the current density
and the row in 144ths of an inch, and both are turned into the raster here:
the dot covers as much of it as its own size covers of the paper, so a coarse
density draws fat dots and a line of them has no gaps in it.
*/
func (w *ImageWriter) dot(column int, row int) {
	if w.paper == nil {
		w.paper = newPaper()
	}
	w.inked = true

	left := rasterX(column, w.density)
	width := dotWidth(w.density)
	height := rasterDpi / pinDpi

	for y := row; y < row+height; y++ {
		if y < 0 || y >= pageLength {
			continue
		}
		for x := left; x < left+width; x++ {
			if x < 0 || x >= pageWidth {
				continue
			}
			w.paper.SetGray(x, y, color.Gray{Y: 0})
		}
	}
}

/*
rasterX is where a dot column of the current density falls on the page,
rounded to the nearest row of the raster rather than truncated. Truncating
would put the eighty dots of an inch at an uneven spacing of one and two
columns, which shows as a texture across a printed page that is not on the
paper.
*/
func rasterX(column int, density int) int {
	return (column*rasterDpi + density/2) / density
}

/*
dotWidth is how much of the raster a dot covers, rounded up so that the dots
of a line touch. A dot on paper is bigger than the spacing anyway: the ink
runs together, which is why a dot matrix printer can draw a solid line at
all.
*/
func dotWidth(density int) int {
	width := (rasterDpi + density - 1) / density
	if width < 1 {
		return 1
	}
	return width
}

// charactersPerInch is the pitch that goes with a density, which is what a
// margin given in characters has to be measured in
func charactersPerInch(density int) int {
	// The densities are eight times the pitch on this printer, which is
	// the eight dots a character cell is wide
	pitch := density / 8
	if pitch < 1 {
		return 1
	}
	return pitch
}

/*
advance pulls the paper through. The page is finished when it has gone past
the end of it, which on this printer is the only way a page ever ends: the
Macintosh driver does not send a form feed, it feeds the paper out and starts
the next one.
*/
func (w *ImageWriter) advance(units int) error {
	if w.reverse {
		units = -units
	}

	w.row += units
	if w.row < 0 {
		w.row = 0
	}

	for w.row >= pageLength {
		w.row -= pageLength
		if err := w.finishPage(); err != nil {
			return err
		}
	}

	return nil
}

// formFeed takes the paper to the top of the next page, which is what a
// program that drives the printer itself would send
func (w *ImageWriter) formFeed() error {
	w.row = 0
	w.column = 0
	return w.finishPage()
}

/*
finishPage takes the paper out of the machine and writes it. A page with
nothing on it is not written: the paper going past the end of the last page
of a job would otherwise leave a blank one behind every time.
*/
func (w *ImageWriter) finishPage() error {
	if !w.inked {
		w.paper = nil
		return nil
	}

	w.pages++
	name, err := w.emit(w.paper, w.pages)

	w.paper = nil
	w.inked = false

	if err != nil {
		return err
	}

	if w.OnPage != nil {
		w.OnPage(name)
	}
	return nil
}

// newPaper is a blank sheet, which is white and not the zero of the image
func newPaper() *image.Gray {
	paper := image.NewGray(image.Rect(0, 0, pageWidth, pageLength))
	for i := range paper.Pix {
		paper.Pix[i] = 0xff
	}
	return paper
}

// writePng is what a finished page is normally done with
func (w *ImageWriter) writePng(paper *image.Gray, number int) (string, error) {
	name := freeName(w.prefix, number)

	file, err := os.Create(name)
	if err != nil {
		return "", err
	}

	if err := png.Encode(file, paper); err != nil {
		file.Close()
		return "", err
	}

	return name, file.Close()
}

/*
freeName is the first name from a number up that is not taken. The pages of a
session are numbered from one, and a machine run twice would otherwise print
its second run over the first: the out tray fills up instead.
*/
func freeName(prefix string, number int) string {
	for {
		name := fmt.Sprintf("%v_%03d.png", prefix, number)
		if _, err := os.Stat(name); err != nil {
			return name
		}
		number++
	}
}

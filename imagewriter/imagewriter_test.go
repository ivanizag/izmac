package imagewriter

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/*
printed builds a printer whose pages are kept rather than written, and runs a
job through it. The pages come back in the order they came out.
*/
func printed(t *testing.T, job string) []*image.Gray {
	t.Helper()

	var pages []*image.Gray

	w := New("unused")
	w.emit = func(paper *image.Gray, number int) (string, error) {
		pages = append(pages, paper)
		return fmt.Sprintf("page %v", number), nil
	}

	if _, err := w.Write([]uint8(job)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	return pages
}

// inked says whether there is a dot at a place on the page
func inked(page *image.Gray, x int, y int) bool {
	return page.GrayAt(x, y).Y < 0x80
}

/*
A run of graphics is one byte per column of eight dots, and the bit 0 is the
top one. Getting that the wrong way round is the mistake that costs a day:
the page still comes out, with every character of it upside down inside its
own eight dot strip, which reads as a font that has gone wrong rather than as
a bug in the decoder.
*/
func TestTheBitZeroOfAColumnIsTheTopDot(t *testing.T) {
	// One column with the top dot set and one with the bottom
	pages := printed(t, "\x1bG0002\x01\x80\x0c")

	if len(pages) != 1 {
		t.Fatalf("the job printed %v pages, wanted 1", len(pages))
	}
	page := pages[0]

	// The pins are a 72nd of an inch apart and the page is drawn at a
	// 144th, so the eighth pin is fourteen rows below the first
	if !inked(page, 0, 0) {
		t.Error("the bit 0 did not print at the top of the strip")
	}
	if inked(page, 0, 14) {
		t.Error("the bit 0 printed at the bottom of the strip as well")
	}

	second := rasterX(1, defaultDensity)
	if !inked(page, second, 14) {
		t.Error("the bit 7 did not print at the bottom of the strip")
	}
	if inked(page, second, 0) {
		t.Error("the bit 7 printed at the top of the strip as well")
	}
}

/*
A dot is as wide as the density says it is. The pica pitch the Macintosh
prints at is 80 dots to the inch on a page drawn at 144, so a dot covers most
of two columns of it and a line of them has no gaps.
*/
func TestADotCoversWhatItWouldOnPaper(t *testing.T) {
	pages := printed(t, "\x1bN\x1bG0002\x01\x01\x0c")
	page := pages[0]

	for x := 0; x < 4; x++ {
		if !inked(page, x, 0) {
			t.Errorf("the two dots left a gap at %v", x)
		}
	}
	if inked(page, 4, 0) {
		t.Error("the two dots covered more than the two of them are wide")
	}
}

// The head goes where ESC F says, counted in dots of the current density
func TestTheHeadIsPositioned(t *testing.T) {
	pages := printed(t, "\x1bN\x1bF0080\x1bG0001\x01\x0c")
	page := pages[0]

	// Eighty dots at eighty to the inch is an inch in, and an inch of a
	// page drawn at 144 dots is 144 of them
	if !inked(page, rasterDpi, 0) {
		t.Error("the head did not print an inch in")
	}
}

/*
The paper moves by what ESC T last said, in 144ths of an inch, and a line
feed is what moves it. The two are separate commands and the driver changes
the distance far more often than it uses it.
*/
func TestThePaperMovesByTheLineFeedDistance(t *testing.T) {
	pages := printed(t, "\x1bT36\n\x1bG0001\x01\x0c")
	page := pages[0]

	if !inked(page, 0, 36) {
		t.Error("a line feed of 36 did not move the paper a quarter of an inch")
	}
}

// And it moves back when the direction is reversed, which is how the driver
// goes over a strip it has already printed
func TestThePaperMovesBackwards(t *testing.T) {
	pages := printed(t, "\x1bT72\n\n\x1br\n\x1bG0001\x01\x0c")
	page := pages[0]

	if !inked(page, 0, 72) {
		t.Error("the reverse line feed did not take the paper back an inch")
	}
}

/*
A page comes out when the paper has gone past the end of it. The Macintosh
driver never sends a form feed: it prints what is on the page and then feeds
the paper out, so this is the only thing that ends a page in a real job.
*/
func TestAPageComesOutWhenThePaperHasGonePastIt(t *testing.T) {
	// A dot, and then twelve inches of paper an inch at a time
	job := "\x1bG0001\x01\x1bT99" + strings.Repeat("\n", 18)
	pages := printed(t, job)

	if len(pages) != 1 {
		t.Fatalf("%v pages came out, wanted 1", len(pages))
	}
	if !inked(pages[0], 0, 0) {
		t.Error("the page that came out is not the one that was printed on")
	}
}

// And paper that goes through with nothing on it is not a page
func TestBlankPaperIsNotAPage(t *testing.T) {
	job := "\x1bT99" + strings.Repeat("\n", 40)
	if pages := printed(t, job); len(pages) != 0 {
		t.Errorf("%v blank pages came out", len(pages))
	}
}

/*
Closing finishes the page in the machine. A job that stopped without feeding
the paper out has left one there, and it is the only page most jobs of one
page have.
*/
func TestClosingTakesTheLastPageOut(t *testing.T) {
	if pages := printed(t, "\x1bG0001\x01"); len(pages) != 1 {
		t.Errorf("%v pages came out of a job that never fed the paper, wanted 1",
			len(pages))
	}
}

// A form feed is the other way a page ends, for something that drives the
// printer itself rather than through the Print Manager
func TestAFormFeedEndsThePage(t *testing.T) {
	pages := printed(t, "\x1bG0001\x01\x0c\x1bG0001\x01")

	if len(pages) != 2 {
		t.Fatalf("%v pages came out, wanted 2", len(pages))
	}
	if !inked(pages[1], 0, 0) {
		t.Error("the second page did not start at the top")
	}
}

/*
Text, which a Macintosh only sends in the draft quality. The glyphs are not
the printer's own, so what is worth pinning is that a character puts ink on
the page and moves the head on by a character cell and not by anything else.
*/
func TestACharacterPrintsAndMovesTheHeadOn(t *testing.T) {
	pages := printed(t, "\x1bNAA\x0c")
	page := pages[0]

	ink := func(from int, to int) bool {
		for x := from; x < to; x++ {
			for y := 0; y < 16; y++ {
				if inked(page, x, y) {
					return true
				}
			}
		}
		return false
	}

	// A character cell is eight dots, which at 80 to the inch on a page
	// drawn at 144 is a little over fourteen columns of the raster
	if !ink(0, 10) {
		t.Error("the first character left no ink")
	}
	if !ink(15, 25) {
		t.Error("the second character did not follow the first")
	}
}

// An unknown command is stepped over rather than printed
func TestAnUnknownCommandIsIgnored(t *testing.T) {
	pages := printed(t, "\x1b!\x1b>\x1bG0001\x01\x0c")

	if len(pages) != 1 {
		t.Fatalf("%v pages came out, wanted 1", len(pages))
	}
	if !inked(pages[0], 0, 0) {
		t.Error("the graphics after the unknown commands did not print")
	}
}

/*
The whole of a real job, in miniature: the preamble the driver sends, two
strips of graphics with the paper pulled through between them, and the feed
that pushes the page out. What this pins is that the pieces work together,
since every one of them is already pinned on its own above.
*/
func TestAJobTheWayTheDriverSendsOne(t *testing.T) {
	job := "\x1b?\r\x1bo\x1bT18\x1br\n\x1bf\n" +
		"\x1bN\x1b!\x1b>\x1bf\x1bT16\n\x1bF0007\x1bG0002\xff\xff\x1bT00\r" +
		"\x1bN\x1b!\x1b>\x1bf\x1bT16\n\x1bF0007\x1bG0002\xff\xff\x1bT00\r"

	pages := printed(t, job)
	if len(pages) != 1 {
		t.Fatalf("%v pages came out, wanted 1", len(pages))
	}
	page := pages[0]

	/*
		Seven dots in at eighty to the inch, and the two strips an eighth
		of an inch apart. The reverse line feed of the preamble runs into
		the top of the page and stops there, the one after it moves the
		paper an eighth of an inch, and each strip is an eighth further
		down.
	*/
	x := rasterX(7, defaultDensity)
	if !inked(page, x, 18+16) {
		t.Error("the first strip is not where the driver put it")
	}
	if !inked(page, x, 18+16+16) {
		t.Error("the second strip did not follow an eighth of an inch below")
	}
}

// The pages are written where they were asked for, and a run does not print
// over the pages of the last one
func TestThePagesAreWritten(t *testing.T) {
	prefix := filepath.Join(t.TempDir(), "page")

	for run := 0; run < 2; run++ {
		w := New(prefix)
		if _, err := w.Write([]uint8("\x1bG0001\x01")); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	}

	for _, name := range []string{prefix + "_001.png", prefix + "_002.png"} {
		info, err := os.Stat(name)
		if err != nil {
			t.Fatalf("%v was not written: %v", name, err)
		}
		if info.Size() == 0 {
			t.Errorf("%v is empty", name)
		}
	}
}

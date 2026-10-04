package operator

import (
	"path/filepath"
)

/*
The diskettes, as the person at a Macintosh handles them. The machine ejects
its own, and asks for the one it wants by name; these put one back in the
drive it left empty. The machine is run in half seconds of its time while
they wait, and every half second calls the function given, if any, which is
how a recording keeps a picture of what happened.
*/

// diskettePace is how often an empty drive is looked at, and how long a hand
// takes to put a diskette in it, in frames
const (
	diskettePace   = 30
	disketteFrames = 60
)

// InsertDiskette puts a diskette image in a drive, 0 for the internal one and
// 1 for the external
func (o *Operator) InsertDiskette(drive int, image string) error {
	return o.m.InsertDiskette(drive, image)
}

/*
SwapDiskettes runs the machine for some frames as someone at a Macintosh of
one drive does: whenever it ejects the diskette in the internal drive, the
other of the two goes in. That is what copying from one diskette to another,
or opening a diskette that is not in the drive, asks for.
*/
func (o *Operator) SwapDiskettes(frames int, a string, b string, each func()) error {
	inDrive := o.m.GetDiskette(0).Image
	for frame := 0; frame < frames; frame += diskettePace {
		o.Run(diskettePace)
		if each != nil {
			each()
		}
		if image := o.m.GetDiskette(0).Image; image != "" {
			inDrive = image
			continue
		}
		next := a
		if filepath.Base(inDrive) == filepath.Base(a) {
			next = b
		}
		o.Run(disketteFrames)
		if err := o.m.InsertDiskette(0, next); err != nil {
			return err
		}
		inDrive = next
	}
	return nil
}

/*
FeedDiskettes runs the machine for some frames as someone with a pile of
diskettes beside it: whenever a drive is empty, the next of the pile goes in.
An Installer asks for its diskettes by name, in an order of its own, and does
not give back one that is not the one it asked for: the pile has to be in
that order.
*/
func (o *Operator) FeedDiskettes(frames int, pile []string, each func()) error {
	for frame := 0; frame < frames; frame += diskettePace {
		o.Run(diskettePace)
		if each != nil {
			each()
		}
		for drive := 0; drive < 2 && len(pile) > 0; drive++ {
			if o.m.GetDiskette(drive).Image != "" {
				continue
			}
			o.Run(disketteFrames)
			if err := o.m.InsertDiskette(drive, pile[0]); err != nil {
				return err
			}
			pile = pile[1:]
		}
	}
	return nil
}

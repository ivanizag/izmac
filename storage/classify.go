package storage

import (
	"fmt"
	"os"
)

/*
Working out what a disk image is from the image itself, so that files named on
a command line can be put where they belong without being told which is which.

A DiskCopy image says what it is in its own header and is always a diskette.
Failing that, a Macintosh disk that has been through Apple's formatter starts
with a driver descriptor map, the two letters 'ER' followed by the block size,
and only a hard disk has one. Failing that too, the size tells them apart: the
drives of this machine make 400K and 800K diskettes and nothing else, so an
image of exactly that size is one, and anything else is a hard disk.
*/

// Kind is what an image turned out to be
type Kind int

const (
	KindHardDisk Kind = iota
	KindFloppy
)

func (k Kind) String() string {
	if k == KindFloppy {
		return "diskette"
	}
	return "hard disk"
}

// driverDescriptorSignature is the 'ER' a partitioned Macintosh disk starts
// with
const driverDescriptorSignature = 0x4552

// Classify says whether a file is a diskette or a hard disk image
func Classify(filename string) (Kind, error) {
	file, err := os.Open(filename)
	if err != nil {
		return KindHardDisk, fmt.Errorf("can not open the disk image: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return KindHardDisk, err
	}

	header := make([]uint8, diskCopyHeaderSize)
	read, _ := file.ReadAt(header, 0)
	header = header[:read]

	// A DiskCopy header settles it the other way, and is looked at first
	// because the name the file starts with could say anything at all
	if _, ok := parseDiskCopyHeader(header); ok {
		return KindFloppy, nil
	}

	// A driver descriptor map, which no diskette carries
	if read >= 2 && uint16(header[0])<<8|uint16(header[1]) == driverDescriptorSignature {
		return KindHardDisk, nil
	}

	/*
		The sizes a diskette comes in, including the two this machine can
		not read. Those are sorted here as the diskettes they are so that
		opening one can say why it is no good, rather than leaving a
		1.44Mb image to be quietly attached to the SCSI bus as a hard disk.
	*/
	switch info.Size() {
	case floppySize400K, floppySize800K, floppySize720K, floppySize1440K:
		return KindFloppy, nil
	}

	return KindHardDisk, nil
}

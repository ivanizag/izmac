package storage

import (
	"fmt"
	"os"
)

/*
Working out what a disk image is from the image itself, so that files named on
a command line can be put where they belong without being told which is which.

A Macintosh disk that has been through Apple's formatter starts with a driver
descriptor map, the two letters 'ER' followed by the block size, and only a
hard disk has one. Failing that the size tells them apart: the drives of this
machine make 400K and 800K diskettes and nothing else, so an image of exactly
that size is one, and anything else is a hard disk.
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

const (
	// driverDescriptorSignature is the 'ER' a partitioned Macintosh disk
	// starts with
	driverDescriptorSignature = 0x4552

	// The sizes the drives of this machine make
	floppySize400K = 400 * 1024
	floppySize800K = 800 * 1024
)

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

	// A driver descriptor map settles it, no diskette carries one
	header := make([]uint8, 2)
	if _, err := file.ReadAt(header, 0); err == nil {
		if uint16(header[0])<<8|uint16(header[1]) == driverDescriptorSignature {
			return KindHardDisk, nil
		}
	}

	switch info.Size() {
	case floppySize400K, floppySize800K:
		return KindFloppy, nil
	}

	return KindHardDisk, nil
}

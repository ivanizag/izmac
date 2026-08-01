package storage

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
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
image of exactly that size is one.

What is left is too big to be a diskette and has no map at the front of it.
If an HFS volume starts where the map should be it is a bare volume, an image
made for the emulators that patch the ROM to supply a driver of their own,
and the machine can do nothing with it. Anything else is taken for a hard
disk, a blank image among them: an image with nothing in it yet is what gets
attached to be formatted from the machine.

The order of all this matters. An 800K diskette is an HFS volume with no map
in front of it too, and is only told from a bare volume by being a size a
drive can hold, so the sizes are looked at first.
*/

// Kind is what an image turned out to be
type Kind int

const (
	KindHardDisk Kind = iota
	KindFloppy
	KindBareVolume
)

func (k Kind) String() string {
	switch k {
	case KindFloppy:
		return "diskette"
	case KindBareVolume:
		return "bare HFS volume"
	}
	return "hard disk"
}

// driverDescriptorSignature is the 'ER' a partitioned Macintosh disk starts
// with
const driverDescriptorSignature = 0x4552

// hfsVolumeSignature is the 'BD' the master directory block starts with, and
// volumeHeaderBlock is where it sits, after the two boot blocks
const (
	hfsVolumeSignature = 0x4244
	volumeHeaderBlock  = 2
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

	// Too big for a drive and with no map in front of it. An HFS volume
	// where the map should be is an image no ROM can boot.
	signature, err := blockSignature(file, volumeHeaderBlock)
	if err != nil {
		return KindHardDisk, err
	}
	if signature == hfsVolumeSignature {
		return KindBareVolume, nil
	}

	return KindHardDisk, nil
}

/*
blockSignature returns the two bytes a block starts with. A block off the end
of the image has no signature and is not an error: the caller is asking what
is there rather than reading something it knows is there.
*/
func blockSignature(r io.ReaderAt, block int64) (uint16, error) {
	header := make([]uint8, 2)
	_, err := r.ReadAt(header, block*BlockSize)
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(header), nil
}

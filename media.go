package izmac

import (
	"fmt"
	"os"
)

/*
Working out what a disk image is from the image itself, so that files named
on the command line can be put where they belong without being told which is
which.

A Macintosh disk that has been through Apple's formatter starts with a driver
descriptor map, the two letters 'ER' followed by the block size, and only a
hard disk has one. Failing that the size tells them apart: the drives of this
machine make 400K and 800K diskettes and nothing else, so an image of exactly
that size is one, and anything else is a hard disk.
*/

// mediaKind is what an image turned out to be
type mediaKind int

const (
	mediaHardDisk mediaKind = iota
	mediaFloppy
)

func (k mediaKind) String() string {
	if k == mediaFloppy {
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

// classifyImage says whether a file is a diskette or a hard disk image
func classifyImage(filename string) (mediaKind, error) {
	file, err := os.Open(filename)
	if err != nil {
		return mediaHardDisk, fmt.Errorf("can not open the disk image: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return mediaHardDisk, err
	}

	// A driver descriptor map settles it, no diskette carries one
	header := make([]uint8, 2)
	if _, err := file.ReadAt(header, 0); err == nil {
		if uint16(header[0])<<8|uint16(header[1]) == driverDescriptorSignature {
			return mediaHardDisk, nil
		}
	}

	switch info.Size() {
	case floppySize400K, floppySize800K:
		return mediaFloppy, nil
	}

	return mediaHardDisk, nil
}

/*
sortMedia takes the files named on the command line and puts each one where it
belongs. Diskettes are recognised and reported rather than silently attached
to the wrong thing, because the drives are not emulated yet and a file quietly
ignored is worse than one refused.
*/
func sortMedia(filenames []string) (hardDisks []string, diskettes []string, err error) {
	for _, filename := range filenames {
		kind, err := classifyImage(filename)
		if err != nil {
			return nil, nil, err
		}

		if kind == mediaFloppy {
			diskettes = append(diskettes, filename)
		} else {
			hardDisks = append(hardDisks, filename)
		}
	}

	if len(hardDisks) > scsiTargetCount {
		return nil, nil, fmt.Errorf("the bus takes %v disks, %v were given",
			scsiTargetCount, len(hardDisks))
	}

	return hardDisks, diskettes, nil
}

// DiskDescription names an attached disk for a frontend to report
type DiskDescription struct {
	Id     int
	Name   string
	Blocks uint32
}

// GetDisks describes the disks on the bus
func (m *Mac) GetDisks() []DiskDescription {
	described := make([]DiskDescription, 0, scsiTargetCount)

	bus, ok := m.mm.scsi.(*scsi5380)
	if !ok {
		return described
	}

	for id, t := range bus.targets {
		if t == nil {
			continue
		}
		described = append(described, DiskDescription{
			Id:     id,
			Name:   t.disk.Name(),
			Blocks: t.disk.Blocks(),
		})
	}
	return described
}

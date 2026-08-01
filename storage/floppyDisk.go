package storage

import (
	"encoding/binary"
	"fmt"
	"os"
)

/*
A diskette image, the sectors of it and the tags that go with them.

Unlike a hard disk, which is read a block at a time out of a file that can be
any size, a diskette is small enough to hold whole: 800Kb of sectors and 19Kb
of tags at the very most. Keeping it in memory means the image on the host is
rewritten complete when something changes, which is what makes the DiskCopy
format bearable, since its checksums cover the whole file and would otherwise
have to be recomputed against it on every sector written.
*/

const (
	// The two sizes the drives of this machine make. A side is eighty
	// tracks of between twelve and eight sectors, 400Kb of them.
	floppySize400K = sectorsPerSide * BlockSize
	floppySize800K = 2 * floppySize400K

	// The two the machine can not read, recognised only so that an image
	// of one can be turned away with a reason rather than by its size
	floppySize720K  = 737280
	floppySize1440K = 1474560
)

// FloppyDisk is a diskette image, held in memory and written back when it
// changes
type FloppyDisk struct {
	name     string
	filename string

	// data is the sectors, one after the other, and tags the twelve bytes
	// that go with each of them. A plain image carries no tags, and they
	// are then zeros that are kept and written back all the same.
	data []uint8
	tags []uint8

	// sides is one for a 400Kb diskette and two for an 800Kb one
	sides int

	readOnly bool
	modified bool

	// diskCopy says the image on the host has a DiskCopy header, so that
	// writing it back puts one there again
	diskCopy bool
}

// NewFloppyDisk reads a diskette image, plain or DiskCopy 4.2
func NewFloppyDisk(filename string, readOnly bool) (*FloppyDisk, error) {
	raw, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("can not open the diskette image: %w", err)
	}

	if !readOnly {
		// A diskette that can not be written to is still usable, the
		// machine sees it as a locked one
		readOnly = !isWritable(filename)
	}

	d := &FloppyDisk{
		name:     filename,
		filename: filename,
		readOnly: readOnly,
	}

	if err := d.load(raw); err != nil {
		return nil, err
	}

	return d, nil
}

// isWritable tells whether the host would let the file be written back
func isWritable(filename string) bool {
	file, err := os.OpenFile(filename, os.O_WRONLY, 0)
	if err != nil {
		return false
	}
	file.Close()
	return true
}

// load takes the bytes of the file apart into sectors and tags
func (d *FloppyDisk) load(raw []uint8) error {
	if header, ok := parseDiskCopyHeader(raw); ok {
		if diskCopyHeaderSize+header.dataSize+header.tagSize > len(raw) {
			return fmt.Errorf("%v has a DiskCopy header claiming %v bytes of "+
				"sectors and %v of tags, and is %v bytes long",
				d.filename, header.dataSize, header.tagSize, len(raw))
		}

		d.diskCopy = true
		if header.name != "" {
			d.name = header.name
		}

		d.data = raw[diskCopyHeaderSize : diskCopyHeaderSize+header.dataSize]
		if header.tagSize != 0 {
			from := diskCopyHeaderSize + header.dataSize
			d.tags = raw[from : from+header.tagSize]
		}
	} else {
		d.data = raw
	}

	switch len(d.data) {
	case floppySize400K:
		d.sides = 1
	case floppySize800K:
		d.sides = 2
	default:
		return fmt.Errorf("%v is %v bytes: the only diskettes a Macintosh "+
			"Plus can read are the 400Kb and 800Kb ones it writes itself",
			d.filename, len(d.data))
	}

	blocks := len(d.data) / BlockSize
	if len(d.tags) != blocks*TagSize {
		// No tags, or not as many as there are sectors. Either way the
		// image is read for its sectors and given tags of its own.
		d.tags = make([]uint8, blocks*TagSize)
	}

	return nil
}

// Name describes the diskette, for a frontend to show and for the traces. A
// DiskCopy image carries the name the volume had when it was made.
func (d *FloppyDisk) Name() string {
	return d.name
}

// Sides is one for a 400Kb diskette and two for an 800Kb one
func (d *FloppyDisk) Sides() int {
	return d.sides
}

// String says what the diskette turned out to be, which is what the summary
// of a download reports
func (d *FloppyDisk) String() string {
	sides := "single sided"
	if d.sides == 2 {
		sides = "double sided"
	}
	return fmt.Sprintf("%v, a %vKb %v diskette",
		d.name, d.sides*floppySize400K/1024, sides)
}

// IsReadOnly tells if the diskette is locked, which is what the machine sees
// through the write protect line
func (d *FloppyDisk) IsReadOnly() bool {
	return d.readOnly
}

// sectorData gathers the tags and the data of every sector of a track, in the
// order the sector numbers run
func (d *FloppyDisk) sectorData(track int, side int) []uint8 {
	sectors := SectorsInTrack(track)

	out := make([]uint8, 0, sectors*sectorSize)
	for sector := 0; sector < sectors; sector++ {
		block := BlockOf(track, side, sector, d.sides)
		out = append(out, d.tags[block*TagSize:(block+1)*TagSize]...)
		out = append(out, d.data[block*BlockSize:(block+1)*BlockSize]...)
	}

	return out
}

/*
ReadTrack builds the bytes that go round one track of the diskette, which is
what the drive turns under the head. It is worked out from the image every
time it is asked for rather than being kept, since a track is read once when
the head arrives on it and the drive holds on to it from there.
*/
func (d *FloppyDisk) ReadTrack(track int, side int) ([]uint8, error) {
	if track < 0 || track >= TracksPerSide {
		return nil, fmt.Errorf("the track %v is not on the diskette", track)
	}
	if side < 0 || side >= d.sides {
		return nil, fmt.Errorf("%v has no side %v", d.name, side)
	}

	return encodeTrack(track, side, d.sides, d.sectorData(track, side))
}

/*
WriteTrack takes the bytes the machine left on a track and stores whatever
sectors can be made out of them. It returns how many were understood, which is
what a trace reports and what tells a formatting pass from a lost track.

A sector that does not decode is left as it was rather than being cleared. The
machine writes one sector at a time into a track it read first, so most of
what comes back is what was there, and a track that was never read holds sync
where the other sectors would be.
*/
func (d *FloppyDisk) WriteTrack(track int, side int, nibbles []uint8) (int, error) {
	if d.readOnly {
		return 0, fmt.Errorf("%v is locked", d.name)
	}
	if track < 0 || track >= TracksPerSide {
		return 0, fmt.Errorf("the track %v is not on the diskette", track)
	}
	if side < 0 || side >= d.sides {
		return 0, fmt.Errorf("%v has no side %v", d.name, side)
	}

	stored := 0
	for sector, sectorData := range DecodeTrack(nibbles) {
		if sector < 0 || sector >= SectorsInTrack(track) {
			continue
		}

		block := BlockOf(track, side, sector, d.sides)
		copy(d.tags[block*TagSize:], sectorData[:TagSize])
		copy(d.data[block*BlockSize:], sectorData[TagSize:])
		stored++
	}

	if stored != 0 {
		d.modified = true
	}

	return stored, nil
}

// Flush writes the image back to the host if anything has changed
func (d *FloppyDisk) Flush() error {
	if !d.modified || d.readOnly {
		return nil
	}

	out := d.data
	if d.diskCopy {
		out = d.buildDiskCopy()
	}

	if err := os.WriteFile(d.filename, out, 0666); err != nil {
		return fmt.Errorf("can not write %v back: %w", d.filename, err)
	}

	d.modified = false
	return nil
}

/*
The DiskCopy 4.2 format, which is how a Macintosh diskette is usually kept:
an 84 byte header, the sectors, and the tags after them.

	 0  64  the name of the volume, as a Pascal string
	64   4  the size of the sectors
	68   4  the size of the tags
	72   4  a checksum of the sectors
	76   4  a checksum of the tags
	80   1  the encoding, 0 for a 400Kb diskette and 1 for an 800Kb one
	81   1  the format byte, as the address fields of the disk carry it
	82   2  $0100, which is what says the file is one of these
*/
const (
	diskCopyHeaderSize = 84

	diskCopyPrivate uint16 = 0x0100

	diskCopyEncoding400K uint8 = 0
	diskCopyEncoding800K uint8 = 1
)

type diskCopyHeader struct {
	name     string
	dataSize int
	tagSize  int
}

/*
parseDiskCopyHeader reads the header if the file has one. Anything that does
not fit is a plain image, which is the other thing it could be.

Only the header itself has to be there, so that a file can be recognised from
its first 84 bytes without being read whole. Whether the rest of it is as long
as the header claims is the caller's to check.
*/
func parseDiskCopyHeader(raw []uint8) (diskCopyHeader, bool) {
	var header diskCopyHeader

	if len(raw) < diskCopyHeaderSize {
		return header, false
	}
	if binary.BigEndian.Uint16(raw[82:84]) != diskCopyPrivate {
		return header, false
	}

	/*
		The size is not checked against what this machine can read. A
		DiskCopy file is a diskette whatever is in it, and one holding a
		1.44Mb disk has to be recognised as the diskette it is so that it
		can be turned away for the right reason.
	*/
	dataSize := int(binary.BigEndian.Uint32(raw[64:68]))
	tagSize := int(binary.BigEndian.Uint32(raw[68:72]))
	if dataSize <= 0 || dataSize > floppySize1440K {
		return header, false
	}

	length := int(raw[0])
	if length > 63 {
		length = 63
	}

	header.name = string(raw[1 : 1+length])
	header.dataSize = dataSize
	header.tagSize = tagSize

	return header, true
}

// buildDiskCopy puts the image back together with a header in front of it
func (d *FloppyDisk) buildDiskCopy() []uint8 {
	out := make([]uint8, diskCopyHeaderSize+len(d.data)+len(d.tags))

	name := d.name
	if len(name) > 63 {
		name = name[:63]
	}
	out[0] = uint8(len(name))
	copy(out[1:], name)

	binary.BigEndian.PutUint32(out[64:68], uint32(len(d.data)))
	binary.BigEndian.PutUint32(out[68:72], uint32(len(d.tags)))
	binary.BigEndian.PutUint32(out[72:76], diskCopyChecksum(d.data))

	/*
		The tags of the first sector are left out of their checksum. The
		file system keeps the boot blocks there and DiskCopy found that
		what it put in those twelve bytes was not worth comparing.
	*/
	binary.BigEndian.PutUint32(out[76:80], diskCopyChecksum(d.tags[min(TagSize, len(d.tags)):]))

	out[80] = diskCopyEncoding400K
	out[81] = formatSingleSided
	if d.sides == 2 {
		out[80] = diskCopyEncoding800K
		out[81] = formatDoubleSided
	}

	binary.BigEndian.PutUint16(out[82:84], diskCopyPrivate)

	copy(out[diskCopyHeaderSize:], d.data)
	copy(out[diskCopyHeaderSize+len(d.data):], d.tags)

	return out
}

/*
diskCopyChecksum adds the image up a word at a time, turning the running total
over by one bit after each, which is the sum the header carries. An odd
trailing byte can not happen on a diskette and is ignored.
*/
func diskCopyChecksum(data []uint8) uint32 {
	var sum uint32

	for i := 0; i+1 < len(data); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(data[i : i+2]))
		sum = sum>>1 | sum<<31
	}

	return sum
}

/*
SectorsInTrack is how many sectors a track holds. The disk turns slower the
further out the head is, in five bands of sixteen tracks, so that the bits
stay the same length along the track and the outer ones hold more of them.
Twelve down to eight over the eighty tracks is 800 sectors a side.
*/
func SectorsInTrack(track int) int {
	return 12 - track/16
}

const (
	// TracksPerSide is how far the head can go, which is what stops the
	// stepper of the drive
	TracksPerSide = 80

	// sectorsPerSide is the sum of SectorsInTrack over every track
	sectorsPerSide = 16 * (12 + 11 + 10 + 9 + 8)
)

/*
BlockOf gives the position in the image of a sector. The blocks run along a
track, then over to the other side of the same track, then outwards, which is
the order a single sided image keeps too once the side is always zero.
*/
func BlockOf(track int, side int, sector int, sides int) int {
	block := 0
	for t := 0; t < track; t++ {
		block += SectorsInTrack(t) * sides
	}
	return block + side*SectorsInTrack(track) + sector
}

// interleave is what the driver formats with, a sector and the next one along
// half a turn apart
const interleave = 2

/*
interleavedOrder is the order the sectors are laid out around the track. The
driver formats two to one, so that a sector and the next one along are half a
turn apart and the machine has time to deal with the first before the second
arrives.

Nothing reads this back: a sector is found by its address field wherever it
is. It is here so that a track izmac writes looks like one the machine wrote.
*/
func interleavedOrder(sectors int) []int {
	order := make([]int, sectors)
	for i := range order {
		order[i] = -1
	}

	position := 0
	for sector := 0; sector < sectors; sector++ {
		for order[position] != -1 {
			position = (position + 1) % sectors
		}
		order[position] = sector
		position = (position + interleave) % sectors
	}

	return order
}

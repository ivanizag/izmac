package storage

import (
	"bytes"
	"encoding/binary"
)

/*
Telling a disk image from anything else that comes out of an archive, and
mending the one way the images people pass around are commonly broken. Both
follow macprep (https://github.com/mastorak/macprep), which does the same for
the images it gives Mini vMac.

Once an archive is unpacked there is no file system to say which of the files
in it are disk images, and nothing else is any use to the machine yet: an
application or a read me would need a volume built around it. A disk image is
known by what is in it, the same things Classify looks at, plus the master
directory block of a volume with no map in front of it, HFS or the MFS of the
first machines.

The breakage is padding. Copying programs and transfers over serial lines left
a diskette image a little longer than the diskette, 401Kb or 807Kb, which is a
size no drive takes and so would go on the bus as a hard disk. When the volume
inside ends where a diskette would, the extra is cut off.
*/

const (
	// mfsVolumeSignature is what the master directory block of an MFS
	// volume starts with, at the same place as the HFS one
	mfsVolumeSignature = 0xd2d7

	// maxPadding is how much longer than a diskette an image can be and
	// still be taken for one that was padded
	maxPadding = 64 << 10

	// The fields of the master directory block that say where the volume
	// ends, at the same offsets in HFS and MFS
	mdbAllocationBlocks    = 0x12
	mdbAllocationBlockSize = 0x14
	mdbFirstBlock          = 0x1c
	mdbFieldsEnd           = 0x1e
)

/*
IsDiskImage tells whether something taken out of an archive is a disk image,
as opposed to a file that was meant to be copied onto one
*/
func IsDiskImage(data []uint8) bool {
	if _, ok := parseDiskCopyHeader(data); ok {
		return true
	}

	if len(data) >= 2 && binary.BigEndian.Uint16(data) == driverDescriptorSignature {
		return true
	}

	switch len(data) {
	case floppySize400K, floppySize800K, floppySize720K, floppySize1440K:
		return true
	}

	_, ok := volumeSignature(data)
	return ok
}

/*
PaddedFloppySize says whether an image is a diskette with something stuck on
the end, and what size it should be. It needs no more of the image than the
start of its master directory block, so that a hard disk image of any size can
be looked at without being read.
*/
func PaddedFloppySize(head []uint8, size int64) (int64, bool) {
	if _, ok := parseDiskCopyHeader(head); ok {
		return 0, false
	}

	if _, ok := volumeSignature(head); !ok {
		return 0, false
	}

	mdb := head[volumeHeaderBlock*BlockSize:]
	blocks := int64(binary.BigEndian.Uint16(mdb[mdbAllocationBlocks:]))
	blockSize := int64(binary.BigEndian.Uint32(mdb[mdbAllocationBlockSize:]))
	if blocks == 0 || blockSize == 0 || blockSize%BlockSize != 0 {
		return 0, false
	}

	end := int64(binary.BigEndian.Uint16(mdb[mdbFirstBlock:]))*BlockSize +
		blocks*blockSize +
		// The copy of the master directory block, in the block before
		// last, and the one block after it
		2*BlockSize

	for _, floppy := range []int64{floppySize400K, floppySize800K} {
		if size > floppy && size <= floppy+maxPadding && end <= floppy {
			return floppy, true
		}
	}
	return 0, false
}

// volumeSignature is the signature of a volume starting at the front of an
// image, HFS or MFS, if there is one
func volumeSignature(data []uint8) (uint16, bool) {
	at := volumeHeaderBlock * BlockSize
	if len(data) < at+mdbFieldsEnd {
		return 0, false
	}

	signature := binary.BigEndian.Uint16(data[at:])
	if signature != hfsVolumeSignature && signature != mfsVolumeSignature {
		return 0, false
	}

	// A block of zeros with the two letters in front is not a volume
	if bytes.Count(data[at:at+mdbFieldsEnd], []uint8{0}) == mdbFieldsEnd-2 {
		return 0, false
	}
	return signature, true
}

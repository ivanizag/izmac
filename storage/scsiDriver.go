package storage

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
)

/*
What a disk a Macintosh Plus can boot from is made of, and where to find a
SCSI driver to put in one.

Images made for the emulators that patch the ROM are the volume and nothing
else: the boot blocks are at block 0 and there is no sign of Apple's
formatter. The ROM of a real machine wants more than that. It reads block 0
looking for a driver descriptor map, takes the first driver descriptor of
type 1, loads that many blocks into memory and jumps to the first of them,
with the second block of the disk in A0. Everything after that, finding the
volume and reading it, is the SCSI driver's work and not the ROM's.

So a bootable disk is the driver descriptor map, a partition map, a SCSI
driver, and the volume after them. The layout below is the one Apple's HD SC
Setup writes and the one the SCSI drivers expect:

	block 0        driver descriptor map
	blocks 1-63    partition map, three entries used of the 63 reserved
	blocks 64-95   the SCSI driver
	blocks 96-     the HFS volume

None of it is ever written to a file. buildHeader lays out everything before
the volume and bareVolume.go holds it in memory in front of one, so a bare
volume can go on the bus without being touched.

The SCSI driver is not built here either. It is real Macintosh code, lifted
from a disk that already has one, for the same reason izmac does not carry a
ROM: it is Apple's code and not ours to hand out.
*/

const (
	// driverDescriptorSize is the block 0 record, 'ER' and what follows
	driverDescriptorSize = 18

	// partitionSignature is the 'PM' every partition map entry starts with
	partitionSignature = 0x504d

	// macintoshDriverType is the ddType the Plus ROM looks for. A disk can
	// carry drivers for more than one kind of machine and this is ours.
	macintoshDriverType = 1

	// The layout, in blocks. The partition map is given the 63 blocks
	// Apple's formatter reserves even though three entries fill only
	// three, and the SCSI driver the 32 it is given there too.
	partitionMapStart  = 1
	partitionMapBlocks = 63
	scsiDriverStart    = 64
	scsiDriverBlocks   = 32
	volumeStart        = scsiDriverStart + scsiDriverBlocks
)

/*
The status words Apple's formatter writes. They are flags the SCSI driver
reads and there is nothing to work out about them, so they are copied as they
are found on a working disk.
*/
const (
	statusPartitionMap = 0x37
	statusVolume       = 0xb7
)

// ScsiDriver is the SCSI driver taken off a disk that already boots, along
// with the partition map entry that describes it
type ScsiDriver struct {
	// code is the SCSI driver itself, a whole number of blocks
	code []uint8

	/*
		entry is the whole block the donor's partition map spends on the
		SCSI driver, carried over as it is rather than built again. Past the
		fields Inside Macintosh documents there is more in there, a table
		the SCSI driver reads to find itself, and since the driver being
		described is the same one byte for byte, everything the donor said
		about it still holds. Building the entry instead means writing out
		fields nobody here understands, and a SCSI driver that loads and
		then stops without a word.
	*/
	entry []uint8

	// Processor is the cpu the SCSI driver is for, '68000' on the disks a
	// Plus can use. Read out of the entry for the messages.
	Processor string

	// source is where it came from, for the messages
	source string
}

// Blocks is the size of the SCSI driver, which is what the driver descriptor
// and the partition map entry both count in
func (s *ScsiDriver) Blocks() uint32 {
	return uint32(len(s.code) / BlockSize)
}

/*
ReadScsiDriver takes the SCSI driver out of a partitioned disk image. Any disk
that boots a Macintosh has one, and it is found the same way the ROM finds
it: the driver descriptor map at block 0 says which blocks it lives in.
*/
func ReadScsiDriver(filename string) (*ScsiDriver, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("can not open the donor image: %w", err)
	}
	defer file.Close()

	return readScsiDriverAt(file, filename)
}

/*
readScsiDriverAt is ReadScsiDriver over anything that reads, so that an image
can be looked at before it is a file. Only the blocks the SCSI driver is in
are touched, which is why a few blocks off the front of a disk are as good as
the whole of one.
*/
func readScsiDriverAt(r io.ReaderAt, filename string) (*ScsiDriver, error) {
	block0 := make([]uint8, BlockSize)
	_, err := r.ReadAt(block0, 0)
	if err != nil {
		return nil, fmt.Errorf("can not read block 0 of %v: %w", filename, err)
	}

	if binary.BigEndian.Uint16(block0) != driverDescriptorSignature {
		return nil, fmt.Errorf("%v does not start with a driver descriptor map, "+
			"so it has no SCSI driver to take", filename)
	}

	blockSize := binary.BigEndian.Uint16(block0[2:])
	if blockSize != BlockSize {
		return nil, fmt.Errorf("%v is in blocks of %v bytes, izmac only knows %v",
			filename, blockSize, BlockSize)
	}

	// The driver descriptors follow the record, eight bytes each: the block
	// it starts at, how many blocks it takes, and what machine it is for
	count := binary.BigEndian.Uint16(block0[16:])
	var start, size uint32
	found := false
	for i := 0; i < int(count); i++ {
		entry := block0[driverDescriptorSize+i*8:]
		if binary.BigEndian.Uint16(entry[6:]) == macintoshDriverType {
			start = binary.BigEndian.Uint32(entry)
			size = uint32(binary.BigEndian.Uint16(entry[4:]))
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("%v carries %v driver(s), none of them of the type "+
			"a Macintosh loads", filename, count)
	}

	loaded := make([]uint8, size*BlockSize)
	_, err = r.ReadAt(loaded, int64(start)*BlockSize)
	if err != nil {
		return nil, fmt.Errorf("can not read the SCSI driver of %v at block %v: %w",
			filename, start, err)
	}

	scsiDriver := &ScsiDriver{
		code:   loaded,
		source: filename,
	}

	err = scsiDriver.readEntry(r, start)
	if err != nil {
		return nil, err
	}

	return scsiDriver, nil
}

/*
readEntry walks the partition map for the entry that covers the SCSI driver
and keeps the whole block.
*/
func (s *ScsiDriver) readEntry(r io.ReaderAt, scsiDriverBlock uint32) error {
	entry := make([]uint8, BlockSize)
	_, err := r.ReadAt(entry, partitionMapStart*BlockSize)
	if err != nil {
		return fmt.Errorf("can not read the partition map of %v: %w", s.source, err)
	}
	if binary.BigEndian.Uint16(entry) != partitionSignature {
		return fmt.Errorf("%v has a SCSI driver but no partition map", s.source)
	}

	entries := binary.BigEndian.Uint32(entry[4:])
	for i := uint32(0); i < entries; i++ {
		_, err = r.ReadAt(entry, int64(partitionMapStart+i)*BlockSize)
		if err != nil {
			return err
		}
		if binary.BigEndian.Uint16(entry) != partitionSignature {
			continue
		}
		if binary.BigEndian.Uint32(entry[8:]) != scsiDriverBlock {
			continue
		}

		s.entry = entry
		s.Processor = strings.TrimRight(string(entry[120:136]), "\x00")
		return nil
	}

	return fmt.Errorf("%v has a SCSI driver at block %v that no partition map entry covers",
		s.source, scsiDriverBlock)
}

/*
bareVolumeError is what to tell someone holding an image the machine can not
use. It names the way out, which is not something they could work out from
the blinking diskette the ROM would otherwise leave them with.
*/
func bareVolumeError(filename string) error {
	return fmt.Errorf("%v is a bare HFS volume, with no partition map and no "+
		"SCSI driver on it. Images like this one are made for the emulators that "+
		"patch the ROM to supply a driver of their own, and the ROM izmac runs "+
		"boots by loading one off the disk. Name a disk that has one and izmac "+
		"makes up the rest as it goes, writing to neither:\n"+
		"    izmac -scsidriver <a disk that boots> %v",
		filename, filename)
}

/*
buildHeader lays out the driver descriptor map, the partition map and the
SCSI driver, which is everything that comes before the volume on a disk the ROM
can boot. It is the same bytes whether they are written to an image or kept
in memory in front of one.
*/
func buildHeader(scsiDriver *ScsiDriver, volumeBlocks uint32) []uint8 {
	header := make([]uint8, volumeStart*BlockSize)

	// Block 0, the driver descriptor map. The block count is where the
	// volume ends rather than where the file does, which is what a
	// formatter writes when the disk has slack at the end.
	binary.BigEndian.PutUint16(header, driverDescriptorSignature)
	binary.BigEndian.PutUint16(header[2:], BlockSize)
	binary.BigEndian.PutUint32(header[4:], volumeStart+volumeBlocks)
	binary.BigEndian.PutUint16(header[8:], 1)  // sbDevType
	binary.BigEndian.PutUint16(header[10:], 1) // sbDevId
	binary.BigEndian.PutUint16(header[16:], 1) // one SCSI driver follows
	binary.BigEndian.PutUint32(header[18:], scsiDriverStart)
	binary.BigEndian.PutUint16(header[22:], uint16(scsiDriver.Blocks()))
	binary.BigEndian.PutUint16(header[24:], macintoshDriverType)

	entries := []struct {
		name   string
		kind   string
		start  uint32
		blocks uint32
		status uint32
	}{
		{"Apple", "Apple_partition_map", partitionMapStart, partitionMapBlocks, statusPartitionMap},
		{"", "", scsiDriverStart, scsiDriverBlocks, 0}, // the donor's, copied below
		{"MacOS", "Apple_HFS", volumeStart, volumeBlocks, statusVolume},
	}

	for i, e := range entries {
		block := header[(partitionMapStart+i)*BlockSize:]

		if e.start == scsiDriverStart {
			copy(block, scsiDriver.entry)
		} else {
			binary.BigEndian.PutUint16(block, partitionSignature)
			copy(block[16:48], e.name)
			copy(block[48:80], e.kind)
			binary.BigEndian.PutUint32(block[88:], e.status)
		}

		// Where each partition sits is ours to say, whether the rest of the
		// entry was built here or came off the donor
		binary.BigEndian.PutUint32(block[4:], uint32(len(entries)))
		binary.BigEndian.PutUint32(block[8:], e.start)
		binary.BigEndian.PutUint32(block[12:], e.blocks)
		binary.BigEndian.PutUint32(block[84:], e.blocks) // pmDataCnt
	}

	copy(header[scsiDriverStart*BlockSize:], scsiDriver.code)

	return header
}

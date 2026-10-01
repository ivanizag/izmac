package storage

import (
	"encoding/binary"
	"os"
	"testing"
)

/*
volumeImage makes an image with an HFS master directory block saying the
volume ends at volumeSize, in an image of imageSize. The allocation blocks are
512 bytes and start after the volume bitmap, as on a diskette.
*/
func volumeImage(volumeSize int, imageSize int) []uint8 {
	data := make([]uint8, imageSize)
	mdb := data[volumeHeaderBlock*BlockSize:]

	const firstBlock = 4
	binary.BigEndian.PutUint16(mdb, hfsVolumeSignature)
	binary.BigEndian.PutUint16(mdb[mdbAllocationBlocks:],
		uint16((volumeSize-firstBlock*BlockSize-2*BlockSize)/BlockSize))
	binary.BigEndian.PutUint32(mdb[mdbAllocationBlockSize:], BlockSize)
	binary.BigEndian.PutUint16(mdb[mdbFirstBlock:], firstBlock)
	return data
}

func TestADisketteWithPaddingIsCutBack(t *testing.T) {
	for _, floppy := range []int{floppySize400K, floppySize800K} {
		data := volumeImage(floppy, floppy+1024)

		size, padded := PaddedFloppySize(data[:4096], int64(len(data)))
		if !padded || size != int64(floppy) {
			t.Errorf("a %vKb diskette padded by 1Kb gave %v, %v", floppy/1024, size, padded)
		}
	}
}

func TestAVolumeBiggerThanADisketteIsNotCutBack(t *testing.T) {
	// The volume itself runs past 800Kb, so the extra is not padding
	data := volumeImage(floppySize800K+2048, floppySize800K+2048)

	if _, padded := PaddedFloppySize(data[:4096], int64(len(data))); padded {
		t.Errorf("a volume of 802Kb was taken for a padded diskette")
	}
}

func TestAnImageOfTheRightSizeIsNotPadded(t *testing.T) {
	data := volumeImage(floppySize800K, floppySize800K)

	if _, padded := PaddedFloppySize(data[:4096], int64(len(data))); padded {
		t.Errorf("an 800Kb diskette was taken for a padded one")
	}
}

func TestADiskCopyImageIsNotPadded(t *testing.T) {
	// 84 bytes longer than the diskette, and the volume where a plain
	// image has it
	data := volumeImage(floppySize400K, floppySize400K+diskCopyHeaderSize)
	binary.BigEndian.PutUint32(data[64:], floppySize400K)
	binary.BigEndian.PutUint16(data[82:], diskCopyPrivate)

	if _, padded := PaddedFloppySize(data[:4096], int64(len(data))); padded {
		t.Errorf("a DiskCopy image was taken for a padded diskette")
	}
}

func TestDiskImagesAreToldFromOtherFiles(t *testing.T) {
	partitioned := make([]uint8, 4096)
	binary.BigEndian.PutUint16(partitioned, driverDescriptorSignature)

	diskCopy := make([]uint8, diskCopyHeaderSize+floppySize400K)
	binary.BigEndian.PutUint32(diskCopy[64:], floppySize400K)
	binary.BigEndian.PutUint16(diskCopy[82:], diskCopyPrivate)

	mfs := volumeImage(floppySize400K, 600*1024)
	binary.BigEndian.PutUint16(mfs[volumeHeaderBlock*BlockSize:], mfsVolumeSignature)

	images := map[string][]uint8{
		"partitioned":               partitioned,
		"DiskCopy":                  diskCopy,
		"800Kb":                     make([]uint8, floppySize800K),
		"bare HFS volume":           volumeImage(2<<20, 2<<20),
		"bare MFS volume":           mfs,
		"padded diskette":           volumeImage(floppySize400K, floppySize400K+512),
		"1.44Mb, turned away later": make([]uint8, floppySize1440K),
	}
	for name, data := range images {
		if !IsDiskImage(data) {
			t.Errorf("a %v image was not taken for one", name)
		}
	}

	// Two letters where the master directory block goes do not make one
	signatureOnly := make([]uint8, 100*1024)
	binary.BigEndian.PutUint16(signatureOnly[volumeHeaderBlock*BlockSize:], hfsVolumeSignature)

	others := map[string][]uint8{
		"read me":        []uint8("Read me first, then install."),
		"application":    make([]uint8, 123456),
		"signature only": signatureOnly,
	}
	for name, data := range others {
		if IsDiskImage(data) {
			t.Errorf("a %v was taken for a disk image", name)
		}
	}
}

func TestClassifyingInMemoryAgreesWithTheFile(t *testing.T) {
	for _, partitioned := range []bool{true, false} {
		for _, size := range []int{floppySize800K, 4 << 20} {
			filename := writeImage(t, size, partitioned)

			fromFile, err := Classify(filename)
			if err != nil {
				t.Fatal(err)
			}

			data := make([]uint8, size)
			if partitioned {
				binary.BigEndian.PutUint16(data, driverDescriptorSignature)
			}
			if fromMemory := ClassifyData(data); fromMemory != fromFile {
				t.Errorf("an image of %v bytes is a %v in a file and a %v in memory",
					size, fromFile, fromMemory)
			}
		}
	}

	if kind := ClassifyData(volumeImage(2<<20, 2<<20)); kind != KindBareVolume {
		t.Errorf("a bare volume in memory was taken for a %v", kind)
	}
}

func TestADisketteInMemoryIsWritableAndItsOwn(t *testing.T) {
	data := make([]uint8, floppySize800K)
	data[0] = 'L'

	// The name is a path that would be written to if anything were
	t.Chdir(t.TempDir())
	d, err := NewFloppyDiskData("Disk.dsk", data)
	if err != nil {
		t.Fatal(err)
	}

	if d.IsReadOnly() {
		t.Errorf("a diskette held in memory is locked")
	}
	if d.Sides() != 2 {
		t.Errorf("an 800Kb diskette has %v sides", d.Sides())
	}

	data[0] = 'X'
	if d.data[0] != 'L' {
		t.Errorf("the diskette changed with the slice it was made from")
	}

	// The machine writes a track, which is kept in memory and goes no
	// further when the diskette is flushed
	track, err := d.ReadTrack(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if stored, err := d.WriteTrack(0, 0, track); err != nil || stored == 0 {
		t.Fatalf("writing a track stored %v sectors: %v", stored, err)
	}
	if err := d.Flush(); err != nil {
		t.Errorf("flushing a diskette held in memory failed: %v", err)
	}
	if _, err := os.Stat("Disk.dsk"); err == nil {
		t.Errorf("a diskette held in memory was written to a file")
	}
}

func TestAHardDiskInMemoryTakesWrites(t *testing.T) {
	data := make([]uint8, 4<<20)
	binary.BigEndian.PutUint16(data, driverDescriptorSignature)

	disk, err := NewBlockDiskData("archive.zip/Hard.img", data, nil)
	if err != nil {
		t.Fatal(err)
	}

	block := make([]uint8, BlockSize)
	block[0] = 'W'
	if disk.IsReadOnly() || disk.Write(10, block) != nil {
		t.Fatalf("a hard disk held in memory refused a write")
	}
	if read, err := disk.Read(10); err != nil || read[0] != 'W' {
		t.Errorf("the block written did not read back")
	}
	if disk.Name() != "archive.zip/Hard.img" || disk.Blocks() != 8192 {
		t.Errorf("the disk is %v of %v blocks", disk.Name(), disk.Blocks())
	}
}

func TestABareVolumeInMemoryWantsASCSIDriver(t *testing.T) {
	if _, err := NewBlockDiskData("volume.dsk", volumeImage(2<<20, 2<<20), nil); err == nil {
		t.Errorf("a bare volume went on the bus with no SCSI driver to boot it")
	}
}

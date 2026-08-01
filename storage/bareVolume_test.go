package storage

import (
	"bytes"
	"encoding/binary"
	"os"
	"strings"
	"testing"
)

// attachVolume dresses a bare volume of the given size, the way the bus gets
// one
func attachVolume(t *testing.T, blocks int) (BlockDisk, string) {
	t.Helper()

	driver, err := ReadDriver(writeDonor(t))
	if err != nil {
		t.Fatal(err)
	}

	filename := writeVolume(t, blocks)
	disk, err := NewBlockDisk(filename, driver, false)
	if err != nil {
		t.Fatal(err)
	}
	return disk, filename
}

/*
The machine has to see a disk the ROM can boot: a driver descriptor map where
it looks for one, sending it to a driver that is really there.
*/
func TestABareVolumeComesUpLookingPartitioned(t *testing.T) {
	disk, _ := attachVolume(t, 40)

	block0, err := disk.Read(0)
	if err != nil {
		t.Fatal(err)
	}
	if binary.BigEndian.Uint16(block0) != driverDescriptorSignature {
		t.Fatalf("block 0 of a dressed volume is %x, not a driver descriptor map",
			binary.BigEndian.Uint16(block0))
	}

	// The descriptor has to name a driver of the type the Plus ROM loads
	if got := binary.BigEndian.Uint16(block0[24:]); got != macintoshDriverType {
		t.Errorf("the descriptor is of type %v, which no Macintosh loads", got)
	}

	// And what it points at has to be the driver
	at := binary.BigEndian.Uint32(block0[18:])
	driverBlock, err := disk.Read(at)
	if err != nil {
		t.Fatal(err)
	}
	if driverBlock[1] != 1 {
		t.Errorf("block %v is not the driver, it starts %v", at, driverBlock[:4])
	}
}

/*
The map the machine reads has to describe the volume that is really behind
it, or the driver goes looking in the wrong place.
*/
func TestTheMadeUpMapDescribesTheVolume(t *testing.T) {
	const blocks = 40
	disk, _ := attachVolume(t, blocks)

	// The volume entry is the third of the three built
	entry, err := disk.Read(partitionMapStart + 2)
	if err != nil {
		t.Fatal(err)
	}

	if binary.BigEndian.Uint16(entry) != partitionSignature {
		t.Fatal("the third map entry is not one")
	}
	if got := binary.BigEndian.Uint32(entry[8:]); got != volumeStart {
		t.Errorf("the volume entry starts at %v, the volume is at %v", got, volumeStart)
	}
	if got := binary.BigEndian.Uint32(entry[12:]); got != blocks {
		t.Errorf("the volume entry is %v blocks, the volume is %v", got, blocks)
	}
	if kind := strings.TrimRight(string(entry[48:80]), "\x00"); kind != "Apple_HFS" {
		t.Errorf("the volume entry is of type %q", kind)
	}
}

/*
The driver's entry says more about it than this package knows how to write,
so it is carried over whole from wherever the driver came from. Building it
from the documented fields alone leaves the driver loading and then going
quiet, which is a long way to find a missing word.
*/
func TestTheDriverEntryIsCarriedOverWhole(t *testing.T) {
	disk, _ := attachVolume(t, 40)

	// The driver entry is the second of the three
	entry, err := disk.Read(partitionMapStart + 1)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(entry[136:136+len(driverMagic)], driverMagic) {
		t.Errorf("the driver entry lost what was past the fields built here: %v",
			entry[136:136+len(driverMagic)])
	}
	if got := strings.TrimRight(string(entry[120:136]), "\x00"); got != "68000" {
		t.Errorf("the driver entry names the processor %q", got)
	}
	// Where it sits is still this package's to say
	if got := binary.BigEndian.Uint32(entry[8:]); got != driverStart {
		t.Errorf("the driver entry covers block %v, the driver is at %v", got, driverStart)
	}
}

/*
The volume itself has to come back from where the partition map says it is,
and unchanged. Everything the machine reads of the disk beyond the made up
blocks is the file, 96 blocks earlier.
*/
func TestTheVolumeIsReadThroughTheOffset(t *testing.T) {
	const blocks = 40
	disk, filename := attachVolume(t, blocks)

	file, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}

	if disk.Blocks() != volumeStart+blocks {
		t.Errorf("the disk is %v blocks, wanted the volume's %v plus the %v in front",
			disk.Blocks(), blocks, volumeStart)
	}

	for _, block := range []uint32{0, 2, 17, blocks - 1} {
		got, err := disk.Read(volumeStart + block)
		if err != nil {
			t.Fatal(err)
		}
		want := file[block*BlockSize : (block+1)*BlockSize]
		if !bytes.Equal(got, want) {
			t.Errorf("block %v of the volume came back wrong through the offset", block)
		}
	}
}

// A read off the end is an error rather than a made up block
func TestReadingPastTheEndOfADressedVolumeFails(t *testing.T) {
	disk, _ := attachVolume(t, 40)

	if _, err := disk.Read(volumeStart + 40); err == nil {
		t.Error("a block past the end of the volume was read")
	}
}

/*
A write lands in the file at the offset the machine never sees, which is what
makes the disk usable rather than just bootable.
*/
func TestAWriteReachesTheVolume(t *testing.T) {
	const blocks = 40
	disk, filename := attachVolume(t, blocks)

	data := make([]uint8, BlockSize)
	for i := range data {
		data[i] = 0xa5
	}
	if err := disk.Write(volumeStart+9, data); err != nil {
		t.Fatal(err)
	}

	file, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(file[9*BlockSize:10*BlockSize], data) {
		t.Error("a write to the disk did not reach block 9 of the volume")
	}
	if len(file) != blocks*BlockSize {
		t.Errorf("the file grew to %v blocks", len(file)/BlockSize)
	}
}

/*
The made up blocks are not the file's, so a write to them has to stay in
memory. Letting one through would put a partition map on the front of a
volume that has no room for one and overwrite the boot blocks.
*/
func TestAWriteToTheMadeUpBlocksStaysInMemory(t *testing.T) {
	disk, filename := attachVolume(t, 40)

	before, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}

	data := make([]uint8, BlockSize)
	for i := range data {
		data[i] = 0x5a
	}
	if err := disk.Write(1, data); err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("a write to the partition map reached the file")
	}

	// It is remembered, though, rather than dropped
	got, err := disk.Read(1)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Error("a write to the partition map was not remembered")
	}
}

// A partitioned disk needs none of this and goes on the bus untouched
func TestAPartitionedDiskIsNotDressed(t *testing.T) {
	donor := writeDonor(t)

	disk, err := NewBlockDisk(donor, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if disk.Blocks() != driverStart+donorDriverBlocks {
		t.Errorf("a partitioned disk came up %v blocks, it was not left alone",
			disk.Blocks())
	}
}

// Without a driver there is nothing to send the ROM to, so the volume is
// turned away rather than attached to fail later
func TestABareVolumeWithoutADriverIsRefused(t *testing.T) {
	_, err := NewBlockDisk(writeVolume(t, 40), nil, false)
	if err == nil {
		t.Fatal("a bare volume was attached with no driver to boot through")
	}
}

package storage

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// donorScsiDriverBlocks is how long the SCSI driver of the made up donor is
const donorScsiDriverBlocks = 2

/*
scsiDriverMagic sits in the part of the donor's SCSI driver entry that this
package does not build for itself. Finding it again in the image written is
what says the entry was carried over whole rather than put together from the
fields that are understood.
*/
var scsiDriverMagic = []uint8{0x00, 0x01, 0x06, 0x00}

/*
writeDonor makes a disk image with everything ReadScsiDriver looks for: a
driver descriptor map naming a SCSI driver, a partition map with an entry
covering it, and the SCSI driver itself.
*/
func writeDonor(t *testing.T) string {
	t.Helper()

	data := make([]uint8, (scsiDriverStart+donorScsiDriverBlocks)*BlockSize)

	binary.BigEndian.PutUint16(data, driverDescriptorSignature)
	binary.BigEndian.PutUint16(data[2:], BlockSize)
	binary.BigEndian.PutUint16(data[16:], 2) // two drivers, only one for us
	// The first is for some other machine, so that the search has to skip it
	binary.BigEndian.PutUint32(data[18:], 8)
	binary.BigEndian.PutUint16(data[22:], 1)
	binary.BigEndian.PutUint16(data[24:], 0xffff)
	binary.BigEndian.PutUint32(data[26:], scsiDriverStart)
	binary.BigEndian.PutUint16(data[30:], donorScsiDriverBlocks)
	binary.BigEndian.PutUint16(data[32:], macintoshDriverType)

	entries := []struct {
		kind   string
		start  uint32
		blocks uint32
	}{
		{"Apple_partition_map", partitionMapStart, partitionMapBlocks},
		{"Apple_Driver43", scsiDriverStart, scsiDriverBlocks},
		{"Apple_HFS", volumeStart, 16},
	}
	for i, e := range entries {
		block := data[(partitionMapStart+i)*BlockSize:]
		binary.BigEndian.PutUint16(block, partitionSignature)
		binary.BigEndian.PutUint32(block[4:], uint32(len(entries)))
		binary.BigEndian.PutUint32(block[8:], e.start)
		binary.BigEndian.PutUint32(block[12:], e.blocks)
		copy(block[48:80], e.kind)
		if e.start == scsiDriverStart {
			copy(block[120:136], "68000")
			copy(block[136:], scsiDriverMagic)
		}
	}

	// The SCSI driver is not run by any test, only moved about, so anything
	// recognizable will do
	for i := 0; i < donorScsiDriverBlocks*BlockSize; i++ {
		data[scsiDriverStart*BlockSize+i] = uint8(i)
	}

	filename := filepath.Join(t.TempDir(), "donor.img")
	if err := os.WriteFile(filename, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return filename
}

// writeVolume makes a bare HFS volume, the boot blocks and a master
// directory block and nothing before them
func writeVolume(t *testing.T, blocks int) string {
	t.Helper()

	data := make([]uint8, blocks*BlockSize)
	data[0], data[1] = 'L', 'K'
	binary.BigEndian.PutUint16(data[2*BlockSize:], hfsVolumeSignature)
	for i := range data {
		if i >= 3*BlockSize {
			data[i] = uint8(i * 7)
		}
	}

	filename := filepath.Join(t.TempDir(), "volume.dsk")
	if err := os.WriteFile(filename, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return filename
}

func TestTheScsiDriverIsFoundThroughTheDescriptorMap(t *testing.T) {
	scsiDriver, err := ReadScsiDriver(writeDonor(t))
	if err != nil {
		t.Fatal(err)
	}

	if scsiDriver.Blocks() != donorScsiDriverBlocks {
		t.Errorf("the SCSI driver came out %v blocks long, not %v",
			scsiDriver.Blocks(), donorScsiDriverBlocks)
	}
	if scsiDriver.Processor != "68000" {
		t.Errorf("the SCSI driver is for %q, not the 68000 the entry names", scsiDriver.Processor)
	}
	if scsiDriver.code[1] != 1 {
		t.Errorf("the SCSI driver was read from the wrong place, it starts %v", scsiDriver.code[:4])
	}
}

/*
A disk can carry a SCSI driver for more than one kind of machine, and only the
type the Plus ROM loads is any use here. The donor names another one first.
*/
func TestAScsiDriverForAnotherMachineIsPassedOver(t *testing.T) {
	scsiDriver, err := ReadScsiDriver(writeDonor(t))
	if err != nil {
		t.Fatal(err)
	}
	if binary.BigEndian.Uint32(scsiDriver.entry[8:]) != scsiDriverStart {
		t.Errorf("the entry kept covers block %v, not the SCSI driver at %v",
			binary.BigEndian.Uint32(scsiDriver.entry[8:]), scsiDriverStart)
	}
}

func TestAnImageWithNoDescriptorMapHasNoScsiDriver(t *testing.T) {
	_, err := ReadScsiDriver(writeVolume(t, 8))
	if err == nil {
		t.Fatal("a bare volume was asked for a SCSI driver and gave one")
	}
	if !strings.Contains(err.Error(), "driver descriptor map") {
		t.Errorf("the error was %q, which does not say what is missing", err)
	}
}

package storage

import "fmt"

/*
Putting a bare HFS volume on the bus without touching the file it lives in.

The volume is all there is in one of these images: no driver descriptor map,
no partition map and no driver, none of which the emulators that patch the
ROM have any use for. A real ROM boots through all three, so what is missing
is made up here and held in memory in front of the file.

The machine sees a disk 96 blocks longer than the file, laid out the way
Apple's formatter writes one. A read of those first blocks is answered from
the header built at attach time and a read past them is passed down to the
file, 96 blocks earlier. The volume is never moved and never rewritten, and
the file is the same bare volume afterwards as it was before, still good for
the emulator it was made for.

Writes to the header stay in memory. The machine has no reason to write
there, having never been told the disk has room to spare, but a driver that
does gets a disk that remembers rather than one that refuses, and the file
keeps out of it either way.
*/

// blockDiskBareVolume is a bare volume dressed as a partitioned disk
type blockDiskBareVolume struct {
	volume BlockDisk
	header []uint8
	name   string
}

/*
newBareVolumeDisk puts a bare volume on the bus behind a made up driver
descriptor map, partition map and driver. The driver is real code and has to
come from somewhere, so it is passed in rather than invented.
*/
func newBareVolumeDisk(volume BlockDisk, driver *Driver) (BlockDisk, error) {
	if driver.Blocks() > driverBlocks {
		return nil, fmt.Errorf("the driver takes %v blocks, more than the %v the "+
			"layout leaves for it", driver.Blocks(), driverBlocks)
	}

	return &blockDiskBareVolume{
		volume: volume,
		header: buildHeader(driver, volume.Blocks()),

		// The name reaches the summary and the errors, and a disk that is
		// 96 blocks longer than the file behind it and boots through a
		// driver from somewhere else should say as much somewhere
		name: fmt.Sprintf("%v (bare volume, driver from %v)",
			volume.Name(), driver.source),
	}, nil
}

func (d *blockDiskBareVolume) Blocks() uint32 {
	return volumeStart + d.volume.Blocks()
}

func (d *blockDiskBareVolume) IsReadOnly() bool {
	return d.volume.IsReadOnly()
}

func (d *blockDiskBareVolume) Name() string {
	return d.name
}

func (d *blockDiskBareVolume) Read(block uint32) ([]uint8, error) {
	if block >= d.Blocks() {
		return nil, fmt.Errorf("the block %v is past the end of %v", block, d.name)
	}

	if block < volumeStart {
		data := make([]uint8, BlockSize)
		copy(data, d.header[block*BlockSize:])
		return data, nil
	}

	return d.volume.Read(block - volumeStart)
}

func (d *blockDiskBareVolume) Write(block uint32, data []uint8) error {
	if block >= d.Blocks() {
		return fmt.Errorf("the block %v is past the end of %v", block, d.name)
	}
	if len(data) != BlockSize {
		return fmt.Errorf("a block is %v bytes, not %v", BlockSize, len(data))
	}

	// The made up blocks are nobody's to keep, so a write to them goes no
	// further than the memory they were built in
	if block < volumeStart {
		if d.IsReadOnly() {
			return fmt.Errorf("%v is read only", d.name)
		}
		copy(d.header[block*BlockSize:], data)
		return nil
	}

	return d.volume.Write(block-volumeStart, data)
}

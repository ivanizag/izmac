// Package storage holds the disk images izmac can attach to the machine.
package storage

import (
	"fmt"
	"os"
)

// BlockSize is the size of a SCSI block on the disks izmac attaches
const BlockSize = 512

// BlockDisk is any block device the SCSI target can sit on top of
type BlockDisk interface {
	// Blocks returns the size of the device
	Blocks() uint32
	// IsReadOnly tells if writes are refused
	IsReadOnly() bool
	// Read returns one block
	Read(block uint32) ([]uint8, error)
	// Write stores one block
	Write(block uint32, data []uint8) error
	// Name describes the device, for the traces
	Name() string
}

// blockDiskFile is a block device backed by a file, read in place rather
// than loaded, so that an image of any size costs nothing to attach
type blockDiskFile struct {
	file     *os.File
	name     string
	blocks   uint32
	readOnly bool
}

// NewBlockDiskFile opens a disk image. The image is a plain sequence of
// blocks, which is what a disk copied with dd or made by a Macintosh
// formatter looks like.
func NewBlockDiskFile(filename string, readOnly bool) (BlockDisk, error) {
	flags := os.O_RDWR
	if readOnly {
		flags = os.O_RDONLY
	}

	file, err := os.OpenFile(filename, flags, 0)
	if err != nil {
		if !readOnly {
			// A read only image is still usable, the ROM can boot from it
			return NewBlockDiskFile(filename, true)
		}
		return nil, fmt.Errorf("can not open the disk image: %w", err)
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}

	size := info.Size()
	if size < BlockSize {
		file.Close()
		return nil, fmt.Errorf("the disk image is %v bytes, too small to hold a block", size)
	}

	return &blockDiskFile{
		file:     file,
		name:     filename,
		blocks:   uint32(size / BlockSize),
		readOnly: readOnly,
	}, nil
}

func (d *blockDiskFile) Blocks() uint32 {
	return d.blocks
}

func (d *blockDiskFile) IsReadOnly() bool {
	return d.readOnly
}

func (d *blockDiskFile) Name() string {
	return d.name
}

func (d *blockDiskFile) Read(block uint32) ([]uint8, error) {
	if block >= d.blocks {
		return nil, fmt.Errorf("the block %v is past the end of %v", block, d.name)
	}

	data := make([]uint8, BlockSize)
	_, err := d.file.ReadAt(data, int64(block)*BlockSize)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (d *blockDiskFile) Write(block uint32, data []uint8) error {
	if d.readOnly {
		return fmt.Errorf("%v is read only", d.name)
	}
	if block >= d.blocks {
		return fmt.Errorf("the block %v is past the end of %v", block, d.name)
	}
	if len(data) != BlockSize {
		return fmt.Errorf("a block is %v bytes, not %v", BlockSize, len(data))
	}

	_, err := d.file.WriteAt(data, int64(block)*BlockSize)
	return err
}

// blockDiskMemory is a block device on a slice, for the tests
type blockDiskMemory struct {
	data     []uint8
	readOnly bool
}

// NewBlockDiskMemory builds a block device of the given size in memory
func NewBlockDiskMemory(blocks uint32) BlockDisk {
	return &blockDiskMemory{data: make([]uint8, blocks*BlockSize)}
}

func (d *blockDiskMemory) Blocks() uint32 {
	return uint32(len(d.data) / BlockSize)
}

func (d *blockDiskMemory) IsReadOnly() bool {
	return d.readOnly
}

func (d *blockDiskMemory) Name() string {
	return "memory"
}

func (d *blockDiskMemory) Read(block uint32) ([]uint8, error) {
	if block >= d.Blocks() {
		return nil, fmt.Errorf("the block %v is past the end", block)
	}
	data := make([]uint8, BlockSize)
	copy(data, d.data[block*BlockSize:])
	return data, nil
}

func (d *blockDiskMemory) Write(block uint32, data []uint8) error {
	if d.readOnly {
		return fmt.Errorf("the device is read only")
	}
	if block >= d.Blocks() {
		return fmt.Errorf("the block %v is past the end", block)
	}
	copy(d.data[block*BlockSize:], data)
	return nil
}

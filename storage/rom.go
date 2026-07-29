package storage

import (
	"fmt"
	"os"
)

// RomSize is the size of the Macintosh Plus ROM
const RomSize = 128 * 1024

// Rom is a loaded and identified ROM image
type Rom struct {
	data     []uint8
	checksum uint32
	version  RomVersion
}

// RomVersion describes one of the known ROM revisions
type RomVersion struct {
	Checksum uint32
	Name     string
	Nickname string
	Notes    string
}

/*
The three revisions of the Macintosh Plus ROM. They differ in the SCSI driver,
which is why the revision matters to an emulator with an emulated SCSI target
and not only to a collector. See doc/plan.md.
*/
func PlusRomVersions() []RomVersion {
	return []RomVersion{
		{Checksum: 0x4d1eeee1, Name: "v1", Nickname: "Lonely Hearts",
			Notes: "will not boot if an external SCSI drive is powered off"},
		{Checksum: 0x4d1eeae1, Name: "v2", Nickname: "Lonely Heifers",
			Notes: "does not expect a Unit Attention on power up or reset"},
		{Checksum: 0x4d1f8172, Name: "v3", Nickname: "Loud Harmonicas",
			Notes: ""},
	}
}

// loadRom reads a ROM image and identifies it by its checksum
func LoadRom(filename string) (*Rom, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("can not read the ROM file: %w", err)
	}

	return parseRom(data)
}

// parseRom identifies a ROM image by its checksum. The first long of a
// Macintosh ROM is the checksum of the rest of it.
func parseRom(data []uint8) (*Rom, error) {
	if len(data) != RomSize {
		return nil, fmt.Errorf("the ROM file is %v bytes, a Macintosh Plus ROM is %v",
			len(data), RomSize)
	}

	stored := uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])
	computed := romChecksum(data)
	if stored != computed {
		return nil, fmt.Errorf("the ROM file is corrupt, it declares the checksum 0x%08x but has 0x%08x",
			stored, computed)
	}

	r := &Rom{data: data, checksum: stored}
	for _, v := range PlusRomVersions() {
		if v.Checksum == stored {
			r.version = v
			return r, nil
		}
	}

	return nil, fmt.Errorf("0x%08x is not a known Macintosh Plus ROM, izmac needs one of the three revisions",
		stored)
}

// romChecksum adds up the words of the image after the stored checksum,
// which is how the ROM checks itself on the power on tests.
func romChecksum(data []uint8) uint32 {
	var sum uint32
	for i := 4; i < len(data); i += 2 {
		sum += uint32(data[i])<<8 | uint32(data[i+1])
	}
	return sum
}

/*
RomFromData wraps bytes that are already in hand as an image, without the
checking LoadRom does. It is for a caller that made them itself.
*/
func RomFromData(data []uint8) *Rom {
	return &Rom{data: data}
}

// Data returns the image itself
func (r *Rom) Data() []uint8 {
	return r.data
}

// Checksum returns the value the image declares and was checked against
func (r *Rom) Checksum() uint32 {
	return r.checksum
}

// Version returns the revision the checksum identified
func (r *Rom) Version() RomVersion {
	return r.version
}

func (r *Rom) String() string {
	return fmt.Sprintf("Macintosh Plus ROM %v '%v', checksum 0x%08x",
		r.version.Name, r.version.Nickname, r.checksum)
}

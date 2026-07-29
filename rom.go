package izmac

import (
	"fmt"
	"os"
)

const (
	// romSize is the size of the Macintosh Plus ROM
	romSize = 128 * 1024

	// romBase is where the ROM lives on the normal address map
	romBase = 0x400000
)

// rom is a loaded and identified ROM image
type rom struct {
	data     []uint8
	checksum uint32
	version  romVersion
}

// romVersion describes one of the known ROM revisions
type romVersion struct {
	checksum uint32
	name     string
	nickname string
	notes    string
}

/*
The three revisions of the Macintosh Plus ROM. They differ in the SCSI driver,
which is why the revision matters to an emulator with an emulated SCSI target
and not only to a collector. See doc/plan.md.
*/
func plusRomVersions() []romVersion {
	return []romVersion{
		{checksum: 0x4d1eeee1, name: "v1", nickname: "Lonely Hearts",
			notes: "will not boot if an external SCSI drive is powered off"},
		{checksum: 0x4d1eeae1, name: "v2", nickname: "Lonely Heifers",
			notes: "does not expect a Unit Attention on power up or reset"},
		{checksum: 0x4d1f8172, name: "v3", nickname: "Loud Harmonicas",
			notes: ""},
	}
}

// preferredRomChecksum is the revision izmac targets. The others are accepted
// with a warning.
const preferredRomChecksum = 0x4d1f8172

// loadRom reads a ROM image and identifies it by its checksum
func loadRom(filename string) (*rom, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("can not read the ROM file: %w", err)
	}

	return parseRom(data)
}

// parseRom identifies a ROM image by its checksum. The first long of a
// Macintosh ROM is the checksum of the rest of it.
func parseRom(data []uint8) (*rom, error) {
	if len(data) != romSize {
		return nil, fmt.Errorf("the ROM file is %v bytes, a Macintosh Plus ROM is %v",
			len(data), romSize)
	}

	stored := uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])
	computed := romChecksum(data)
	if stored != computed {
		return nil, fmt.Errorf("the ROM file is corrupt, it declares the checksum 0x%08x but has 0x%08x",
			stored, computed)
	}

	r := &rom{data: data, checksum: stored}
	for _, v := range plusRomVersions() {
		if v.checksum == stored {
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

// isPreferred returns true for the revision izmac targets
func (r *rom) isPreferred() bool {
	return r.checksum == preferredRomChecksum
}

func (r *rom) String() string {
	return fmt.Sprintf("Macintosh Plus ROM %v '%v', checksum 0x%08x",
		r.version.name, r.version.nickname, r.checksum)
}

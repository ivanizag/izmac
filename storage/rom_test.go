package storage

import (
	"os"
	"path/filepath"
	"testing"
)

// buildTestRom returns a ROM image of the right size whose stored checksum
// matches its contents. The words after the checksum are filled until they
// add up to the value wanted, which lets the tests build an image that
// identifies as any of the known revisions without shipping a copyrighted
// ROM.
func buildTestRom(checksum uint32) []uint8 {
	data := make([]uint8, RomSize)

	data[0] = uint8(checksum >> 24)
	data[1] = uint8(checksum >> 16)
	data[2] = uint8(checksum >> 8)
	data[3] = uint8(checksum)

	remaining := checksum
	for i := 4; i+1 < len(data) && remaining != 0; i += 2 {
		word := remaining
		if word > 0xffff {
			word = 0xffff
		}
		data[i] = uint8(word >> 8)
		data[i+1] = uint8(word)
		remaining -= word
	}

	return data
}

func writeTestRom(t *testing.T, data []uint8) string {
	t.Helper()

	filename := filepath.Join(t.TempDir(), "rom.bin")
	err := os.WriteFile(filename, data, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	return filename
}

func TestBuildTestRomChecksums(t *testing.T) {
	for _, v := range PlusRomVersions() {
		data := buildTestRom(v.Checksum)
		computed := romChecksum(data)
		if computed != v.Checksum {
			t.Errorf("%v: built a ROM with the checksum 0x%08x, wanted 0x%08x",
				v.Nickname, computed, v.Checksum)
		}
	}
}

func TestLoadRomIdentifiesTheRevisions(t *testing.T) {
	for _, v := range PlusRomVersions() {
		filename := writeTestRom(t, buildTestRom(v.Checksum))

		r, err := LoadRom(filename)
		if err != nil {
			t.Fatalf("%v: %v", v.Nickname, err)
		}
		if r.Version().Nickname != v.Nickname {
			t.Errorf("identified 0x%08x as '%v', wanted '%v'",
				v.Checksum, r.Version().Nickname, v.Nickname)
		}
	}
}

func TestLoadRomRejectsAnUnknownChecksum(t *testing.T) {
	filename := writeTestRom(t, buildTestRom(0x01020304))

	_, err := LoadRom(filename)
	if err == nil {
		t.Error("an unknown ROM revision was accepted")
	}
}

func TestLoadRomRejectsACorruptImage(t *testing.T) {
	data := buildTestRom(PlusRomVersions()[2].Checksum)
	data[100] ^= 0xff

	_, err := LoadRom(writeTestRom(t, data))
	if err == nil {
		t.Error("a ROM not matching its own checksum was accepted")
	}
}

func TestLoadRomRejectsTheWrongSize(t *testing.T) {
	_, err := LoadRom(writeTestRom(t, make([]uint8, 64*1024)))
	if err == nil {
		t.Error("a ROM of the wrong size was accepted")
	}
}

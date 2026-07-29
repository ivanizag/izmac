package izmac

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
	data := make([]uint8, romSize)

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
	for _, v := range plusRomVersions() {
		data := buildTestRom(v.checksum)
		computed := romChecksum(data)
		if computed != v.checksum {
			t.Errorf("%v: built a ROM with the checksum 0x%08x, wanted 0x%08x",
				v.nickname, computed, v.checksum)
		}
	}
}

func TestLoadRomIdentifiesTheRevisions(t *testing.T) {
	for _, v := range plusRomVersions() {
		filename := writeTestRom(t, buildTestRom(v.checksum))

		r, err := loadRom(filename)
		if err != nil {
			t.Fatalf("%v: %v", v.nickname, err)
		}
		if r.version.nickname != v.nickname {
			t.Errorf("identified 0x%08x as '%v', wanted '%v'",
				v.checksum, r.version.nickname, v.nickname)
		}
		if r.isPreferred() != (v.checksum == preferredRomChecksum) {
			t.Errorf("%v: unexpected preferred revision report", v.nickname)
		}
	}
}

func TestLoadRomRejectsAnUnknownChecksum(t *testing.T) {
	filename := writeTestRom(t, buildTestRom(0x01020304))

	_, err := loadRom(filename)
	if err == nil {
		t.Error("an unknown ROM revision was accepted")
	}
}

func TestLoadRomRejectsACorruptImage(t *testing.T) {
	data := buildTestRom(preferredRomChecksum)
	data[100] ^= 0xff

	_, err := loadRom(writeTestRom(t, data))
	if err == nil {
		t.Error("a ROM not matching its own checksum was accepted")
	}
}

func TestLoadRomRejectsTheWrongSize(t *testing.T) {
	_, err := loadRom(writeTestRom(t, make([]uint8, 64*1024)))
	if err == nil {
		t.Error("a ROM of the wrong size was accepted")
	}
}

package izmac

import (
	"os"
	"testing"
)

func BenchmarkBoot(b *testing.B) {
	const (
		diskFile = "frontend/macebiten/HD20SC.vhd"
		romFile  = defaultRomFile
	)
	for _, name := range []string{diskFile, romFile} {
		if _, err := os.Stat(name); err != nil {
			b.Skipf("%v is not here", name)
		}
	}

	for b.Loop() {
		config := NewConfiguration()
		config.RomFile = romFile
		config.HardDisks = []string{diskFile}
		if err := config.Validate(); err != nil {
			b.Fatal(err)
		}
		m, err := NewMac(config)
		if err != nil {
			b.Fatal(err)
		}
		m.RunFrames(1200)
	}
}

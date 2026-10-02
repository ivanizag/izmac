package izmac

import (
	"testing"
)

// BenchmarkBoot is the first twenty emulated seconds of booting the System 6
// test disk
func BenchmarkBoot(b *testing.B) {
	disk := testImage(b, testSystemSixDisk)
	for b.Loop() {
		config := testConfig(b)
		config.DiskFiles = []string{disk}
		m := buildTestMac(b, config)
		m.RunFrames(1200)
	}
}

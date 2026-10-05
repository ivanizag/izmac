package e2e_tests

import (
	"bytes"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
	"time"
)

/*
Two runs of a machine from the same start time, on the same images and with
the same input, are the same to the byte: its memory and its screen at the
end. The run goes through what depends on time the most, the file server of
-share: the folder mounted, a document of it opened in TeachText, a word typed
and saved over the network. A run that comes out different has something in
it going by the time of the host.
*/
func TestTwoRunsFromTheSameStartAreTheSame(t *testing.T) {
	t.Parallel()
	start := time.Date(1987, 3, 2, 10, 0, 0, 0, time.UTC)

	run := func() ([32]uint8, []uint8) {
		// The folder has the same name, which is the volume's, and the same
		// dates, in both runs
		share := filepath.Join(t.TempDir(), "Shared")
		os.Mkdir(share, 0o755)
		readme := filepath.Join(share, "readme.txt")
		if err := os.WriteFile(readme, []uint8("hello from the host\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		os.Mkdir(filepath.Join(share, "Folder"), 0o755)
		before := start.Add(-time.Hour)
		for _, p := range []string{readme, filepath.Join(share, "Folder"), share} {
			os.Chtimes(p, before, before)
		}

		config := testConfig(t)
		config.Diskettes = []string{testImage(t, testSystemSixDiskette)}
		config.RamSizeKb = 4096
		config.Share = share
		config.ShareName = "izmac"
		config.PrinterPort = ""
		config.StartTime = start
		m := buildTestMac(t, config)
		m.RunFrames(3000)

		openSharedFolder(t, m)
		doubleClickAt(t, m, 178, 125)
		waitForApplication(t, m, "TeachText", 20)
		m.RunFrames(600)
		typeText(m, "edited ")
		pressCommand(m, "S")
		m.RunFrames(600)
		pressCommand(m, "Q")
		waitForApplication(t, m, "Finder", 60)
		m.RunFrames(600)
		if data, _ := os.ReadFile(readme); !bytes.HasPrefix(data, []uint8("edited ")) {
			t.Fatalf("the document was not saved to the host, which has %q", data)
		}

		ram := make([]uint8, config.RamSizeKb*1024)
		for i := range ram {
			ram[i] = m.Peek(uint32(i))
		}
		return sha256.Sum256(ram), m.Screenshot().Pix
	}

	ramOne, screenOne := run()
	ramTwo, screenTwo := run()
	if ramOne != ramTwo {
		t.Errorf("the memory of the two runs is not the same")
	}
	if !bytes.Equal(screenOne, screenTwo) {
		t.Errorf("the screen of the two runs is not the same")
	}
}

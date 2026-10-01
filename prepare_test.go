package izmac

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ivanizag/izmac/storage"
)

// diskette is an 800Kb diskette image told from the others by its first byte
func diskette(mark uint8) []uint8 {
	data := make([]uint8, 800*1024)
	data[0] = mark
	return data
}

// writeZip makes a zip on a temporary directory, the files in the order given
func writeZip(t *testing.T, name string, names []string, contents [][]uint8) string {
	t.Helper()

	var buffer bytes.Buffer
	w := zip.NewWriter(&buffer)
	for i, entry := range names {
		f, err := w.Create(entry)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(contents[i]); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	filename := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(filename, buffer.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return filename
}

// quietConfiguration is a configuration that reports into a buffer
func quietConfiguration() (*Configuration, *strings.Builder) {
	c := NewConfiguration()
	out := &strings.Builder{}
	c.messages = out
	return c, out
}

func TestTheDiskettesInAnArchiveFillTheDrives(t *testing.T) {
	archive := writeZip(t, "set.zip",
		[]string{"Set/One.dsk", "Set/Read Me", "Set/Two.dsk", "Set/Three.dsk"},
		[][]uint8{diskette('1'), []uint8("Install from One."), diskette('2'), diskette('3')})

	c, out := quietConfiguration()
	if err := c.AddFiles([]string{archive}); err != nil {
		t.Fatal(err)
	}

	wanted := []string{archive + "/One.dsk", archive + "/Two.dsk"}
	if len(c.Diskettes) != 2 || c.Diskettes[0] != wanted[0] || c.Diskettes[1] != wanted[1] {
		t.Fatalf("the diskettes are %v, wanted %v", c.Diskettes, wanted)
	}

	report := out.String()
	if !strings.Contains(report, "Three.dsk, left out") {
		t.Errorf("the third diskette was not said to be left out:\n%v", report)
	}
	if !strings.Contains(report, "Read Me") {
		t.Errorf("the file that is not a disk image was not mentioned:\n%v", report)
	}

	d, err := c.openDiskette(c.Diskettes[1])
	if err != nil {
		t.Fatal(err)
	}
	if d.IsReadOnly() {
		t.Errorf("a diskette out of an archive is locked")
	}

	// Nothing was written anywhere
	if entries, _ := os.ReadDir(filepath.Dir(archive)); len(entries) != 1 {
		t.Errorf("unpacking left %v files next to the archive, wanted it alone", len(entries))
	}
}

func TestAnArchiveNamedWithAFlagIsUnpackedWhereItWasPut(t *testing.T) {
	hard := make([]uint8, 4<<20)
	hard[0], hard[1] = 'E', 'R'
	archive := writeZip(t, "hard.zip", []string{"Hard.img"}, [][]uint8{hard})

	c, _ := quietConfiguration()
	if err := c.ParseFlags("izmac", []string{"-rom", "rom.bin", "-hd", archive}, os.Stderr); err != nil {
		t.Fatal(err)
	}

	if len(c.DiskFiles) != 1 || c.DiskFiles[0] != archive+"/Hard.img" {
		t.Fatalf("the disks are %v, wanted the one in the archive", c.DiskFiles)
	}

	// Validating again leaves the lists as they are
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(c.DiskFiles) != 1 || c.DiskFiles[0] != archive+"/Hard.img" {
		t.Errorf("validating twice changed the disks to %v", c.DiskFiles)
	}

	disk, err := c.openDisk(c.DiskFiles[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	if disk.IsReadOnly() || disk.Blocks() != 8192 {
		t.Errorf("the disk is %v blocks, read only %v", disk.Blocks(), disk.IsReadOnly())
	}
}

func TestAnArchiveWithNoDiskImageIsRefused(t *testing.T) {
	archive := writeZip(t, "app.zip", []string{"App", "Read Me"},
		[][]uint8{make([]uint8, 5000), []uint8("Double click App.")})

	c, _ := quietConfiguration()
	err := c.AddFiles([]string{archive})
	if err == nil || !strings.Contains(err.Error(), "no disk image") {
		t.Errorf("an archive with no disk image in it gave %v", err)
	}
}

func TestAPaddedDisketteIsMendedInMemory(t *testing.T) {
	// An HFS volume of 400Kb, in a file a kilobyte longer
	data := make([]uint8, 401*1024)
	mdb := data[1024:]
	mdb[0], mdb[1] = 0x42, 0x44       // 'BD'
	mdb[0x12], mdb[0x13] = 0x03, 0x1a // 794 allocation blocks
	mdb[0x16] = 0x02                  // of 512 bytes
	mdb[0x1d] = 0x04                  // starting at block 4

	filename := filepath.Join(t.TempDir(), "padded.dsk")
	if err := os.WriteFile(filename, data, 0o600); err != nil {
		t.Fatal(err)
	}

	c, _ := quietConfiguration()
	if err := c.AddFiles([]string{filename}); err != nil {
		t.Fatal(err)
	}

	if len(c.Diskettes) != 1 || c.Diskettes[0] != filename {
		t.Fatalf("the diskettes are %v, wanted the padded one", c.Diskettes)
	}
	if len(c.memoryImages[filename]) != 400*1024 {
		t.Errorf("the diskette held is %v bytes, wanted 400Kb", len(c.memoryImages[filename]))
	}

	if info, _ := os.Stat(filename); info.Size() != 401*1024 {
		t.Errorf("the file was changed to %v bytes", info.Size())
	}
}

func TestADiskImageIsStillUsedInPlace(t *testing.T) {
	floppy := writeImage(t, "floppy.dsk", 800*1024, false)

	c, out := quietConfiguration()
	if err := c.AddFiles([]string{floppy}); err != nil {
		t.Fatal(err)
	}

	if len(c.Diskettes) != 1 || c.Diskettes[0] != floppy || len(c.memoryImages) != 0 {
		t.Errorf("a plain diskette was not taken as the file it is")
	}
	if out.String() != "" {
		t.Errorf("a plain diskette was reported: %q", out.String())
	}
}

func TestTheImagesKeptWithPersistAreUsedAgain(t *testing.T) {
	archive := writeZip(t, "Game.dsk.zip", []string{"Game.dsk"}, [][]uint8{diskette('G')})
	t.Chdir(t.TempDir())

	c, out := quietConfiguration()
	c.Persist = true
	if err := c.AddFiles([]string{archive}); err != nil {
		t.Fatal(err)
	}

	const kept = "izmac_Game.dsk"
	if len(c.Diskettes) != 1 || c.Diskettes[0] != kept || len(c.memoryImages) != 0 {
		t.Fatalf("the diskettes are %v, wanted %v on the working directory", c.Diskettes, kept)
	}
	if !strings.Contains(out.String(), "saved as "+kept) {
		t.Errorf("saving the diskette was not reported:\n%v", out.String())
	}

	// The machine saves something on it, and the next run finds it there
	saved := diskette('S')
	if err := os.WriteFile(kept, saved, 0o600); err != nil {
		t.Fatal(err)
	}

	again, out := quietConfiguration()
	again.Persist = true
	if err := again.AddFiles([]string{archive}); err != nil {
		t.Fatal(err)
	}

	if len(again.Diskettes) != 1 || again.Diskettes[0] != kept {
		t.Fatalf("the second run has the diskettes %v", again.Diskettes)
	}
	if data, _ := os.ReadFile(kept); !bytes.Equal(data, saved) {
		t.Errorf("the kept diskette was unpacked over")
	}
	if !strings.Contains(out.String(), "kept from an earlier run") {
		t.Errorf("finding the diskette again was not reported:\n%v", out.String())
	}

	d, err := again.openDiskette(kept)
	if err != nil {
		t.Fatal(err)
	}
	if d.IsReadOnly() {
		t.Errorf("a kept diskette is locked")
	}
}

func TestSeveralKeptImagesAreNamedAfterTheArchiveAndThemselves(t *testing.T) {
	archive := writeZip(t, "System 6.0.8.7z.zip",
		[]string{"System Tools.img", "Utilities 1.img"},
		[][]uint8{diskette('S'), diskette('U')})
	t.Chdir(t.TempDir())

	c, _ := quietConfiguration()
	c.Persist = true
	if err := c.AddFiles([]string{archive}); err != nil {
		t.Fatal(err)
	}

	wanted := []string{
		"izmac_System 6.0.8 - System Tools.dsk",
		"izmac_System 6.0.8 - Utilities 1.dsk",
	}
	for i, name := range wanted {
		if len(c.Diskettes) <= i || c.Diskettes[i] != name {
			t.Errorf("the diskettes are %v, wanted %v", c.Diskettes, wanted)
			break
		}
		if _, err := os.Stat(name); err != nil {
			t.Errorf("%v was not written: %v", name, err)
		}
	}
}

func TestADisketteDroppedInAnArchiveGoesInWritable(t *testing.T) {
	archive := writeZip(t, "two.zip", []string{"One.dsk", "Two.dsk"},
		[][]uint8{diskette('1'), diskette('2')})

	config, out := quietConfiguration()
	config.RomFile = "<test>"
	m := ensureNewMac(t, config, storage.RomFromData(make([]uint8, storage.RomSize)), nil, nil)

	if err := m.InsertDiskette(DriveInternal, archive); err != nil {
		t.Fatal(err)
	}

	drive := m.GetDiskette(DriveInternal)
	if drive.Image != archive+"/One.dsk" || drive.ReadOnly {
		t.Errorf("the drive holds %q, locked %v, wanted the first diskette unlocked",
			drive.Image, drive.ReadOnly)
	}
	if !strings.Contains(out.String(), "Two.dsk, left out") {
		t.Errorf("the second diskette was not said to be left out:\n%v", out.String())
	}
	if len(config.memoryImages) != 0 {
		t.Errorf("a diskette dropped on the machine was kept in the configuration")
	}
}

func TestAHardDiskDroppedOnTheWindowIsTurnedAway(t *testing.T) {
	hard := writeImage(t, "hard.img", 4<<20, true)

	config, _ := quietConfiguration()
	config.RomFile = "<test>"
	m := ensureNewMac(t, config, storage.RomFromData(make([]uint8, storage.RomSize)), nil, nil)

	err := m.InsertDiskette(DriveInternal, hard)
	if err == nil || !strings.Contains(err.Error(), "holds no diskette") {
		t.Errorf("a hard disk put in a drive gave %v", err)
	}
}

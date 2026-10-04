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

// isVolumeWith tells an image is an HFS volume with a file of that name on it
func isVolumeWith(image []uint8, name string) bool {
	return len(image) > 1026 && string(image[1024:1026]) == "BD" &&
		bytes.Contains(image, []uint8(name))
}

func TestTheFilesOfAnArchiveGoOnANewVolume(t *testing.T) {
	archive := writeZip(t, "Game.zip", []string{"Game/Game", "Game/Read Me"},
		[][]uint8{make([]uint8, 5000), []uint8("Double click Game.")})

	c, out := quietConfiguration()
	if err := c.AddFiles([]string{archive}); err != nil {
		t.Fatal(err)
	}

	if len(c.Diskettes) != 1 || c.Diskettes[0] != archive+"/Game" {
		t.Fatalf("the diskettes are %v, wanted a new volume called Game", c.Diskettes)
	}
	image := c.memoryImages[c.Diskettes[0]]
	if len(image) != 800*1024 || !isVolumeWith(image, "Read Me") {
		t.Errorf("the new volume is %v bytes and not an HFS diskette with Read Me on it", len(image))
	}
	if !strings.Contains(out.String(), "Packing 2 files on a new volume") {
		t.Errorf("packing the files was not reported:\n%v", out.String())
	}
}

func TestTheMacintoshFilesBesideADiskImageGoOnAVolume(t *testing.T) {
	// The ._ file of an application with no data fork, as macOS zips one
	appleDouble := make([]uint8, 26+12+32)
	copy(appleDouble, "\x00\x05\x16\x07\x00\x02\x00\x00")
	appleDouble[25] = 1                          // one entry
	appleDouble[29] = 9                          // the Finder information
	appleDouble[33], appleDouble[37] = 26+12, 32 // where and how long
	copy(appleDouble[26+12:], "APPLGAME")

	archive := writeZip(t, "Set.zip",
		[]string{"One.dsk", "checksums.txt", "__MACOSX/._Utility"},
		[][]uint8{diskette('1'), []uint8("1234 One.dsk"), appleDouble})

	c, out := quietConfiguration()
	if err := c.AddFiles([]string{archive}); err != nil {
		t.Fatal(err)
	}

	wanted := []string{archive + "/One.dsk", archive + "/Set Files"}
	if len(c.Diskettes) != 2 || c.Diskettes[0] != wanted[0] || c.Diskettes[1] != wanted[1] {
		t.Fatalf("the diskettes are %v, wanted %v", c.Diskettes, wanted)
	}
	if !isVolumeWith(c.memoryImages[wanted[1]], "Utility") {
		t.Errorf("the application is not on the new volume")
	}
	if !strings.Contains(out.String(), "left out, not disk images: checksums.txt") {
		t.Errorf("the file for the host was not left out:\n%v", out.String())
	}
}

/*
A DiskCopy image as a Macintosh keeps it: its type says what it is, and it has
a resource fork with the checksums of the copy, which an application or a
document would have too. ResEdit's diskette comes in a StuffIt archive like
this.
*/
func TestADiskCopyImageWithAResourceForkIsADiskImage(t *testing.T) {
	image := make([]uint8, 84+800*1024)
	copy(image, "\x07ResEdit")
	image[64+1], image[64+2] = 0x0c, 0x80 // 800K of sectors
	image[80] = 1                         // an 800K diskette
	image[82] = 1                         // $0100

	// The ._ file with the Finder information and a resource fork of four
	// bytes, as macOS zips them
	const entries = 2
	header := 26 + 12*entries
	appleDouble := make([]uint8, header+32+4)
	copy(appleDouble, "\x00\x05\x16\x07\x00\x02\x00\x00")
	appleDouble[25] = entries
	appleDouble[29], appleDouble[33], appleDouble[37] = 9, uint8(header), 32
	appleDouble[29+12], appleDouble[33+12], appleDouble[37+12] = 2, uint8(header+32), 4
	copy(appleDouble[header:], "dImgdCpy")
	copy(appleDouble[header+32:], "ckid")

	archive := writeZip(t, "ResEdit.zip",
		[]string{"ResEdit.img", "__MACOSX/._ResEdit.img"}, [][]uint8{image, appleDouble})

	c, _ := quietConfiguration()
	if err := c.AddFiles([]string{archive}); err != nil {
		t.Fatal(err)
	}
	if len(c.Diskettes) != 1 || c.Diskettes[0] != archive+"/ResEdit.img" {
		t.Errorf("the diskettes are %v, wanted the image itself", c.Diskettes)
	}
}

func TestAFolderGoesOnAVolume(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Documents")
	if err := os.MkdirAll(filepath.Join(dir, "Letters"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Letters", "To Ivan"), []uint8("Hi."), 0o600); err != nil {
		t.Fatal(err)
	}

	c, _ := quietConfiguration()
	if err := c.AddFiles([]string{dir}); err != nil {
		t.Fatal(err)
	}

	if len(c.Diskettes) != 1 || c.Diskettes[0] != dir {
		t.Fatalf("the diskettes are %v, wanted the folder", c.Diskettes)
	}
	if !isVolumeWith(c.memoryImages[dir], "To Ivan") {
		t.Errorf("the folder did not go on a volume")
	}
}

func TestAFolderTooBigForADisketteGoesOnTheBus(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Big"), make([]uint8, 900*1024), 0o600); err != nil {
		t.Fatal(err)
	}

	c, _ := quietConfiguration()
	if err := c.AddFiles([]string{dir}); err != nil {
		t.Fatal(err)
	}

	if len(c.DiskFiles) != 1 || c.DiskFiles[0] != dir || len(c.Diskettes) != 0 {
		t.Fatalf("the disks are %v and the diskettes %v, wanted the folder on the bus",
			c.DiskFiles, c.Diskettes)
	}
	if kind := storage.ClassifyData(c.memoryImages[dir]); kind != storage.KindBareVolume {
		t.Errorf("the volume is a %v, wanted a bare volume for the SCSI driver to be "+
			"made up in front of", kind)
	}
	if needed, _ := c.needsScsiDriver(); !needed {
		t.Errorf("a volume made in memory does not ask for a SCSI driver")
	}
}

func TestAFileThatIsNotADiskImageGoesOnAVolume(t *testing.T) {
	text := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(text, []uint8("Things to do."), 0o600); err != nil {
		t.Fatal(err)
	}

	c, _ := quietConfiguration()
	if err := c.AddFiles([]string{text}); err != nil {
		t.Fatal(err)
	}

	if len(c.Diskettes) != 1 || !isVolumeWith(c.memoryImages[text], "notes.txt") {
		t.Errorf("a text file did not go on a volume of its own")
	}
}

func TestABlankImageIsStillAHardDisk(t *testing.T) {
	blank := writeImage(t, "blank.img", 4<<20, false)

	c, _ := quietConfiguration()
	if err := c.AddFiles([]string{blank}); err != nil {
		t.Fatal(err)
	}

	if len(c.DiskFiles) != 1 || c.DiskFiles[0] != blank || len(c.memoryImages) != 0 {
		t.Errorf("a blank image was not attached as the hard disk to be formatted it is")
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
	if !strings.Contains(out.String(), "Using what was kept of") ||
		strings.Contains(out.String(), "Unpacking") {
		t.Errorf("the second run did not go straight to the kept diskette:\n%v", out.String())
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

// writeFolder makes a folder with one file in it, under a temporary directory
func writeFolder(t *testing.T, name string, file string, data string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, file), []uint8(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// persistent is a configuration that keeps what it makes
func persistent() (*Configuration, *strings.Builder) {
	c, out := quietConfiguration()
	c.Persist = true
	return c, out
}

func TestAKeptFolderIsNotPackedAgain(t *testing.T) {
	folder := writeFolder(t, "Letters", "To Ivan", "Hi.")
	t.Chdir(t.TempDir())

	first, _ := persistent()
	if err := first.AddFiles([]string{folder}); err != nil {
		t.Fatal(err)
	}

	// The folder goes away, and the kept volume is all the next run needs
	if err := os.RemoveAll(folder); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(folder, 0o700); err != nil {
		t.Fatal(err)
	}

	second, out := persistent()
	if err := second.AddFiles([]string{folder}); err != nil {
		t.Fatal(err)
	}
	if len(second.Diskettes) != 1 || second.Diskettes[0] != "izmac_Letters.dsk" {
		t.Errorf("the diskettes are %v, wanted the kept volume", second.Diskettes)
	}
	if strings.Contains(out.String(), "Packing") {
		t.Errorf("the folder was packed again:\n%v", out.String())
	}
}

func TestTwoFoldersOfTheSameNameAreKeptApart(t *testing.T) {
	one := writeFolder(t, "Letters", "One", "one")
	two := writeFolder(t, "Letters", "Two", "two")
	t.Chdir(t.TempDir())

	for round := 0; round < 2; round++ {
		for i, folder := range []string{one, two} {
			c, _ := persistent()
			if err := c.AddFiles([]string{folder}); err != nil {
				t.Fatal(err)
			}

			wanted := []string{"izmac_Letters.dsk", "izmac_Letters 2.dsk"}[i]
			if len(c.Diskettes) != 1 || c.Diskettes[0] != wanted {
				t.Fatalf("round %v: %v is kept as %v, wanted %v",
					round, folder, c.Diskettes, wanted)
			}
		}
	}

	image, err := os.ReadFile("izmac_Letters 2.dsk")
	if err != nil {
		t.Fatal(err)
	}
	if !isVolumeWith(image, "Two") || isVolumeWith(image, "One") {
		t.Errorf("the second folder's volume does not hold the second folder")
	}
}

func TestAnImageKeptBeforeThereWasARecordIsUsed(t *testing.T) {
	folder := writeFolder(t, "Letters", "To Ivan", "Hi.")
	t.Chdir(t.TempDir())

	// What an earlier izmac kept, with no record of where it came from
	saved := diskette('S')
	if err := os.WriteFile("izmac_Letters.dsk", saved, 0o600); err != nil {
		t.Fatal(err)
	}

	c, _ := persistent()
	if err := c.AddFiles([]string{folder}); err != nil {
		t.Fatal(err)
	}

	if len(c.Diskettes) != 1 || c.Diskettes[0] != "izmac_Letters.dsk" {
		t.Errorf("the diskettes are %v, wanted the image kept before", c.Diskettes)
	}
	if data, _ := os.ReadFile("izmac_Letters.dsk"); !bytes.Equal(data, saved) {
		t.Errorf("the image kept before was packed over")
	}
}

func TestAKeptImageThatIsGoneIsMadeAgain(t *testing.T) {
	archive := writeZip(t, "Set.zip", []string{"One.dsk", "Two.dsk"},
		[][]uint8{diskette('1'), diskette('2')})
	t.Chdir(t.TempDir())

	first, _ := persistent()
	if err := first.AddFiles([]string{archive}); err != nil {
		t.Fatal(err)
	}

	// One of them is kept with something saved on it, the other deleted
	saved := diskette('S')
	if err := os.WriteFile("izmac_Set - One.dsk", saved, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove("izmac_Set - Two.dsk"); err != nil {
		t.Fatal(err)
	}

	second, _ := persistent()
	if err := second.AddFiles([]string{archive}); err != nil {
		t.Fatal(err)
	}

	if data, _ := os.ReadFile("izmac_Set - One.dsk"); !bytes.Equal(data, saved) {
		t.Errorf("the image still there was unpacked over")
	}
	if data, err := os.ReadFile("izmac_Set - Two.dsk"); err != nil || data[0] != '2' {
		t.Errorf("the image that was gone was not made again")
	}
	if len(second.Diskettes) != 2 {
		t.Errorf("the diskettes are %v, wanted both", second.Diskettes)
	}
}

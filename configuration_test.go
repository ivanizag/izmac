package izmac

import (
	"github.com/ivanizag/izmac/storage"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/ivanizag/izmac/scsi"
)

func TestTheDefaultRomIsUsedWhenNoneIsNamed(t *testing.T) {
	c := NewConfiguration()
	err := c.ParseFlags("izmac", []string{}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	if c.RomFile != defaultRomFile {
		t.Errorf("the ROM defaults to %v, wanted %v", c.RomFile, defaultRomFile)
	}
	if !c.romIsDefault {
		t.Error("the default ROM was not marked as downloadable")
	}
}

func TestANamedRomIsNeverDownloaded(t *testing.T) {
	c := NewConfiguration()
	err := c.ParseFlags("izmac", []string{"-rom", "mine.rom"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	if c.romIsDefault {
		t.Error("a ROM named on the command line was marked as downloadable")
	}
}

func TestTheDefaults(t *testing.T) {
	c := NewConfiguration()
	err := c.ParseFlags("izmac", []string{"-rom", "rom.bin"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	if c.RamSizeKb != 1024 {
		t.Errorf("the default RAM size is %vKb, wanted 1024Kb", c.RamSizeKb)
	}
	if c.PramFile != "pram.bin" {
		t.Errorf("the parameter RAM defaults to %v, wanted pram.bin on the working directory",
			c.PramFile)
	}
	if !c.Clipboard {
		t.Error("the clipboard is not shared with the host by default")
	}
}

/*
The clipboard is on unless it is asked for off, which for a boolean flag means
the long form: a bare -clipboard would turn on what is already on, so the only
thing left to say is -clipboard=false.
*/
func TestTheClipboardCanBeTurnedOff(t *testing.T) {
	c := NewConfiguration()
	err := c.ParseFlags("izmac", []string{"-rom", "rom.bin", "-clipboard=false"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	if c.Clipboard {
		t.Error("-clipboard=false left the clipboard shared")
	}

	m := ensureNewMac(t, c, storage.RomFromData(make([]uint8, storage.RomSize)), nil, nil)
	if m.HasClipboard() {
		t.Error("the machine shares its clipboard with the clipboard turned off")
	}
	if _, copied := m.TakeCopiedText(); copied {
		t.Error("a machine with no clipboard offered text to the host")
	}

	// And a paste asked for anyway is dropped rather than queued for ever
	m.PasteText("Hello")
	if m.pastePending {
		t.Error("a machine with no clipboard took a paste")
	}
}

func TestTheRamSizeIsChecked(t *testing.T) {
	for _, c := range []struct {
		size     string
		accepted bool
	}{
		{"1024", true},
		{"4096", true},
		{"512", false},
		{"2048", false},
	} {
		config := NewConfiguration()
		err := config.ParseFlags("izmac",
			[]string{"-rom", "rom.bin", "-ram", c.size}, io.Discard)

		if c.accepted && err != nil {
			t.Errorf("the RAM size %vKb was rejected: %v", c.size, err)
		}
		if !c.accepted && err == nil {
			t.Errorf("the RAM size %vKb was accepted", c.size)
		}
	}
}

func TestTheTracersAreListed(t *testing.T) {
	c := NewConfiguration()
	err := c.ParseFlags("izmac",
		[]string{"-rom", "rom.bin", "-trace", "cpu, toolbox"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	if !c.hasTracer("cpu") || !c.hasTracer("toolbox") {
		t.Error("the tracers listed were not recognized")
	}
	if c.hasTracer("sadmac") {
		t.Error("a tracer not listed was recognized")
	}
}

/*
Asking for the help is not a failure. The frontends exit quietly and
successfully on it, which the Homebrew formula relies on when it runs the
binary with -h to check the bottle works.
*/
func TestAskingForTheHelpIsNotAnError(t *testing.T) {
	c := NewConfiguration()
	err := c.ParseFlags("izmac", []string{"-h"}, io.Discard)

	if err == nil {
		t.Fatal("asking for the help parsed as an ordinary run")
	}
	if !IsHelpRequested(err) {
		t.Errorf("asking for the help gave %v, which is not recognized as it", err)
	}

	// And a real mistake still is one
	bad := NewConfiguration().ParseFlags("izmac", []string{"-nosuchflag"}, io.Discard)
	if bad == nil {
		t.Fatal("an unknown flag was accepted")
	}
	if IsHelpRequested(bad) {
		t.Error("an unknown flag was taken for a request for the help")
	}
}

// A Configuration must be buildable more than once in the same process, which
// is why the flags do not go on the package level FlagSet
func TestTheConfigurationCanBeBuiltTwice(t *testing.T) {
	for i := 0; i < 2; i++ {
		c := NewConfiguration()
		err := c.ParseFlags("izmac", []string{"-rom", "rom.bin"}, io.Discard)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestTheSpeedOption(t *testing.T) {
	native := 1000.0 / CPUClockMhz

	for _, c := range []struct {
		speed    string
		duration float64
		accepted bool
	}{
		{"", native, true},       // The default is the real machine
		{"plus", native, true},   // And so is naming it
		{"full", 0, true},        // Zero means no throttling at all
		{"1", 1000.0, true},      // A slow motion Macintosh
		{"15.6672", 63.83, true}, // Or twice the real speed
		{"0", 0, false},          // A stopped clock is not a speed
		{"-2", 0, false},         //
		{"quick", 0, false},      //
	} {
		config := NewConfiguration()
		args := []string{"-rom", "rom.bin"}
		if c.speed != "" {
			args = append(args, "-speed", c.speed)
		}

		err := config.ParseFlags("izmac", args, io.Discard)

		if !c.accepted {
			if err == nil {
				t.Errorf("the speed %q was accepted", c.speed)
			}
			continue
		}

		if err != nil {
			t.Errorf("the speed %q was rejected: %v", c.speed, err)
			continue
		}
		if diff := config.cycleDurationNs - c.duration; diff > 0.01 || diff < -0.01 {
			t.Errorf("the speed %q gives %.2fns a cycle, wanted %.2f",
				c.speed, config.cycleDurationNs, c.duration)
		}
	}
}

func TestFullSpeedReachesTheMachine(t *testing.T) {
	config := NewConfiguration()
	err := config.ParseFlags("izmac",
		[]string{"-rom", "rom.bin", "-speed", "full"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	m := ensureNewMac(t, config, storage.RomFromData(make([]uint8, storage.RomSize)), nil, nil)
	if !m.IsFullSpeed() {
		t.Error("the machine is throttled with the full speed option")
	}

	// And the reported clock falls back to the real one, there is no
	// meaningful number to give until it has run
	if m.clockMhz() != CPUClockMhz {
		t.Errorf("the clock reads %v at full speed, wanted the real %v",
			m.clockMhz(), CPUClockMhz)
	}
}

func TestTheSpeedCanBeToggled(t *testing.T) {
	config := NewConfiguration()
	err := config.ParseFlags("izmac", []string{"-rom", "rom.bin"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	m := ensureNewMac(t, config, storage.RomFromData(make([]uint8, storage.RomSize)), nil, nil)
	if m.IsFullSpeed() {
		t.Fatal("the machine starts unthrottled by default")
	}

	m.toggleSpeed()
	if !m.IsFullSpeed() {
		t.Error("the toggle did not go to full speed")
	}

	m.toggleSpeed()
	if m.IsFullSpeed() {
		t.Error("the toggle did not come back to the speed configured")
	}
	if m.clockMhz() != CPUClockMhz {
		t.Errorf("the speed came back as %v, wanted %v", m.clockMhz(), CPUClockMhz)
	}
}

// Toggling from a machine already configured for full speed has to give the
// real one, otherwise the toggle would do nothing
func TestTogglingFromFullSpeedGivesTheRealOne(t *testing.T) {
	config := NewConfiguration()
	err := config.ParseFlags("izmac",
		[]string{"-rom", "rom.bin", "-speed", "full"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	m := ensureNewMac(t, config, storage.RomFromData(make([]uint8, storage.RomSize)), nil, nil)
	m.toggleSpeed()

	if m.IsFullSpeed() {
		t.Fatal("the toggle left the machine at full speed")
	}
	if m.clockMhz() != CPUClockMhz {
		t.Errorf("the toggle gave %v Mhz, wanted the real %v", m.clockMhz(), CPUClockMhz)
	}
}

// writeImage makes a file of the given size, optionally starting with the
// driver descriptor map a partitioned Macintosh disk carries
func writeImage(t *testing.T, name string, size int, partitioned bool) string {
	t.Helper()

	data := make([]uint8, size)
	if partitioned {
		data[0], data[1] = 0x45, 0x52 // 'ER'
		data[2], data[3] = 0x02, 0x00 // 512 byte blocks
	}

	filename := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(filename, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return filename
}

// writeVolumeImage makes a bare HFS volume, too big for any drive so that
// only the missing map tells it from a diskette
func writeVolumeImage(t *testing.T, name string) string {
	t.Helper()

	data := make([]uint8, 4*1024*1024)
	data[0], data[1] = 'L', 'K'                                         // the boot blocks
	data[2*storage.BlockSize], data[2*storage.BlockSize+1] = 0x42, 0x44 // 'BD'

	filename := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(filename, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return filename
}

/*
A bare volume is attached behind a borrowed driver, so it is accepted however
it was named. Naming it with -hd and naming it on its own have to behave
alike, which is why the disks are looked at rather than the way they got
there.
*/
func TestABareVolumeAsksForADriver(t *testing.T) {
	volume := writeVolumeImage(t, "volume.dsk")

	for _, args := range [][]string{
		{"-rom", "rom.bin", volume},
		{"-rom", "rom.bin", "-hd", volume},
	} {
		c := NewConfiguration()
		err := c.ParseFlags("izmac", args, io.Discard)
		if err != nil {
			t.Fatalf("a bare volume was refused with %v: %v", args, err)
		}

		wanted, err := c.needsDriver()
		if err != nil {
			t.Fatal(err)
		}
		if !wanted {
			t.Errorf("%v left the bare volume without asking for a driver", args)
		}
	}
}

// A driver is only wanted for a bare volume. A disk that carries its own is
// not a reason to go looking, still less to download one.
func TestAPartitionedDiskAsksForNoDriver(t *testing.T) {
	c := NewConfiguration()
	err := c.ParseFlags("izmac",
		[]string{"-rom", "rom.bin", writeImage(t, "hard.img", 4*1024*1024, true)}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	wanted, err := c.needsDriver()
	if err != nil {
		t.Fatal(err)
	}
	if wanted {
		t.Error("a partitioned disk asked for a driver")
	}
}

func TestTheDefaultDriverIsUsedWhenNoneIsNamed(t *testing.T) {
	c := NewConfiguration()
	err := c.ParseFlags("izmac", []string{"-rom", "rom.bin"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	if c.DriverFile != defaultDriverFile {
		t.Errorf("the driver defaults to %v, wanted %v", c.DriverFile, defaultDriverFile)
	}
	if !c.driverIsDefault {
		t.Error("the default driver was not marked as downloadable")
	}
}

// One named on the command line is the caller's to provide, so it is never
// fetched behind their back
func TestANamedDriverIsNeverDownloaded(t *testing.T) {
	c := NewConfiguration()
	err := c.ParseFlags("izmac",
		[]string{"-rom", "rom.bin", "-driver", "mine.img"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	if c.driverIsDefault {
		t.Error("a driver named on the command line was marked as downloadable")
	}
}

func TestTheFilesOnTheCommandLineAreSorted(t *testing.T) {
	hard := writeImage(t, "hard.img", 4*1024*1024, true)
	floppy := writeImage(t, "floppy.img", 800*1024, false)

	c := NewConfiguration()
	err := c.ParseFlags("izmac", []string{"-rom", "rom.bin", hard, floppy}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	if len(c.DiskFiles) != 1 || c.DiskFiles[0] != hard {
		t.Errorf("the hard disk did not reach the bus, the disks are %v", c.DiskFiles)
	}
	if len(c.Diskettes) != 1 || c.Diskettes[0] != floppy {
		t.Errorf("the diskette was not set aside, the diskettes are %v", c.Diskettes)
	}
}

func TestTheDiskFlagCanBeRepeated(t *testing.T) {
	c := NewConfiguration()
	err := c.ParseFlags("izmac",
		[]string{"-rom", "rom.bin", "-hd", "one.img", "-hd", "two.img"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	if len(c.DiskFiles) != 2 || c.DiskFiles[0] != "one.img" || c.DiskFiles[1] != "two.img" {
		t.Errorf("the disks are %v, wanted both in the order given", c.DiskFiles)
	}
}

func TestMoreDisksThanTheBusTakesIsRefused(t *testing.T) {
	args := []string{"-rom", "rom.bin"}
	for i := 0; i <= scsi.TargetCount; i++ {
		args = append(args, "-hd", "disk.img")
	}

	c := NewConfiguration()
	if err := c.ParseFlags("izmac", args, io.Discard); err == nil {
		t.Errorf("%v disks were accepted on a bus that takes %v",
			scsi.TargetCount+1, scsi.TargetCount)
	}
}

func TestTheFloppyFlagCanBeRepeated(t *testing.T) {
	c := NewConfiguration()
	err := c.ParseFlags("izmac",
		[]string{"-rom", "rom.bin", "-floppy", "one.dsk", "-floppy", "two.dsk"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	if len(c.Diskettes) != 2 || c.Diskettes[0] != "one.dsk" || c.Diskettes[1] != "two.dsk" {
		t.Errorf("the diskettes are %v, wanted both in the order given", c.Diskettes)
	}
}

func TestMoreDiskettesThanThereAreDrivesIsRefused(t *testing.T) {
	args := []string{"-rom", "rom.bin"}
	for i := 0; i <= DriveCount; i++ {
		args = append(args, "-floppy", "floppy.dsk")
	}

	c := NewConfiguration()
	if err := c.ParseFlags("izmac", args, io.Discard); err == nil {
		t.Errorf("%v diskettes were accepted by a machine with %v drives",
			DriveCount+1, DriveCount)
	}
}

// A DiskCopy image is bigger than the diskette it holds, so its size says
// nothing and it has to be recognised by its header
func TestADiskCopyFileIsSortedAsADiskette(t *testing.T) {
	data := make([]uint8, 84+800*1024)
	data[0] = 4
	copy(data[1:], "Work")
	data[64], data[65], data[66], data[67] = 0x00, 0x0c, 0x80, 0x00 // 800Kb
	data[82], data[83] = 0x01, 0x00                                 // What says it is one

	filename := filepath.Join(t.TempDir(), "image.dc42")
	if err := os.WriteFile(filename, data, 0o600); err != nil {
		t.Fatal(err)
	}

	c := NewConfiguration()
	if err := c.AddFiles([]string{filename}); err != nil {
		t.Fatal(err)
	}

	if len(c.Diskettes) != 1 || len(c.DiskFiles) != 0 {
		t.Errorf("a DiskCopy image gave %v diskettes and %v disks, wanted one diskette",
			len(c.Diskettes), len(c.DiskFiles))
	}
}

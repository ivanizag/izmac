package izmac

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

/*
The images the end to end tests run on, in test_images, which is part of the
repository so that every test runs on a fresh checkout and none of them skips
for want of a file. test_images/README.md says what is on each and how they
were made.

A test never runs a machine on one of them as it is: the Finder writes to its
disks as it starts, and a run would change the files the next run starts from.
testImage hands out a copy of the test's own.

The end to end tests take most of the time of the tests, a machine started and
taken through what it does each: go test -short leaves them out. They run in
parallel, each machine being of its own, but for what shares something of the
host: the test of LocalTalk over UDP, whose multicast group the others would
be heard on, and the one that makes the images.
*/
const (
	testImages = "test_images"

	// testRom is the ROM izmac targets, the Plus v3
	testRom = testImages + "/macplus.rom"

	// testScsiDriver is a blank disk formatted by Apple's HD SC Setup, the
	// driver of a disk that comes without one
	testScsiDriver = testImages + "/hddriver.img"

	// testSystemSixDisk is System 6.0.8 on a bare 2 MB volume, with the desk
	// accessories in its System; testSystemSixDiskette the same System on an
	// 800K diskette with the drivers AppleShare needs and its Chooser
	// extension, MultiFinder and TeachText
	testSystemSixDisk     = testImages + "/system6.img"
	testSystemSixDiskette = testImages + "/system6.dsk"

	// testSystemSevenDisk is System 7.1.2 on a partitioned 4 MB disk, with
	// AppleShare and File Sharing
	testSystemSevenDisk = testImages + "/system7.img"

	// testPaintDiskette is MacPaint 1.5 with System 2.0 and Finder 4.1 on a
	// 400K diskette
	testPaintDiskette = testImages + "/macpaint.dsk"

	// testSystemFourDiskette is System 4.1 and the Finder 5.5 on an 800K
	// diskette
	testSystemFourDiskette = testImages + "/system41.dsk"

	// The four 800K diskettes System 6.0.8 was sold on, as they were
	testSystemToolsDiskette   = testImages + "/system-tools.dsk"
	testUtilitiesOneDiskette  = testImages + "/utilities-1.dsk"
	testUtilitiesTwoDiskette  = testImages + "/utilities-2.dsk"
	testPrintingToolsDiskette = testImages + "/printing-tools.dsk"

	// The disks and archives of the activities, each as it was downloaded:
	// applications of the time on diskettes that start the machine, and
	// archives as the software sites kept them
	testMacWriteDiskette  = testImages + "/macwrite.dsk"
	testMultiplanDiskette = testImages + "/multiplan.dsk"
	testBasicDiskette     = testImages + "/basic.dsk"
	testHyperCardDiskette = testImages + "/hypercard.dsk"
	testResEditArchive    = testImages + "/resedit.sit"
	testBoloArchive       = testImages + "/bolo.sit"
	testLodeRunnerDisk    = testImages + "/loderunner.dsk"
	testDarkCastleDisk    = testImages + "/darkcastle.dsk"
)

// testImage is a copy of one of the test images, in the test's own directory
func testImage(t testing.TB, name string) string {
	t.Helper()

	source, err := os.Open(name)
	if err != nil {
		t.Fatalf("the test image %v is missing: %v", name, err)
	}
	defer source.Close()

	copied := filepath.Join(t.TempDir(), filepath.Base(name))
	target, err := os.Create(copied)
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()

	if _, err := io.Copy(target, source); err != nil {
		t.Fatal(err)
	}
	return copied
}

/*
testConfig is a configuration for a machine on the test images, with nothing
in its drives yet, for an end to end test, which -short skips: the test ROM, the SCSI driver for bare volumes, and the
parameter RAM in a file of the test's own. The default is a file on the
working directory that outlives the run, and the clock starts from what it
holds: a machine booted from a parameter RAM left by a run an hour ago
believes it is an hour ago.
*/
func testConfig(t testing.TB) *Configuration {
	t.Helper()
	if testing.Short() {
		t.Skip("an end to end test, which -short leaves out")
	}

	config := NewConfiguration()
	config.RomFile = testRom
	config.ScsiDriverFile = testScsiDriver
	config.PramFile = filepath.Join(t.TempDir(), "pram.bin")
	config.messages = io.Discard
	return config
}

// buildTestMac validates a configuration and builds its machine
func buildTestMac(t testing.TB, config *Configuration) *Mac {
	t.Helper()

	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	m, err := NewMac(config)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

/*
TestSettleTestImages is not a test: it is the last step of making the test
images, after test_images/build.sh, and does nothing unless asked for with
IZMAC_SETTLE_TEST_IMAGES. It boots each image that starts a machine, in place,
lets the Finder make the desktop file a new disk gets on its first start, and
shuts it down. The tests then start from a disk that has been started before,
as a disk mostly has, and spend their time on what they test.
*/
func TestSettleTestImages(t *testing.T) {
	if os.Getenv("IZMAC_SETTLE_TEST_IMAGES") == "" {
		t.Skip("this makes the test images, see test_images/build.sh")
	}

	for _, image := range []struct {
		name     string
		diskette bool
		ramKb    int
		boot     uint64
		// Where Shut Down is: the Special menu, and its last item
		menuH, itemH, itemV int16
	}{
		{testSystemSixDiskette, true, 1024, 4000, 185, 200, 123},
		{testSystemSixDisk, false, 1024, 3000, 185, 200, 123},
		{testSystemSevenDisk, false, 4096, 9000, 215, 240, 139},
		{testSystemFourDiskette, true, 1024, 3000, 185, 200, 139},
	} {
		config := testConfig(t)
		if image.diskette {
			config.Diskettes = []string{image.name}
		} else {
			config.DiskFiles = []string{image.name}
		}
		config.RamSizeKb = image.ramKb
		m := buildTestMac(t, config)
		m.RunFrames(image.boot)
		shutDownFromTheFinder(t, m, image.menuH, image.itemH, image.itemV)
		waitForSwitchOff(t, m)
		if err := m.FlushDiskettes(); err != nil {
			t.Fatal(err)
		}
		t.Logf("%v has been started and shut down", image.name)
	}
}

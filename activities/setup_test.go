package activities

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ivanizag/izmac"
)

/*
What the activities of izmac's documentation run on: the images of
test_images, which a reader downloads the same of, a machine of the tests'
configuration on them, and the folder their pictures go to.
*/

// activityImages is where the pictures of the pages go, a folder each
const activityImages = "../doc/activities/images"

const (
	testImages = "../test_images"

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

	// The first two diskettes of THINK Pascal 4.0, as they were
	testThinkPascalOneDiskette = testImages + "/thinkpascal-1.dsk"
	testThinkPascalTwoDiskette = testImages + "/thinkpascal-2.dsk"
)

/*
systemSevenBootFrames is how long System 7 takes to reach its Finder from the
System 7 test disk, about twice what System 6 takes
*/
const systemSevenBootFrames = 6000

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
testConfig is a configuration for a machine on the test images: the test ROM,
the SCSI driver for bare volumes, the parameter RAM in a file of the test's
own, and nothing said of what is unpacked
*/
func testConfig(t testing.TB) *izmac.Configuration {
	t.Helper()
	config := izmac.NewConfiguration()
	config.RomFile = testRom
	config.ScsiDriverFile = testScsiDriver
	config.PramFile = filepath.Join(t.TempDir(), "pram.bin")
	config.Messages = io.Discard
	config.StartTime = activityStart()
	return config
}

/*
activityStart is the time the machines of the activities start at, the same on
every run so that a run of a page makes the same pictures: the clocks show the
same time, what is saved has the same date, and what depends on the time, as
anything random does, comes out the same
*/
func activityStart() time.Time {
	return time.Date(1991, 10, 1, 10, 0, 0, 0, time.UTC)
}

/*
predate dates files and folders of the host an hour before the machines
start, so that a folder shared with them is the same on every run too: the
host dates what it makes with its own time
*/
func predate(t testing.TB, paths ...string) {
	t.Helper()
	before := activityStart().Add(-time.Hour)
	for _, path := range paths {
		if err := os.Chtimes(path, before, before); err != nil {
			t.Fatal(err)
		}
	}
}

// buildTestMac validates a configuration and builds its machine, which is
// closed when the test ends
func buildTestMac(t testing.TB, config *izmac.Configuration) *izmac.Mac {
	t.Helper()
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	m, err := izmac.NewMac(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	return m
}

// must stops the activity when the operator could not do what it was asked
func must(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// exists says whether there is a file at a path
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

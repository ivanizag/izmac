package e2e_tests

import (
	"testing"
)

/*
The ways a Plus starts, each to the Finder on the test images: from a
diskette, from a hard disk that is a bare volume and borrows its SCSI driver,
and from a partitioned one with Apple's own driver, which is also System 7.
What says the machine got there is the Finder running, and the volume it
started from.
*/
func TestTheMachineStartsFromTheSystemSixDiskette(t *testing.T) {
	t.Parallel()
	config := testConfig(t)
	config.Diskettes = []string{testImage(t, testSystemSixDiskette)}
	m := buildTestMac(t, config)

	waitForApplication(t, m, "Finder", 60)
	if volume := startupVolume(m); volume != "System 6" {
		t.Errorf("the machine started from %q, wanted the diskette", volume)
	}
}

func TestTheMachineStartsFromABareVolume(t *testing.T) {
	t.Parallel()
	config := testConfig(t)
	config.DiskFiles = []string{testImage(t, testSystemSixDisk)}
	m := buildTestMac(t, config)

	waitForApplication(t, m, "Finder", 60)
	if volume := startupVolume(m); volume != "System 6 HD" {
		t.Errorf("the machine started from %q, wanted the bare volume", volume)
	}
}

func TestTheMachineStartsFromAPartitionedDisk(t *testing.T) {
	t.Parallel()
	config := testConfig(t)
	config.DiskFiles = []string{testImage(t, testSystemSevenDisk)}
	config.RamSizeKb = 4096
	m := buildTestMac(t, config)

	waitForApplication(t, m, "Finder", 120)
	if volume := startupVolume(m); volume != "System 7 HD" {
		t.Errorf("the machine started from %q, wanted the partitioned disk", volume)
	}
}

/*
With a diskette and a hard disk, the diskette goes first: the ROM looks in the
drives before the bus, which is how a Macintosh with a broken System on its
hard disk is started from a diskette
*/
func TestADisketteStartsTheMachineBeforeTheHardDisk(t *testing.T) {
	t.Parallel()
	config := testConfig(t)
	config.Diskettes = []string{testImage(t, testSystemSixDiskette)}
	config.DiskFiles = []string{testImage(t, testSystemSixDisk)}
	m := buildTestMac(t, config)

	waitForApplication(t, m, "Finder", 60)
	if volume := startupVolume(m); volume != "System 6" {
		t.Errorf("the machine started from %q, wanted the diskette", volume)
	}
}

/*
A document opened from the Finder starts its application: the disk's window
opened with Command O, the startup disk being what the Finder has selected,
and Read Me, a TeachText document, opened with a double click
*/
func TestTheFinderOpensADocumentInItsApplication(t *testing.T) {
	t.Parallel()
	m := bootedMac(t)
	waitForApplication(t, m, "Finder", 10)

	pressCommand(m, "O")
	m.RunFrames(300)
	doubleClickAt(t, m, 236, 110)

	waitForApplication(t, m, "TeachText", 20)
}

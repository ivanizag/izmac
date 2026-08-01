package izmac

import (
	"fmt"
	"io"
	"os"

	"github.com/ivanizag/izmac/storage"
)

/*
Where to find a SCSI driver, which is the part storage has no opinion on.
Reading one out of a disk image belongs to the image; knowing that izmac
wants one, what to call it and where a copy can be had belongs here.

A driver is only ever wanted for a bare volume, an image with no partition
map and no driver of its own. Those are made for the emulators that patch the
ROM to supply a driver, and izmac runs an unpatched ROM, so it borrows one
and makes up the maps around it as the disk is attached. Nothing is written
to the volume and nothing is written to the disk the driver came from.
*/
const (
	// defaultDriverFile is where the borrowed driver is kept, and is not a
	// ROM at all: it is the front of a disk image, the maps and the driver
	// in the layout they were found in. The name says what it is for.
	defaultDriverFile = "hddriver.rom"

	/*
		defaultDriverURL is a blank disk formatted by Apple's HD SC Setup,
		out of a collection of them kept for the SCSI adapters people put in
		real machines. It is pinned to the commit rather than the branch, so
		that what is fetched is the file that was tested and not whatever
		the branch has moved on to.

		Only the first blocks of it are kept. The rest is an empty volume
		and of no interest.
	*/
	defaultDriverURL = "https://raw.githubusercontent.com/MrGasS/" +
		"Blank-SCSI-hard-disk-images-for-Macintosh/" +
		"1e4b92eed88b3d0b8d535e6387f6813bd40b512b/" +
		"Blank%20Apple%20HD%20SC%20formatted%20images/" +
		"20mb%20%5Bpce-macplus%20-%20AppleHDSC%5D.zip"
)

/*
ensureDriver finds a driver for the bare volumes on the bus, if there are
any. A driver named on the command line is used as it is and never
downloaded; the default one is fetched if it is not on the working directory
already. A machine with nothing but proper disks on it needs none of this and
never goes looking.
*/
func ensureDriver(config *Configuration, out io.Writer) (*storage.Driver, error) {
	wanted, err := config.needsDriver()
	if err != nil {
		return nil, err
	}
	if !wanted {
		return nil, nil
	}

	if _, err := os.Stat(config.DriverFile); err != nil {
		if !config.driverIsDefault {
			return nil, fmt.Errorf("can not open the driver image: %w", err)
		}

		fmt.Fprintf(out, "A disk with no driver on it is attached, and %v is not here.\n",
			config.DriverFile)
		fmt.Fprintf(out, "Downloading one from %v\n", defaultDriverURL)

		driver, err := storage.DownloadDriver(config.DriverFile, defaultDriverURL)
		if err != nil {
			return nil, err
		}

		fmt.Fprintf(out, "Saved as %v: %v blocks of %v code\n",
			config.DriverFile, driver.Blocks(), driver.Processor)
		return driver, nil
	}

	return storage.ReadDriver(config.DriverFile)
}

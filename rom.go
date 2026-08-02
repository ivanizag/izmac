package izmac

import (
	"fmt"
	"io"
	"os"

	"github.com/ivanizag/izmac/storage"
)

/*
Which ROM izmac wants, which is the part storage has no opinion on. Reading an
image, checking it against its own checksum and saying which of the three
revisions it is belongs to the image; preferring one of them and knowing where
to find a copy belongs here.

The revision matters more than it would to a collector. They differ in the
SCSI driver, and v3 is the one that copes with a target answering a unit
attention after a reset, which is what a freshly built emulated disk does.
*/
const (
	// romBase is where the ROM lives on the normal address map
	romBase = 0x40_0000

	// preferredRomChecksum is the revision izmac targets, 'Loud Harmonicas'
	preferredRomChecksum = 0x4d1f8172

	/*
		defaultRomFile is the ROM used when none is named. It carries the
		izmac_ prefix that everything izmac writes for itself carries: the
		files land in the directory it was run from, beside whatever else is
		there, and the prefix is what says which of them are izmac's and
		which are the user's own.
	*/
	defaultRomFile = "izmac_default.rom"

	// defaultRomURL is the revision izmac targets, inside the Macintosh ROM
	// archive kept at the Internet Archive
	defaultRomURL = "https://archive.org/download/mac_rom_archive_-_as_of_8-19-2011/" +
		"mac_rom_archive_-_as_of_8-19-2011.zip/4D1F8172%20-%20MacPlus%20v3.ROM"
)

/*
ensureRom fetches the default ROM if it is the one wanted and it is not on the
working directory. A ROM named on the command line is never downloaded; a
missing one is an error the caller reports.
*/
func ensureRom(config *Configuration, out io.Writer) error {
	if !config.romIsDefault {
		return nil
	}
	if _, err := os.Stat(config.RomFile); err == nil {
		return nil
	}

	fmt.Fprintf(out, "The ROM file %v is not here.\n", config.RomFile)
	fmt.Fprintf(out, "  Downloading it from %v\n", defaultRomURL)

	r, err := storage.DownloadRom(config.RomFile, defaultRomURL)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "  Saved as %v: %v\n", config.RomFile, r)
	return nil
}

// isPreferredRom returns true for the revision izmac targets
func isPreferredRom(r *storage.Rom) bool {
	return r.Checksum() == preferredRomChecksum
}

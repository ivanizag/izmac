package izmac

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

/*
The ROM is copyrighted and can not be distributed with izmac, but a run
without one can not do anything at all. When no ROM is named on the command
line and the default file is not on the working directory, it is fetched from
the Macintosh ROM archive kept at the Internet Archive.

The image downloaded is the revision 3, 'Loud Harmonicas', which is the one
izmac targets.
*/
const (
	// defaultRomFile is the ROM used when none is given
	defaultRomFile = "default.rom"

	// defaultRomURL is the revision 3 image inside the Macintosh ROM
	// archive at the Internet Archive
	defaultRomURL = "https://archive.org/download/mac_rom_archive_-_as_of_8-19-2011/" +
		"mac_rom_archive_-_as_of_8-19-2011.zip/4D1F8172%20-%20MacPlus%20v3.ROM"

	romDownloadTimeout = 2 * time.Minute
)

// ensureRom downloads the default ROM if it is the one wanted and it is not
// on the working directory. A ROM named on the command line is never
// downloaded, a missing one is an error the caller reports.
func ensureRom(config *Configuration, out io.Writer) error {
	if !config.romIsDefault {
		return nil
	}
	if _, err := os.Stat(config.RomFile); err == nil {
		return nil
	}

	return downloadRom(config.RomFile, defaultRomURL, out)
}

// downloadRom fetches a ROM image, checks it before writing it so that a
// failed download does not leave a broken file behind
func downloadRom(filename string, url string, out io.Writer) error {
	fmt.Fprintf(out, "The ROM file %v is not here.\n", filename)
	fmt.Fprintf(out, "Downloading it from %v\n", url)

	client := &http.Client{Timeout: romDownloadTimeout}
	response, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("can not download the ROM: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("can not download the ROM: the server answered %v", response.Status)
	}

	data, err := io.ReadAll(io.LimitReader(response.Body, romSize+1))
	if err != nil {
		return fmt.Errorf("can not download the ROM: %w", err)
	}

	r, err := parseRom(data)
	if err != nil {
		return fmt.Errorf("what was downloaded is not a usable ROM: %w", err)
	}

	err = os.WriteFile(filename, data, 0o600)
	if err != nil {
		return fmt.Errorf("can not save the ROM: %w", err)
	}

	fmt.Fprintf(out, "Saved as %v: %v\n", filename, r)
	return nil
}

package storage

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

/*
The three things izmac needs and can not carry: the ROM, a SCSI driver for the
disks that have none of their own, and a diskette to boot when nothing at all
was named. All of it is somebody else's software, so none of it is distributed
and all of it is fetched on demand. Whether to fetch is the caller's decision;
this only does it.

They are fetched the same way and saved on the same terms. What is downloaded
is checked before anything is written, so a download that turns out to be
something else leaves no file behind to be picked up as good next time.
*/

const downloadTimeout = 2 * time.Minute

/*
download reads a URL into memory, up to limit bytes so that a wrong URL can
not be pulled in forever. The name is what is being fetched, for the errors.
*/
func download(url string, limit int64, name string) ([]uint8, error) {
	client := &http.Client{Timeout: downloadTimeout}
	response, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("can not download the %v: %w", name, err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("can not download the %v: the server answered %v",
			name, response.Status)
	}

	data, err := io.ReadAll(io.LimitReader(response.Body, limit))
	if err != nil {
		return nil, fmt.Errorf("can not download the %v: %w", name, err)
	}
	return data, nil
}

// DownloadRom fetches a ROM image and returns what it turned out to be
func DownloadRom(filename string, url string) (*Rom, error) {
	data, err := download(url, RomSize+1, "ROM")
	if err != nil {
		return nil, err
	}

	r, err := parseRom(data)
	if err != nil {
		return nil, fmt.Errorf("what was downloaded is not a usable ROM: %w", err)
	}

	err = os.WriteFile(filename, data, 0o600)
	if err != nil {
		return nil, fmt.Errorf("can not save the ROM: %w", err)
	}

	return r, nil
}

const (
	/*
		scsiDriverFileBlocks is how much of the donor is kept. It only has to
		reach past the SCSI driver, and every disk Apple's formatter writes
		puts that inside the first hundred or so blocks. Keeping too much
		costs nothing and keeping too little is caught: the image is parsed
		before it is written and a driver past the end is a read that fails.
	*/
	scsiDriverFileBlocks = 128

	// scsiDriverDownloadLimit bounds what is read for a SCSI driver. The
	// disks that carry one come zipped and the smallest of them are tens of
	// kilobytes.
	scsiDriverDownloadLimit = 4 << 20
)

/*
DownloadScsiDriver fetches a disk image with a SCSI driver on it and keeps the
front of it in filename. What is saved is not a driver on its own but the
first blocks of a disk, the driver descriptor map and the partition map along
with it, so that it parses as the disk image it is.
*/
func DownloadScsiDriver(filename string, url string) (*ScsiDriver, error) {
	archive, err := download(url, scsiDriverDownloadLimit, "SCSI driver")
	if err != nil {
		return nil, err
	}

	data, err := frontOfZippedImage(archive)
	if err != nil {
		return nil, fmt.Errorf("what was downloaded from %v is no use: %w", url, err)
	}

	scsiDriver, err := readScsiDriverAt(bytes.NewReader(data), url)
	if err != nil {
		return nil, fmt.Errorf("what was downloaded is not a disk with a SCSI driver: %w", err)
	}

	err = os.WriteFile(filename, data, 0o600)
	if err != nil {
		return nil, fmt.Errorf("can not save the SCSI driver: %w", err)
	}

	scsiDriver.source = filename
	return scsiDriver, nil
}

/*
disketteDownloadLimit bounds what is read for a diskette, packed and unpacked
alike. The biggest one the machine can read is 800Kb of sectors with 19Kb of
tags behind them, so anything that goes past this is not a diskette however it
got here.
*/
const disketteDownloadLimit = 2 << 20

/*
DownloadDiskette fetches a diskette image and saves it as it comes. It is
parsed before it is written, the way the others are, so that what is left on
the working directory is a diskette that goes in a drive on the next run
without being downloaded again.

The image is taken as it is or out of a zip, since the places these are
published do it both ways, and a diskette that arrives zipped is unpacked
rather than turned away.
*/
func DownloadDiskette(filename string, url string) (*FloppyDisk, error) {
	data, err := download(url, disketteDownloadLimit, "diskette")
	if err != nil {
		return nil, err
	}

	if isZip(data) {
		data, err = zippedImage(data)
		if err != nil {
			return nil, fmt.Errorf("what was downloaded from %v is no use: %w", url, err)
		}
	}

	d := &FloppyDisk{name: filename, filename: filename}
	if err := d.load(data); err != nil {
		return nil, fmt.Errorf("what was downloaded is not a diskette: %w", err)
	}

	if err := os.WriteFile(filename, data, 0o600); err != nil {
		return nil, fmt.Errorf("can not save the diskette: %w", err)
	}

	return d, nil
}

// isZip tells an archive from an image by the four bytes a zip starts with,
// which no diskette does
func isZip(data []uint8) bool {
	return len(data) >= 4 && string(data[:4]) == "PK\x03\x04"
}

/*
theOneFileIn finds the disk image in a zip. A zip packed on a Macintosh
carries a second entry for every file, under __MACOSX and with the name
prefixed, holding the resource fork and the Finder information split off it.
Those are not images and are stepped over.
*/
func theOneFileIn(archive []uint8) (*zip.File, error) {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("not a zip: %w", err)
	}

	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name, "__MACOSX/") ||
			strings.HasPrefix(path.Base(entry.Name), "._") {
			continue
		}
		return entry, nil
	}

	return nil, fmt.Errorf("the zip holds no file")
}

/*
frontOfZippedImage takes the first blocks of the one disk image in a zip. The
image inside is far bigger than the part wanted, so it is read up to the
length needed and no further rather than unpacked whole.
*/
func frontOfZippedImage(archive []uint8) ([]uint8, error) {
	entry, err := theOneFileIn(archive)
	if err != nil {
		return nil, err
	}

	file, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer file.Close()

	data := make([]uint8, scsiDriverFileBlocks*BlockSize)
	_, err = io.ReadFull(file, data)
	if err != nil {
		return nil, fmt.Errorf("the zip holds %v, which is too short to be a disk: %w",
			entry.Name, err)
	}
	return data, nil
}

/*
zippedImage unpacks the one disk image in a zip whole, which is what a
diskette is taken as: it is small enough to keep and every byte of it is
wanted, tags and all.
*/
func zippedImage(archive []uint8) ([]uint8, error) {
	entry, err := theOneFileIn(archive)
	if err != nil {
		return nil, err
	}

	file, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, disketteDownloadLimit))
	if err != nil {
		return nil, fmt.Errorf("the zip holds %v, which can not be read: %w",
			entry.Name, err)
	}
	return data, nil
}

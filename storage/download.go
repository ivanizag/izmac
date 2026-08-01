package storage

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

/*
The two things izmac needs and can not carry: the ROM, and a SCSI driver for
the disks that have none of their own. Both are Apple's code, so neither is
distributed and both are fetched on demand. Whether to fetch is the caller's
decision; this only does it.

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
		driverFileBlocks is how much of the donor is kept. It only has to
		reach past the driver, and every disk Apple's formatter writes puts
		that inside the first hundred or so blocks. Keeping too much costs
		nothing and keeping too little is caught: the image is parsed before
		it is written and a driver past the end is a read that fails.
	*/
	driverFileBlocks = 128

	// driverDownloadLimit bounds what is read for a driver. The disks that
	// carry one come zipped and the smallest of them are tens of kilobytes.
	driverDownloadLimit = 4 << 20
)

/*
DownloadDriver fetches a disk image with a driver on it and keeps the front
of it in filename. What is saved is not a driver on its own but the first
blocks of a disk, the driver descriptor map and the partition map along with
it, so that it parses as the disk image it is.
*/
func DownloadDriver(filename string, url string) (*Driver, error) {
	archive, err := download(url, driverDownloadLimit, "driver")
	if err != nil {
		return nil, err
	}

	data, err := frontOfZippedImage(archive)
	if err != nil {
		return nil, fmt.Errorf("what was downloaded from %v is no use: %w", url, err)
	}

	driver, err := readDriverAt(bytes.NewReader(data), url)
	if err != nil {
		return nil, fmt.Errorf("what was downloaded is not a disk with a driver: %w", err)
	}

	err = os.WriteFile(filename, data, 0o600)
	if err != nil {
		return nil, fmt.Errorf("can not save the driver: %w", err)
	}

	driver.source = filename
	return driver, nil
}

/*
frontOfZippedImage takes the first blocks of the one disk image in a zip. The
image inside is far bigger than the part wanted, so it is read up to the
length needed and no further rather than unpacked whole.
*/
func frontOfZippedImage(archive []uint8) ([]uint8, error) {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("not a zip: %w", err)
	}

	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}

		file, err := entry.Open()
		if err != nil {
			return nil, err
		}
		defer file.Close()

		data := make([]uint8, driverFileBlocks*BlockSize)
		_, err = io.ReadFull(file, data)
		if err != nil {
			return nil, fmt.Errorf("the zip holds %v, which is too short to be a disk: %w",
				entry.Name, err)
		}
		return data, nil
	}

	return nil, fmt.Errorf("the zip holds no file")
}

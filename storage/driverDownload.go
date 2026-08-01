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
A bare volume needs a driver and a driver is Apple's code, so izmac carries
none and fetches one the way it fetches the ROM. What it saves is not a
driver on its own but the front of a disk that has one: the driver descriptor
map, the partition map and the driver, in the layout they were found in. That
is a disk image as far as everything else here is concerned, so nothing has
to know it was cut short.

The disks that carry a driver come zipped, and the smallest of them is a few
tens of kilobytes, which is less than the part of an unzipped one would be.
So the whole file is fetched and opened in memory rather than reached into
over a range request.
*/

const (
	driverDownloadTimeout = 2 * time.Minute

	/*
		driverFileBlocks is how much of the donor is kept. It only has to
		reach past the driver, and every disk Apple's formatter writes puts
		that inside the first hundred or so blocks. Keeping too much costs
		nothing and keeping too little is caught: the image is parsed before
		it is written and a driver past the end is a read that fails.
	*/
	driverFileBlocks = 128

	// downloadSizeLimit is a bound on what is read from the network, so
	// that a wrong URL can not be pulled in forever
	downloadSizeLimit = 4 << 20
)

/*
DownloadDriver fetches a disk image with a driver on it and keeps the front
of it in filename. What was fetched is parsed before anything is written, so
a download that turns out to have no driver leaves no file behind.
*/
func DownloadDriver(filename string, url string) (*Driver, error) {
	client := &http.Client{Timeout: driverDownloadTimeout}
	response, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("can not download the driver: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("can not download the driver: the server answered %v",
			response.Status)
	}

	archive, err := io.ReadAll(io.LimitReader(response.Body, downloadSizeLimit))
	if err != nil {
		return nil, fmt.Errorf("can not download the driver: %w", err)
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
			return nil, fmt.Errorf("%v holds %v, which is too short to be a disk: %w",
				"the zip", entry.Name, err)
		}
		return data, nil
	}

	return nil, fmt.Errorf("the zip holds no file")
}

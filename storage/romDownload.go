package storage

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

/*
The ROM is copyrighted and can not be distributed, but a machine without one
can not do anything at all, so there is a way to fetch one. Whether to is the
caller's decision; this only does it.
*/
const romDownloadTimeout = 2 * time.Minute

/*
DownloadRom fetches a ROM image and returns what it turned out to be. It is
checked before anything is written, so a download that is not a ROM does not
leave a broken file behind.
*/
func DownloadRom(filename string, url string) (*Rom, error) {
	client := &http.Client{Timeout: romDownloadTimeout}
	response, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("can not download the ROM: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("can not download the ROM: the server answered %v",
			response.Status)
	}

	data, err := io.ReadAll(io.LimitReader(response.Body, RomSize+1))
	if err != nil {
		return nil, fmt.Errorf("can not download the ROM: %w", err)
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

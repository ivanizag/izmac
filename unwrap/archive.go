package unwrap

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"path"
	"strings"
)

// The two archivers the standard library undoes

func isZip(head []uint8) bool {
	return hasPrefix(head, "PK\x03\x04")
}

func isGzip(head []uint8) bool {
	return hasPrefix(head, "\x1f\x8b")
}

/*
openZip takes out every file in a zip. A zip made on a Macintosh carries a
second entry for each file, under __MACOSX and with the name prefixed, holding
the resource fork and the Finder information. Those are no use here and are
stepped over.
*/
func openZip(data []uint8) ([]File, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}

	var files []File
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() || isMacMetadata(entry.Name) {
			continue
		}

		content, err := readEntry(entry)
		if err != nil {
			return nil, fmt.Errorf("can not read %v: %w", entry.Name, err)
		}
		files = append(files, File{Name: path.Base(entry.Name), Data: content})
	}
	return files, nil
}

func readEntry(entry *zip.File) ([]uint8, error) {
	file, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return readLimited(file)
}

// isMacMetadata tells the files a Macintosh leaves next to the real ones on a
// host that has no resource forks
func isMacMetadata(name string) bool {
	return strings.HasPrefix(name, "__MACOSX/") ||
		strings.HasPrefix(path.Base(name), "._")
}

// openGzip takes out the one file in a gzip, named as it was or as the gzip
// is without the .gz
func openGzip(name string, data []uint8) ([]File, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	content, err := readLimited(reader)
	if err != nil {
		return nil, err
	}

	inner := reader.Name
	if inner == "" {
		inner = strings.TrimSuffix(path.Base(name), path.Ext(name))
	}
	return []File{{Name: path.Base(inner), Data: content}}, nil
}

// readLimited reads all of an archive entry, up to what one file is allowed
// to unpack to
func readLimited(r io.Reader) ([]uint8, error) {
	content, err := io.ReadAll(io.LimitReader(r, maxTotal+1))
	if err != nil {
		return nil, err
	}
	if len(content) > maxTotal {
		return nil, fmt.Errorf("unpacks to more than %vGb", maxTotal>>30)
	}
	return content, nil
}

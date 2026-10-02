package afp

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
)

/*
What a Macintosh file has besides its data, its resource fork and its Finder
information, is kept wherever the host keeps it. macOS keeps it with the file
itself, in extended attributes. Anywhere else, and on macOS for a file that
already has one, it is in an AppleDouble file next to the file, named with ._
in front, which is what macOS leaves itself on a volume that cannot hold
extended attributes and what netatalk and unar write.
*/
type metadataStore interface {
	// finderInfo is the Finder information of a file or a folder, and
	// whether it has any
	finderInfo(host string) ([32]uint8, bool)
	setFinderInfo(host string, info [32]uint8) error
	// resource is the resource fork of a file, empty if it has none
	resource(host string) ([]uint8, error)
	setResource(host string, data []uint8) error
	resourceLength(host string) int64
	// removed and renamed follow a file that went or moved, after it did
	removed(host string)
	renamed(from string, to string)
}

/*
AppleDouble, version 2: a header, the entries, each an id, an offset and a
length, and what they hold. What is written here is the Finder information,
entry 9, and the resource fork, entry 2.

	0  4  $00051607
	4  4  $00020000
	8 16  filler
	24 2  how many entries
	26    the entries
*/
const (
	appleDoubleMagic   = 0x00051607
	appleDoubleVersion = 0x00020000
	appleDoublePrefix  = "._"

	entryResource = 2
	entryFinder   = 9
)

// appleDoubleStore keeps everything in AppleDouble files
type appleDoubleStore struct{}

func sidecar(host string) string {
	return filepath.Join(filepath.Dir(host), appleDoublePrefix+filepath.Base(host))
}

// hasSidecar tells whether a file has an AppleDouble file
func hasSidecar(host string) bool {
	_, err := os.Stat(sidecar(host))
	return err == nil
}

// readAppleDouble is the Finder information and the resource fork of an
// AppleDouble file, and whether there is one
func readAppleDouble(host string) ([32]uint8, bool, []uint8) {
	var finder [32]uint8
	raw, err := os.ReadFile(sidecar(host))
	if err != nil || len(raw) < 26 || binary.BigEndian.Uint32(raw) != appleDoubleMagic {
		return finder, false, nil
	}

	hasFinder := false
	var resource []uint8
	count := int(binary.BigEndian.Uint16(raw[24:]))
	for i := 0; i < count; i++ {
		at := 26 + 12*i
		if at+12 > len(raw) {
			break
		}
		id := binary.BigEndian.Uint32(raw[at:])
		offset := int64(binary.BigEndian.Uint32(raw[at+4:]))
		length := int64(binary.BigEndian.Uint32(raw[at+8:]))
		if offset+length > int64(len(raw)) {
			continue
		}
		switch id {
		case entryFinder:
			copy(finder[:], raw[offset:offset+length])
			hasFinder = true
		case entryResource:
			resource = raw[offset : offset+length]
		}
	}
	return finder, hasFinder, resource
}

// writeAppleDouble writes an AppleDouble file, or removes it when there is
// nothing left to keep in it
func writeAppleDouble(host string, finder [32]uint8, resource []uint8) error {
	if finder == [32]uint8{} && len(resource) == 0 {
		err := os.Remove(sidecar(host))
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	const header = 26 + 2*12
	raw := make([]uint8, header, header+32+len(resource))
	binary.BigEndian.PutUint32(raw[0:], appleDoubleMagic)
	binary.BigEndian.PutUint32(raw[4:], appleDoubleVersion)
	binary.BigEndian.PutUint16(raw[24:], 2)

	binary.BigEndian.PutUint32(raw[26:], entryFinder)
	binary.BigEndian.PutUint32(raw[30:], header)
	binary.BigEndian.PutUint32(raw[34:], 32)
	binary.BigEndian.PutUint32(raw[38:], entryResource)
	binary.BigEndian.PutUint32(raw[42:], header+32)
	binary.BigEndian.PutUint32(raw[46:], uint32(len(resource)))

	raw = append(raw, finder[:]...)
	raw = append(raw, resource...)
	return os.WriteFile(sidecar(host), raw, 0o644)
}

func (appleDoubleStore) finderInfo(host string) ([32]uint8, bool) {
	finder, ok, _ := readAppleDouble(host)
	return finder, ok
}

func (appleDoubleStore) setFinderInfo(host string, info [32]uint8) error {
	_, _, resource := readAppleDouble(host)
	return writeAppleDouble(host, info, resource)
}

func (appleDoubleStore) resource(host string) ([]uint8, error) {
	_, _, resource := readAppleDouble(host)
	return resource, nil
}

func (appleDoubleStore) setResource(host string, data []uint8) error {
	finder, _, _ := readAppleDouble(host)
	return writeAppleDouble(host, finder, data)
}

func (s appleDoubleStore) resourceLength(host string) int64 {
	resource, _ := s.resource(host)
	return int64(len(resource))
}

func (appleDoubleStore) removed(host string) {
	os.Remove(sidecar(host))
}

func (appleDoubleStore) renamed(from string, to string) {
	if hasSidecar(from) {
		os.Rename(sidecar(from), sidecar(to))
	}
}

/*
guessFinderInfo is the Finder information of a file that has none, which is
what a file the host made has: a text file is one TeachText opens, and
anything else a document of no application
*/
func guessFinderInfo(name string) [32]uint8 {
	var finder [32]uint8
	switch strings.ToLower(filepath.Ext(name)) {
	case ".txt", ".text", ".md", ".c", ".h", ".p", ".pas", ".a", ".s":
		copy(finder[0:], "TEXTttxt")
	}
	return finder
}

// FinderInfo is the Finder information the server keeps for a file of the
// host, wherever the host keeps it, and whether it has any
func FinderInfo(host string) ([32]uint8, bool) {
	return newMetadataStore().finderInfo(host)
}

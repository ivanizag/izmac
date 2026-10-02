package afp

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
)

/*
What a Macintosh file has besides its data, its resource fork and its Finder
information, is kept in an AppleDouble file next to it, named with ._ in
front: what netatalk and unar write, and macOS itself on a disk that cannot
hold more than data. The same on every host, and nothing kept out of sight:
the files can be seen, copied and deleted like any other.

The folder that is shared keeps its own in a ._. inside it, rather than next
to it, outside what is shared, as macOS does for the root of a disk.

A file that has no AppleDouble file but what the host keeps of a Macintosh
file itself, which on macOS is the extended attributes of an application
unpacked or copied from a Macintosh, is read from there; what the machine
changes of it goes to an AppleDouble file, with the rest of what it had.
*/

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

	// rootSidecar is the name of the AppleDouble file of the shared
	// folder, inside it
	rootSidecar = "._."

	entryResource = 2
	entryFinder   = 9
)

/*
metadata is what a Macintosh file has besides its data. The resource fork is
only read when it is asked for; its length always is.
*/
type metadata struct {
	finder         [32]uint8
	hasFinder      bool
	resource       []uint8
	resourceLength int64
}

func (m metadata) empty() bool {
	return m.finder == [32]uint8{} && m.resourceLength == 0
}

// metadataStore keeps the metadata of the files of a shared folder
type metadataStore struct {
	root string
}

func newMetadataStore(root string) metadataStore {
	return metadataStore{root: root}
}

// sidecar is where the AppleDouble file of a file or a folder is
func (s metadataStore) sidecar(host string) string {
	if host == s.root {
		return filepath.Join(host, rootSidecar)
	}
	return filepath.Join(filepath.Dir(host), appleDoublePrefix+filepath.Base(host))
}

// read is the metadata of a file: its AppleDouble file's, or with none what
// the host keeps of it itself
func (s metadataStore) read(host string, withResource bool) metadata {
	f, err := os.Open(s.sidecar(host))
	if err != nil {
		return readHostMetadata(host, withResource)
	}
	defer f.Close()
	return readAppleDouble(f, withResource)
}

/*
write writes the metadata of a file in its AppleDouble file, or removes that
when there is nothing to keep. One with nothing in it is still written for a
file the host keeps metadata of itself, which would come back otherwise.
*/
func (s metadataStore) write(host string, m metadata) error {
	if m.empty() && readHostMetadata(host, false).empty() {
		err := os.Remove(s.sidecar(host))
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.WriteFile(s.sidecar(host), composeAppleDouble(m), 0o644)
}

// finderInfo is the Finder information of a file or a folder, and whether
// it has any
func (s metadataStore) finderInfo(host string) ([32]uint8, bool) {
	m := s.read(host, false)
	return m.finder, m.hasFinder
}

func (s metadataStore) setFinderInfo(host string, info [32]uint8) error {
	m := s.read(host, true)
	m.finder, m.hasFinder = info, true
	return s.write(host, m)
}

// resource is the resource fork of a file, empty if it has none
func (s metadataStore) resource(host string) []uint8 {
	return s.read(host, true).resource
}

func (s metadataStore) setResource(host string, data []uint8) error {
	m := s.read(host, true)
	m.resource, m.resourceLength = data, int64(len(data))
	return s.write(host, m)
}

func (s metadataStore) resourceLength(host string) int64 {
	return s.read(host, false).resourceLength
}

// removed and renamed take the AppleDouble file along with a file that went
// or moved, after it did
func (s metadataStore) removed(host string) {
	os.Remove(s.sidecar(host))
}

func (s metadataStore) renamed(from string, to string) {
	if _, err := os.Stat(s.sidecar(from)); err == nil {
		os.Rename(s.sidecar(from), s.sidecar(to))
	}
}

/*
readAppleDouble reads an AppleDouble file, the header and the entries first
and then only what is asked for; what is not one has nothing
*/
func readAppleDouble(f *os.File, withResource bool) metadata {
	var m metadata
	header := make([]uint8, 26)
	if _, err := f.ReadAt(header, 0); err != nil || binary.BigEndian.Uint32(header) != appleDoubleMagic {
		return m
	}
	entries := make([]uint8, 12*int(binary.BigEndian.Uint16(header[24:])))
	n, _ := f.ReadAt(entries, 26)
	entries = entries[:n-n%12]

	for at := 0; at < len(entries); at += 12 {
		id := binary.BigEndian.Uint32(entries[at:])
		offset := int64(binary.BigEndian.Uint32(entries[at+4:]))
		length := int64(binary.BigEndian.Uint32(entries[at+8:]))
		switch id {
		case entryFinder:
			// macOS puts its extended attributes after the Finder
			// information in the same entry; only the start is it
			n, _ := f.ReadAt(m.finder[:min(length, 32)], offset)
			m.hasFinder = n > 0
		case entryResource:
			if !withResource {
				m.resourceLength = length
				continue
			}
			resource := make([]uint8, length)
			n, _ := f.ReadAt(resource, offset)
			m.resource, m.resourceLength = resource[:n], int64(n)
		}
	}
	return m
}

// composeAppleDouble makes an AppleDouble file of the Finder information and
// the resource fork
func composeAppleDouble(m metadata) []uint8 {
	const header = 26 + 2*12
	raw := make([]uint8, header, header+32+len(m.resource))
	binary.BigEndian.PutUint32(raw[0:], appleDoubleMagic)
	binary.BigEndian.PutUint32(raw[4:], appleDoubleVersion)
	binary.BigEndian.PutUint16(raw[24:], 2)

	binary.BigEndian.PutUint32(raw[26:], entryFinder)
	binary.BigEndian.PutUint32(raw[30:], header)
	binary.BigEndian.PutUint32(raw[34:], 32)
	binary.BigEndian.PutUint32(raw[38:], entryResource)
	binary.BigEndian.PutUint32(raw[42:], header+32)
	binary.BigEndian.PutUint32(raw[46:], uint32(len(m.resource)))

	raw = append(raw, m.finder[:]...)
	return append(raw, m.resource...)
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
// host, and whether it has any
func FinderInfo(host string) ([32]uint8, bool) {
	return newMetadataStore("").finderInfo(host)
}

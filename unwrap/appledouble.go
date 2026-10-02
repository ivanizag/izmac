package unwrap

import (
	"encoding/binary"
	"path"
	"sort"
	"strings"
	"time"
)

/*
AppleDouble, how a Macintosh file keeps what is not its data fork on a host
that has nothing else to keep it in: a second file next to the first, named
with ._ in front, holding the resource fork and the Finder information. A zip
made on a Mac puts those under __MACOSX, and unar writes them next to what it
unpacks. AppleSingle is the same thing with the data fork inside as well.

	0  4  $00051607, or $00051600 for AppleSingle
	4  4  the version
	8 16  filler
	24 2  how many entries
	26    the entries, each an id, an offset and a length of four bytes

The entries looked at are the data fork, 1, the resource fork, 2, the dates,
8, as seconds since 2000, and the Finder information, 9, which starts with the
type, the creator and the flags. macOS puts its extended attributes after the
Finder information in the same entry, which is why only its start is read.
*/

const (
	appleDoubleMagic = 0x00051607
	appleSingleMagic = 0x00051600

	entryData     = 1
	entryResource = 2
	entryDates    = 8
	entryFinder   = 9

	appleDoublePrefix = "._"

	// epoch2000 is the start of the AppleDouble clock on the Unix one
	epoch2000 = 946684800
)

// appleDouble is what an AppleDouble or AppleSingle file says of a file
type appleDouble struct {
	data      []uint8
	hasData   bool
	resource  []uint8
	finder    []uint8
	modified  time.Time
	hasFinder bool
}

func parseAppleDouble(raw []uint8) (appleDouble, bool) {
	var a appleDouble
	if len(raw) < 26 {
		return a, false
	}
	magic := binary.BigEndian.Uint32(raw)
	if magic != appleDoubleMagic && magic != appleSingleMagic {
		return a, false
	}

	count := int(binary.BigEndian.Uint16(raw[24:]))
	for i := 0; i < count; i++ {
		at := 26 + 12*i
		if at+12 > len(raw) {
			return a, false
		}
		id := binary.BigEndian.Uint32(raw[at:])
		offset := int64(binary.BigEndian.Uint32(raw[at+4:]))
		length := int64(binary.BigEndian.Uint32(raw[at+8:]))
		if offset+length > int64(len(raw)) {
			return a, false
		}
		entry := raw[offset : offset+length]

		switch id {
		case entryData:
			a.data, a.hasData = entry, true
		case entryResource:
			a.resource = entry
		case entryDates:
			if len(entry) >= 8 {
				seconds := int32(binary.BigEndian.Uint32(entry[4:]))
				a.modified = time.Unix(int64(seconds)+epoch2000, 0)
			}
		case entryFinder:
			if len(entry) >= 10 {
				a.finder, a.hasFinder = entry, true
			}
		}
	}
	return a, true
}

// apply gives a file what the AppleDouble file kept for it
func (a *appleDouble) apply(f *File) {
	if a.hasData {
		f.Data = a.data
	}
	if len(a.resource) != 0 {
		f.Resource = a.resource
	}
	if a.hasFinder {
		copy(f.Type[:], a.finder[0:4])
		copy(f.Creator[:], a.finder[4:8])
		f.Flags = binary.BigEndian.Uint16(a.finder[8:10])
	}
	if !a.modified.IsZero() && f.Modified.IsZero() {
		f.Modified = a.modified
	}
}

// looseFile is a file as an archive or a folder of the host has it: a path,
// with slashes, and the bytes in it
type looseFile struct {
	path     string
	data     []uint8
	modified time.Time
}

/*
assemble puts the files of an archive or of a folder back together with their
AppleDouble halves. A file with nothing but a resource fork, an application
most of the time, leaves only the AppleDouble file behind it, and is made out
of that alone. An AppleDouble file that does not parse is not one, and is kept
as the file it is.
*/
func assemble(loose []looseFile) []File {
	files := make(map[string]*File)
	var order []string
	extras := make(map[string]appleDouble)

	for _, l := range loose {
		folder, name := path.Split(l.path)
		folder = strings.TrimPrefix(folder, "__MACOSX/")

		if strings.HasPrefix(name, appleDoublePrefix) {
			if a, ok := parseAppleDouble(l.data); ok {
				// The file the half came in is as old as the file it
				// belongs to, when it says nothing itself
				if a.modified.IsZero() {
					a.modified = l.modified
				}
				extras[folder+strings.TrimPrefix(name, appleDoublePrefix)] = a
				continue
			}
		}

		key := folder + name
		files[key] = &File{
			Name:     name,
			Folders:  splitFolders(folder),
			Data:     l.data,
			Modified: l.modified,
		}
		order = append(order, key)
	}

	// The halves with no file, in a stable order
	var orphans []string
	for key := range extras {
		if _, ok := files[key]; !ok {
			orphans = append(orphans, key)
		}
	}
	sort.Strings(orphans)
	for _, key := range orphans {
		folder, name := path.Split(key)
		files[key] = &File{Name: name, Folders: splitFolders(folder)}
		order = append(order, key)
	}

	out := make([]File, 0, len(order))
	for _, key := range order {
		f := files[key]
		if a, ok := extras[key]; ok {
			a.apply(f)
		}
		out = append(out, *f)
	}
	return out
}

// splitFolders turns a folder path, with slashes, into the names in it
func splitFolders(folder string) []string {
	var folders []string
	for _, name := range strings.Split(folder, "/") {
		if name != "" && name != "." {
			folders = append(folders, name)
		}
	}
	return folders
}

// Package unwrap takes files out of the wrappers classic Macintosh software is
// published in, knowing nothing of what is inside them.
package unwrap

import (
	"bytes"
	"fmt"
	"path"
	"strings"
	"time"
)

/*
Most of the old Macintosh software on the web does not come as a disk image a
machine can use, but wrapped in one or more layers made for getting it through
mail and FTP servers: BinHex, MacBinary, StuffIt, zip. The idea, and the list
of formats worth caring about, come from macprep
(https://github.com/mastorak/macprep), which does the same for Mini vMac with
unar and a Python script.

The layers are peeled off one at a time until what is left is in none of them.
BinHex, MacBinary, zip and gzip are undone here, in Go. StuffIt and the
archivers that came after it have too many compression methods between them to
be worth carrying, so those are handed to unar, from The Unarchiver, when it is
installed, and turned away with a reason when it is not.

What comes out is a Macintosh file as far as the wrappers kept one: both forks,
the type and creator and the Finder flags, when it was last changed, and the
folders it was in. A disk image keeps everything in its data fork; an
application keeps most of itself in its resource fork.
*/

// File is what is left once the wrappers are off
type File struct {
	Name string

	// Folders are the folders the file was in, the outermost first, as the
	// archive had them. A file in an archive that was itself in a folder
	// is in that folder too.
	Folders []string

	Data     []uint8
	Resource []uint8

	// Type and Creator are the Finder's, zero for a file that never had any
	Type    [4]uint8
	Creator [4]uint8

	// Flags are the Finder flags
	Flags uint16

	// Modified is when the file was last changed, zero when nothing said
	Modified time.Time
}

// IsMacFile tells a file made on a Macintosh from one made anywhere else, by
// what only a Macintosh gives a file: a resource fork, or a type and creator
func (f *File) IsMacFile() bool {
	return len(f.Resource) != 0 || f.Type != [4]uint8{} || f.Creator != [4]uint8{}
}

/*
IsClutter tells the files nobody put in an archive on purpose: what the Finder
and Windows leave in every folder they open, and the desktop databases of the
volume the files came from, which would only mislead the Finder of a new one.
*/
func (f *File) IsClutter() bool {
	switch f.Name {
	case ".DS_Store", "Thumbs.db", "desktop.ini", "Desktop DB", "Desktop DF":
		return true
	case "Desktop":
		return string(f.Type[:]) == "FNDR"
	}
	return isMacMetadata(f.Name)
}

const (
	// maxDepth is how many wrappers deep a file is followed. Real ones go
	// three or four: a disk image, in a StuffIt archive, in MacBinary, in
	// BinHex.
	maxDepth = 8

	// maxTotal bounds what one file can unpack to, so that a small archive
	// that expands without end is stopped before it takes the memory with
	// it. It is more than any disk image a Macintosh Plus has a use for.
	maxTotal = 2 << 30

	// HeadSize is how much of a file Identify needs to look at
	HeadSize = 64 << 10
)

// The names of the formats, as the messages give them
const (
	formatBinHex    = "BinHex"
	formatMacBinary = "MacBinary"
	formatZip       = "zip"
	formatGzip      = "gzip"
)

// Unwrapper takes files out of their wrappers. It holds the tables the
// decoding needs.
type Unwrapper struct {
	binhex *binhexDecoder

	// lookPath finds unar, and is a field so that the tests can pretend it
	// is not installed
	lookPath func(file string) (string, error)
}

// NewUnwrapper builds an Unwrapper
func NewUnwrapper() *Unwrapper {
	return &Unwrapper{
		binhex:   newBinhexDecoder(),
		lookPath: defaultLookPath,
	}
}

/*
Identify says which wrapper a file is in, or nothing if it is in none this
package knows. It needs the size of the file and no more than its first
HeadSize bytes, so that a hard disk image of hundreds of megabytes can be
looked at without being read.
*/
func (u *Unwrapper) Identify(name string, head []uint8, size int64) string {
	switch {
	case isZip(head):
		return formatZip
	case isGzip(head):
		return formatGzip
	case u.binhex.identify(head):
		return formatBinHex
	case isMacBinary(head, size):
		return formatMacBinary
	}

	return identifyForUnar(name, head)
}

/*
Unwrap peels every wrapper off a file and returns what was inside, in the order
it was found. A file in no wrapper at all comes back as it is.
*/
func (u *Unwrapper) Unwrap(name string, data []uint8) ([]File, error) {
	var total int64
	return u.unwrap(File{Name: name, Data: data}, 0, &total)
}

func (u *Unwrapper) unwrap(file File, depth int, total *int64) ([]File, error) {
	head := file.Data[:min(len(file.Data), HeadSize)]
	format := u.Identify(file.Name, head, int64(len(file.Data)))

	if format == "" {
		*total += int64(len(file.Data))
		if *total > maxTotal {
			return nil, fmt.Errorf("unpacks to more than %vGb, which is no disk "+
				"image this machine can use", maxTotal>>30)
		}
		return []File{file}, nil
	}

	if depth >= maxDepth {
		return nil, fmt.Errorf("%v is in more than %v wrappers", file.Name, maxDepth)
	}

	inside, err := u.open(format, file)
	if err != nil {
		return nil, fmt.Errorf("can not take %v out of %v: %w", file.Name, format, err)
	}

	var found []File
	for _, f := range inside {
		// What was in an archive goes where the archive was
		f.Folders = append(append([]string(nil), file.Folders...), f.Folders...)

		unwrapped, err := u.unwrap(f, depth+1, total)
		if err != nil {
			return nil, err
		}
		found = append(found, unwrapped...)
	}
	return found, nil
}

// open takes off one wrapper
func (u *Unwrapper) open(format string, file File) ([]File, error) {
	switch format {
	case formatZip:
		return openZip(file.Data)
	case formatGzip:
		return openGzip(file.Name, file.Data)
	case formatBinHex:
		f, err := u.binhex.decode(file.Data)
		if err != nil {
			return nil, err
		}
		return []File{f}, nil
	case formatMacBinary:
		f, err := openMacBinary(file.Data)
		if err != nil {
			return nil, err
		}
		return []File{f}, nil
	}

	return u.openWithUnar(format, file)
}

/*
Describe names a format for a message, with the article it takes: "a BinHex
file", "a StuffIt 5 archive".
*/
func Describe(format string) string {
	switch format {
	case formatBinHex, formatMacBinary:
		return "a " + format + " file"
	case formatGzip:
		return "a gzip file"
	case formatZip:
		return "a zip archive"
	}
	return "a " + format + " archive"
}

/*
Stem is a file name with the extensions of the wrappers and of the disk images
taken off the end, which is what is left to name something made out of it:
"Game.dsk.sit.hqx" is a "Game".
*/
func Stem(name string) string {
	stem := path.Base(strings.ReplaceAll(name, "\\", "/"))

	for {
		extension := strings.ToLower(path.Ext(stem))
		if extension == "" || extension == stem || !isKnownExtension(extension) {
			break
		}
		stem = stem[:len(stem)-len(extension)]
	}

	return stem
}

func isKnownExtension(extension string) bool {
	switch extension {
	case ".hqx", ".bin", ".macbin", ".zip", ".gz", ".sit", ".sitx", ".sea",
		".cpt", ".7z", ".rar", ".dsk", ".img", ".image", ".dc42",
		".diskcopy", ".smi", ".hfs", ".hda", ".vhd":
		return true
	}
	return false
}

/*
crc16 is the CRC both BinHex and MacBinary II carry, the one XMODEM uses: the
CCITT polynomial, starting from zero, the bits taken most significant first.
*/
func crc16(data []uint8) uint16 {
	var crc uint16
	for _, b := range data {
		crc ^= uint16(b) << 8
		for range 8 {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

// hasPrefix is bytes.HasPrefix with a string, which is how the signatures
// read best
func hasPrefix(data []uint8, prefix string) bool {
	return bytes.HasPrefix(data, []uint8(prefix))
}

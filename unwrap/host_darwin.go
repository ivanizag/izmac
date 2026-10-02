//go:build darwin

package unwrap

import (
	"encoding/binary"
	"os"

	"golang.org/x/sys/unix"
)

/*
readHostMetadata reads what macOS keeps of a file besides its data: the
resource fork, through the name the file system gives it, and the Finder
information, in an extended attribute. What an AppleDouble file already gave
the file is not overwritten.
*/
func readHostMetadata(name string, f *File) {
	if len(f.Resource) == 0 {
		if resource, err := os.ReadFile(name + "/..namedfork/rsrc"); err == nil {
			f.Resource = resource
		}
	}

	if f.Type == [4]uint8{} && f.Creator == [4]uint8{} {
		finder := make([]uint8, 32)
		if n, err := unix.Getxattr(name, "com.apple.FinderInfo", finder); err == nil && n >= 10 {
			copy(f.Type[:], finder[0:4])
			copy(f.Creator[:], finder[4:8])
			f.Flags = binary.BigEndian.Uint16(finder[8:10])
		}
	}
}

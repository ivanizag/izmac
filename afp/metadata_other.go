//go:build !darwin

package afp

import (
	"io/fs"
	"time"
)

// newMetadataStore keeps everything in AppleDouble files, which is all a host
// that is not macOS has
func newMetadataStore() metadataStore {
	return appleDoubleStore{}
}

// createdTime is when a file was made, which the host does not say, and is
// taken as when it was last changed
func createdTime(info fs.FileInfo) time.Time {
	return info.ModTime()
}

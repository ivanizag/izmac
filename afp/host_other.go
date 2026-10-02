//go:build !darwin

package afp

import (
	"io/fs"
	"time"
)

// readHostMetadata is what the host keeps of a Macintosh file itself, which
// on a host that is not macOS is nothing
func readHostMetadata(host string, withResource bool) metadata {
	return metadata{}
}

// createdTime is when a file was made, which the host does not say, and is
// taken as when it was last changed
func createdTime(info fs.FileInfo) time.Time {
	return info.ModTime()
}

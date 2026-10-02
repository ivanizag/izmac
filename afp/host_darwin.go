//go:build darwin

package afp

import (
	"io/fs"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

/*
readHostMetadata is what macOS keeps of a Macintosh file itself, in the
extended attributes it has for the Finder information and the resource fork.
They are read, and never written.
*/
func readHostMetadata(host string, withResource bool) metadata {
	var m metadata
	if n, err := unix.Getxattr(host, "com.apple.FinderInfo", m.finder[:]); err == nil && n > 0 {
		m.hasFinder = true
	}
	n, err := unix.Getxattr(host, "com.apple.ResourceFork", nil)
	if err != nil || n == 0 {
		return m
	}
	m.resourceLength = int64(n)
	if withResource {
		data := make([]uint8, n)
		if n, err = unix.Getxattr(host, "com.apple.ResourceFork", data); err == nil {
			m.resource, m.resourceLength = data[:n], int64(n)
		}
	}
	return m
}

// createdTime is when a file was made, which macOS keeps
func createdTime(info fs.FileInfo) time.Time {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return time.Unix(stat.Birthtimespec.Unix())
	}
	return info.ModTime()
}

//go:build darwin

package afp

import (
	"io/fs"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

/*
On macOS the resource fork and the Finder information are where macOS keeps
them, in the extended attributes it has for them, which the Finder of the host
shows and copies along with the file. A file that already has an AppleDouble
file, from a stick or an archive, keeps using it.
*/
const (
	xattrFinderInfo = "com.apple.FinderInfo"
	xattrResource   = "com.apple.ResourceFork"
)

type xattrStore struct {
	appleDoubleStore
}

func newMetadataStore() metadataStore {
	return xattrStore{}
}

func (s xattrStore) finderInfo(host string) ([32]uint8, bool) {
	if hasSidecar(host) {
		return s.appleDoubleStore.finderInfo(host)
	}
	var finder [32]uint8
	n, err := unix.Getxattr(host, xattrFinderInfo, finder[:])
	return finder, err == nil && n > 0
}

func (s xattrStore) setFinderInfo(host string, info [32]uint8) error {
	if hasSidecar(host) {
		return s.appleDoubleStore.setFinderInfo(host, info)
	}
	if info == [32]uint8{} {
		return removeXattr(host, xattrFinderInfo)
	}
	return unix.Setxattr(host, xattrFinderInfo, info[:], 0)
}

func (s xattrStore) resource(host string) ([]uint8, error) {
	if hasSidecar(host) {
		return s.appleDoubleStore.resource(host)
	}
	n, err := unix.Getxattr(host, xattrResource, nil)
	if err != nil || n == 0 {
		return nil, nil
	}
	data := make([]uint8, n)
	n, err = unix.Getxattr(host, xattrResource, data)
	if err != nil {
		return nil, err
	}
	return data[:n], nil
}

func (s xattrStore) setResource(host string, data []uint8) error {
	if hasSidecar(host) {
		return s.appleDoubleStore.setResource(host, data)
	}
	// The resource fork attribute is written into, as a fork is, and a
	// shorter one leaves the end of the longer one there: it goes first
	if err := removeXattr(host, xattrResource); err != nil || len(data) == 0 {
		return err
	}
	return unix.Setxattr(host, xattrResource, data, 0)
}

func (s xattrStore) resourceLength(host string) int64 {
	if hasSidecar(host) {
		return s.appleDoubleStore.resourceLength(host)
	}
	n, err := unix.Getxattr(host, xattrResource, nil)
	if err != nil {
		return 0
	}
	return int64(n)
}

// removeXattr removes an attribute, which is no error if there was none
func removeXattr(host string, name string) error {
	err := unix.Removexattr(host, name)
	if err == unix.ENOATTR {
		return nil
	}
	return err
}

// createdTime is when a file was made, which macOS keeps
func createdTime(info fs.FileInfo) time.Time {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return time.Unix(stat.Birthtimespec.Unix())
	}
	return info.ModTime()
}

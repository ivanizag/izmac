//go:build !windows

package afp

import "golang.org/x/sys/unix"

// freeSpace is how many bytes are free on the disk a folder is on, and how
// many it has
func freeSpace(folder string) (int64, int64) {
	var stat unix.Statfs_t
	if err := unix.Statfs(folder, &stat); err != nil {
		return 0, 0
	}
	block := int64(stat.Bsize)
	return int64(stat.Bavail) * block, int64(stat.Blocks) * block
}

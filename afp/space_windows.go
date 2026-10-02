//go:build windows

package afp

import "golang.org/x/sys/windows"

// freeSpace is how many bytes are free on the disk a folder is on, and how
// many it has
func freeSpace(folder string) (int64, int64) {
	name, err := windows.UTF16PtrFromString(folder)
	if err != nil {
		return 0, 0
	}
	var free, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(name, &free, &total, &totalFree); err != nil {
		return 0, 0
	}
	return int64(min(free, 1<<62)), int64(min(total, 1<<62))
}

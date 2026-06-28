//go:build !windows

package handler

import "syscall"

// diskUsage returns the total and free bytes of the filesystem containing the
// given path (Unix implementation).
func diskUsage(path string) (totalBytes int64, freeBytes int64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	totalBytes = int64(stat.Blocks) * int64(stat.Bsize)
	freeBytes = int64(stat.Bavail) * int64(stat.Bsize)
	return totalBytes, freeBytes, nil
}

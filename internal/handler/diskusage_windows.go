//go:build windows

package handler

import "golang.org/x/sys/windows"

// diskUsage returns the total and free bytes of the filesystem containing the
// given path (Windows implementation).
func diskUsage(path string) (totalBytes int64, freeBytes int64, err error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, err
	}

	var freeBytesAvailable, totalNumberOfBytes, totalNumberOfFreeBytes uint64
	err = windows.GetDiskFreeSpaceEx(pathPtr, &freeBytesAvailable, &totalNumberOfBytes, &totalNumberOfFreeBytes)
	if err != nil {
		return 0, 0, err
	}

	totalBytes = int64(totalNumberOfBytes)
	freeBytes = int64(totalNumberOfFreeBytes)
	return totalBytes, freeBytes, nil
}

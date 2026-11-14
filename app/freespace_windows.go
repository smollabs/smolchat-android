package app

import "golang.org/x/sys/windows"

// FreeSpace returns bytes available to the current user on dir's volume.
// Windows is only used for desktop development builds.
func FreeSpace(dir string) int64 {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0
	}
	var avail uint64
	if err := windows.GetDiskFreeSpaceEx(p, &avail, nil, nil); err != nil {
		return 0
	}
	return int64(avail)
}

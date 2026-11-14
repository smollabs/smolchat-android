//go:build !windows

package app

import "syscall"

// FreeSpace returns bytes available to unprivileged users on dir's volume.
func FreeSpace(dir string) int64 {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0
	}
	return int64(st.Bavail) * int64(st.Bsize)
}

//go:build linux

package executor

import "syscall"

func KillGroup(pid int) error { return syscall.Kill(-pid, syscall.SIGKILL) }
func FreeMB(path string) (uint64, error) {
	var s syscall.Statfs_t
	e := syscall.Statfs(path, &s)
	return s.Bavail * uint64(s.Bsize) / 1048576, e
}

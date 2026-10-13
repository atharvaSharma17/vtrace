//go:build !linux

package executor

import "fmt"

func KillGroup(pid int) error { return fmt.Errorf("process cleanup requires Linux") }
func FreeMB(path string) (uint64, error) {
	return 0, fmt.Errorf("storage capacity check requires Linux")
}

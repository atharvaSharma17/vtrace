//go:build !linux

package executor

import (
	"os/exec"
	"time"
)

func configure(c *exec.Cmd) { c.WaitDelay = 2 * time.Second }
func kill(c *exec.Cmd) {
	if c.Process != nil {
		c.Process.Kill()
	}
}

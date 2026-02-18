//go:build !windows

package update

import "syscall"

// setSysProcAttr configures the process to run in a new process group (Unix).
func setSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setpgid: true,
	}
}

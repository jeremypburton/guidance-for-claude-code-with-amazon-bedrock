//go:build windows

package update

import "syscall"

// setSysProcAttr configures the process to run in a new process group (Windows).
func setSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: 0x00000200, // CREATE_NEW_PROCESS_GROUP
	}
}

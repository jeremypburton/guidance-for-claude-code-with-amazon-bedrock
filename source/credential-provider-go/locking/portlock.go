package locking

import (
	"fmt"
	"net"
	"time"

	"credential-provider-go/internal"
)

// TryAcquirePort tests whether the given port is available by binding to it.
// Returns true if the port was available (and has been released).
func TryAcquirePort(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", internal.ListenAddress(), port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// WaitForPort waits until the given port becomes available (i.e., another process
// using it has finished). It polls every 500ms up to the timeout.
func WaitForPort(port int, timeout time.Duration) bool {
	internal.DebugPrint("Another authentication is in progress, waiting...")
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if TryAcquirePort(port) {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}

	return false
}

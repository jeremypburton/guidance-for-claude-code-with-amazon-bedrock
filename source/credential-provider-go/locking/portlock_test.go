package locking

import (
	"net"
	"testing"
	"time"
)

func TestTryAcquirePort_Available(t *testing.T) {
	// Port 0 lets OS pick a free port, but we need a specific port.
	// Use a high port that's very likely free.
	port := 48901
	if !TryAcquirePort(port) {
		t.Skip("port 48901 was not available")
	}
}

func TestTryAcquirePort_InUse(t *testing.T) {
	// Bind to a port, then verify TryAcquirePort returns false
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind: %v", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	if TryAcquirePort(port) {
		t.Error("expected port to be in use")
	}
}

func TestWaitForPort_AlreadyFree(t *testing.T) {
	port := 48902
	if !WaitForPort(port, 1*time.Second) {
		t.Skip("port 48902 was not available")
	}
}

func TestWaitForPort_BecomesAvailable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	port := ln.Addr().(*net.TCPAddr).Port

	// Release port after 200ms
	go func() {
		time.Sleep(200 * time.Millisecond)
		ln.Close()
	}()

	if !WaitForPort(port, 5*time.Second) {
		t.Error("expected port to become available")
	}
}

func TestWaitForPort_Timeout(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind: %v", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port

	start := time.Now()
	result := WaitForPort(port, 1*time.Second)
	elapsed := time.Since(start)

	if result {
		t.Error("expected timeout (false)")
	}
	if elapsed < 900*time.Millisecond {
		t.Errorf("timeout too early: %v", elapsed)
	}
}

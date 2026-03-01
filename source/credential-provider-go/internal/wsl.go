package internal

import (
	"os"
	"runtime"
	"strings"
	"sync"
)

var (
	wslOnce   sync.Once
	wslResult bool
)

// IsWSL reports whether the current process is running inside Windows
// Subsystem for Linux. The result is cached after the first call.
func IsWSL() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	wslOnce.Do(func() {
		data, err := os.ReadFile("/proc/version")
		if err != nil {
			return
		}
		wslResult = containsWSLMarker(string(data))
	})
	return wslResult
}

// containsWSLMarker checks whether a /proc/version string indicates WSL.
// WSL1 kernels contain "Microsoft", WSL2 kernels contain "microsoft".
func containsWSLMarker(content string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(lower, "microsoft") || strings.Contains(lower, "wsl")
}

// ListenAddress returns "0.0.0.0" on WSL2 (so the Windows host browser can
// reach the callback server) and "127.0.0.1" everywhere else.
func ListenAddress() string {
	if override := os.Getenv("CCWB_BIND_ADDRESS"); override != "" {
		return override
	}
	if IsWSL() {
		return "0.0.0.0"
	}
	return "127.0.0.1"
}

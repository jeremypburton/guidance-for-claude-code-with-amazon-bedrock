package internal

import (
	"runtime"
	"testing"
)

func TestContainsWSLMarker(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name:    "WSL2 kernel",
			content: "Linux version 5.15.90.1-microsoft-standard-WSL2 (oe-user@oe-host) (x86_64-msft-linux-gcc (GCC) 9.3.0, GNU ld (GNU Binutils) 2.34) #1 SMP Fri Jan 27 02:56:13 UTC 2023",
			want:    true,
		},
		{
			name:    "WSL1 kernel",
			content: "Linux version 4.4.0-19041-Microsoft (Microsoft@Microsoft.com) (gcc version 5.4.0 (GCC) ) #1237-Microsoft Sat Sep 11 14:32:00 PST 2021",
			want:    true,
		},
		{
			name:    "native Linux",
			content: "Linux version 6.1.0-18-amd64 (debian-kernel@lists.debian.org) (gcc-12 (Debian 12.2.0-14) 12.2.0, GNU ld (GNU Binutils for Debian) 2.40) #1 SMP PREEMPT_DYNAMIC Debian 6.1.76-1 (2024-02-01)",
			want:    false,
		},
		{
			name:    "empty string",
			content: "",
			want:    false,
		},
		{
			name:    "contains wsl lowercase",
			content: "Linux version 5.15.0-wsl-custom",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsWSLMarker(tt.content)
			if got != tt.want {
				t.Errorf("containsWSLMarker(%q) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}

func TestIsWSL_NonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("test only runs on non-Linux")
	}
	if IsWSL() {
		t.Error("IsWSL() should return false on non-Linux platforms")
	}
}

func TestListenAddress_Default(t *testing.T) {
	t.Setenv("CCWB_BIND_ADDRESS", "")
	addr := ListenAddress()
	if runtime.GOOS != "linux" {
		if addr != "127.0.0.1" {
			t.Errorf("ListenAddress() = %q on non-Linux, want 127.0.0.1", addr)
		}
	}
}

func TestListenAddress_Override(t *testing.T) {
	t.Setenv("CCWB_BIND_ADDRESS", "192.168.1.100")
	addr := ListenAddress()
	if addr != "192.168.1.100" {
		t.Errorf("ListenAddress() = %q, want 192.168.1.100", addr)
	}
}

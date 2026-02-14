package internal

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// resetDebugState restores default debug state after each test.
func resetDebugState(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		if LogFile != nil {
			LogFile.Close()
		}
		LogWriter = os.Stderr
		LogFile = nil
		Debug = false
	})
}

func TestDebugPrint_WritesToLogWriter(t *testing.T) {
	resetDebugState(t)
	var buf bytes.Buffer
	LogWriter = &buf
	Debug = true

	DebugPrint("hello %s", "world")

	got := buf.String()
	if !strings.Contains(got, "Debug: hello world") {
		t.Errorf("expected 'Debug: hello world' in output, got: %q", got)
	}
}

func TestDebugPrint_DisabledSkips(t *testing.T) {
	resetDebugState(t)
	var buf bytes.Buffer
	LogWriter = &buf
	Debug = false

	DebugPrint("should not appear")

	if buf.Len() != 0 {
		t.Errorf("expected no output when Debug=false, got: %q", buf.String())
	}
}

func TestInitDebug_LogFile(t *testing.T) {
	resetDebugState(t)
	tmpFile := filepath.Join(t.TempDir(), "test.log")
	t.Setenv("CREDENTIAL_PROCESS_LOG_FILE", tmpFile)
	t.Setenv("DEBUG_MODE", "true")

	InitDebug()

	if LogFile == nil {
		t.Fatal("expected LogFile to be set after InitDebug with env var")
	}
	if LogWriter == os.Stderr {
		t.Fatal("expected LogWriter to be redirected from stderr")
	}
	if !Debug {
		t.Fatal("expected Debug to be true after InitDebug with DEBUG_MODE=true")
	}

	// Write a debug message and verify it lands in the file
	DebugPrint("file test message")
	CloseDebug()

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	if !strings.Contains(string(data), "file test message") {
		t.Errorf("expected 'file test message' in log file, got: %q", string(data))
	}
}

func TestInitDebug_DebugModeEnvVar(t *testing.T) {
	for _, val := range []string{"true", "1", "yes", "y", "TRUE", "Yes"} {
		t.Run(val, func(t *testing.T) {
			resetDebugState(t)
			t.Setenv("DEBUG_MODE", val)

			InitDebug()

			if !Debug {
				t.Errorf("expected Debug=true for DEBUG_MODE=%q", val)
			}
		})
	}
}

func TestInitDebug_DebugModeDisabled(t *testing.T) {
	for _, val := range []string{"", "false", "0", "no"} {
		t.Run(val, func(t *testing.T) {
			resetDebugState(t)
			t.Setenv("DEBUG_MODE", val)

			InitDebug()

			if Debug {
				t.Errorf("expected Debug=false for DEBUG_MODE=%q", val)
			}
		})
	}
}

func TestInitDebug_InvalidPath(t *testing.T) {
	resetDebugState(t)
	t.Setenv("CREDENTIAL_PROCESS_LOG_FILE", "/nonexistent/dir/impossible.log")

	// Should not panic, should fall back to stderr
	InitDebug()

	if LogFile != nil {
		t.Error("expected LogFile to be nil for invalid path")
	}
	if LogWriter != os.Stderr {
		t.Error("expected LogWriter to remain os.Stderr for invalid path")
	}
}

func TestInitDebug_NoEnvVar(t *testing.T) {
	resetDebugState(t)
	t.Setenv("CREDENTIAL_PROCESS_LOG_FILE", "")

	InitDebug()

	if LogFile != nil {
		t.Error("expected LogFile to be nil when env var is empty")
	}
	if LogWriter != os.Stderr {
		t.Error("expected LogWriter to remain os.Stderr when env var is empty")
	}
}

func TestCloseDebug_NilSafe(t *testing.T) {
	resetDebugState(t)
	LogFile = nil

	// Should not panic
	CloseDebug()
}

func TestCloseDebug_ResetsState(t *testing.T) {
	resetDebugState(t)
	tmpFile := filepath.Join(t.TempDir(), "test.log")
	t.Setenv("CREDENTIAL_PROCESS_LOG_FILE", tmpFile)

	InitDebug()
	CloseDebug()

	if LogFile != nil {
		t.Error("CloseDebug() should nil out LogFile")
	}
	if LogWriter != os.Stderr {
		t.Error("CloseDebug() should reset LogWriter to os.Stderr")
	}
}

func TestStatusPrint_DualWrite(t *testing.T) {
	resetDebugState(t)
	tmpFile := filepath.Join(t.TempDir(), "status.log")
	t.Setenv("CREDENTIAL_PROCESS_LOG_FILE", tmpFile)

	InitDebug()

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	StatusPrint("Error: something broke %d\n", 42)

	w.Close()
	os.Stderr = oldStderr

	var stderrBuf bytes.Buffer
	stderrBuf.ReadFrom(r)

	CloseDebug()

	// Verify stderr got the message
	stderrOutput := stderrBuf.String()
	if !strings.Contains(stderrOutput, "Error: something broke 42") {
		t.Errorf("stderr should contain status message, got: %q", stderrOutput)
	}

	// Verify log file also got the message
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	if !strings.Contains(string(data), "Error: something broke 42") {
		t.Errorf("expected status message in log file, got: %q", string(data))
	}
}

func TestDebugPrint_FileOnly(t *testing.T) {
	resetDebugState(t)
	tmpFile := filepath.Join(t.TempDir(), "debug.log")
	t.Setenv("CREDENTIAL_PROCESS_LOG_FILE", tmpFile)
	t.Setenv("DEBUG_MODE", "true")

	InitDebug()

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	DebugPrint("debug only message")

	w.Close()
	os.Stderr = oldStderr

	var stderrBuf bytes.Buffer
	stderrBuf.ReadFrom(r)

	CloseDebug()

	// Verify stderr did NOT get the debug message
	stderrOutput := stderrBuf.String()
	if strings.Contains(stderrOutput, "debug only message") {
		t.Errorf("stderr should NOT contain debug message when log file is active, got: %q", stderrOutput)
	}

	// Verify log file got the message
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	if !strings.Contains(string(data), "debug only message") {
		t.Errorf("expected 'debug only message' in log file, got: %q", string(data))
	}
}

func TestStatusPrint_NoLogFile(t *testing.T) {
	resetDebugState(t)
	// With no log file, StatusPrint should write to stderr only (no panic)
	LogFile = nil
	LogWriter = os.Stderr

	// Should not panic
	StatusPrint("stderr only message\n")
}

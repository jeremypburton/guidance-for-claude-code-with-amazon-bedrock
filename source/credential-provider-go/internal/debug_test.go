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
		Debug = true
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

	InitDebug()

	if LogFile == nil {
		t.Fatal("expected LogFile to be set after InitDebug with env var")
	}
	if LogWriter == os.Stderr {
		t.Fatal("expected LogWriter to be redirected from stderr")
	}

	// Write a debug message and verify it lands in the file
	DebugPrint("file test message")
	LogFile.Close()

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	if !strings.Contains(string(data), "file test message") {
		t.Errorf("expected 'file test message' in log file, got: %q", string(data))
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

func TestErrorPrint_DualWrite(t *testing.T) {
	resetDebugState(t)
	tmpFile := filepath.Join(t.TempDir(), "error.log")
	t.Setenv("CREDENTIAL_PROCESS_LOG_FILE", tmpFile)

	InitDebug()

	// ErrorPrint writes to both stderr and the log file.
	// We can't easily capture stderr in this test, but we can verify
	// the log file gets the message.
	ErrorPrint("Error: something broke %d\n", 42)
	LogFile.Close()

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	if !strings.Contains(string(data), "Error: something broke 42") {
		t.Errorf("expected error message in log file, got: %q", string(data))
	}
}

func TestDebugPrint_FileOnly(t *testing.T) {
	resetDebugState(t)
	tmpFile := filepath.Join(t.TempDir(), "debug.log")
	t.Setenv("CREDENTIAL_PROCESS_LOG_FILE", tmpFile)

	InitDebug()
	Debug = true

	DebugPrint("debug only message")
	LogFile.Close()

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	if !strings.Contains(string(data), "debug only message") {
		t.Errorf("expected 'debug only message' in log file, got: %q", string(data))
	}
}

func TestErrorPrint_NoLogFile(t *testing.T) {
	resetDebugState(t)
	// With no log file, ErrorPrint should write to stderr only (no panic)
	LogFile = nil
	LogWriter = os.Stderr

	// Should not panic
	ErrorPrint("stderr only error\n")
}

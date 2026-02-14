package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// resetDebugState restores debug globals to their defaults.
func resetDebugState() {
	debugMode = false
	logWriter = os.Stderr
	if logFile != nil {
		logFile.Close()
		logFile = nil
	}
}

func TestInitDebug_AcceptedValues(t *testing.T) {
	accepted := []string{"true", "1", "yes", "y", "TRUE", "True", "YES", "Y"}

	for _, val := range accepted {
		t.Run(val, func(t *testing.T) {
			defer resetDebugState()
			os.Unsetenv("OTEL_HELPER_LOG_FILE")
			debugMode = false
			os.Setenv("DEBUG_MODE", val)
			defer os.Unsetenv("DEBUG_MODE")

			initDebug()

			if !debugMode {
				t.Errorf("initDebug() with DEBUG_MODE=%q should set debugMode=true", val)
			}
		})
	}
}

func TestInitDebug_RejectedValues(t *testing.T) {
	rejected := []string{"false", "0", "no", "n", "", "maybe", "on"}

	for _, val := range rejected {
		t.Run(val, func(t *testing.T) {
			defer resetDebugState()
			os.Unsetenv("OTEL_HELPER_LOG_FILE")
			debugMode = false
			os.Setenv("DEBUG_MODE", val)
			defer os.Unsetenv("DEBUG_MODE")

			initDebug()

			if debugMode {
				t.Errorf("initDebug() with DEBUG_MODE=%q should not set debugMode=true", val)
			}
		})
	}
}

func TestInitDebug_Unset(t *testing.T) {
	defer resetDebugState()
	os.Unsetenv("OTEL_HELPER_LOG_FILE")
	debugMode = false
	os.Unsetenv("DEBUG_MODE")

	initDebug()

	if debugMode {
		t.Error("initDebug() with unset DEBUG_MODE should not set debugMode=true")
	}
}

func TestInitDebug_LogFile(t *testing.T) {
	defer resetDebugState()

	tmpFile := filepath.Join(t.TempDir(), "otel-helper-test.log")
	os.Setenv("OTEL_HELPER_LOG_FILE", tmpFile)
	defer os.Unsetenv("OTEL_HELPER_LOG_FILE")
	os.Setenv("DEBUG_MODE", "true")
	defer os.Unsetenv("DEBUG_MODE")

	initDebug()

	if logFile == nil {
		t.Fatal("initDebug() should have opened a log file")
	}
	if logWriter == os.Stderr {
		t.Fatal("logWriter should not be os.Stderr when log file is configured")
	}

	// Write a debug message and verify it lands in the file
	debugPrint("test message %d", 42)

	closeDebug()
	logFile = nil

	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	if !strings.Contains(string(content), "[DEBUG] test message 42") {
		t.Errorf("Log file should contain debug message, got: %q", string(content))
	}
}

func TestInitDebug_LogFile_InvalidPath(t *testing.T) {
	defer resetDebugState()

	os.Setenv("OTEL_HELPER_LOG_FILE", "/nonexistent/dir/otel-helper.log")
	defer os.Unsetenv("OTEL_HELPER_LOG_FILE")
	os.Unsetenv("DEBUG_MODE")

	// Should not panic, should fall back to stderr
	initDebug()

	if logFile != nil {
		t.Error("logFile should be nil when file cannot be opened")
	}
	if logWriter != os.Stderr {
		t.Error("logWriter should fall back to os.Stderr when file cannot be opened")
	}
}

func TestCloseDebug_NilFile(t *testing.T) {
	defer resetDebugState()
	logFile = nil

	// Should not panic
	closeDebug()
}

func TestLogWarning_DualWrite(t *testing.T) {
	defer resetDebugState()

	tmpFile := filepath.Join(t.TempDir(), "otel-helper-test.log")
	os.Setenv("OTEL_HELPER_LOG_FILE", tmpFile)
	defer os.Unsetenv("OTEL_HELPER_LOG_FILE")
	os.Unsetenv("DEBUG_MODE")

	initDebug()

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	logWarning("something went wrong: %s", "details")

	w.Close()
	os.Stderr = oldStderr

	var stderrBuf bytes.Buffer
	stderrBuf.ReadFrom(r)

	closeDebug()
	logFile = nil

	// Verify stderr got the message
	stderrOutput := stderrBuf.String()
	if !strings.Contains(stderrOutput, "[WARNING] something went wrong: details") {
		t.Errorf("stderr should contain warning, got: %q", stderrOutput)
	}

	// Verify log file also got the message
	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	if !strings.Contains(string(content), "[WARNING] something went wrong: details") {
		t.Errorf("Log file should contain warning, got: %q", string(content))
	}
}

func TestLogError_DualWrite(t *testing.T) {
	defer resetDebugState()

	tmpFile := filepath.Join(t.TempDir(), "otel-helper-test.log")
	os.Setenv("OTEL_HELPER_LOG_FILE", tmpFile)
	defer os.Unsetenv("OTEL_HELPER_LOG_FILE")
	os.Unsetenv("DEBUG_MODE")

	initDebug()

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	logError("fatal problem: %s", "crash")

	w.Close()
	os.Stderr = oldStderr

	var stderrBuf bytes.Buffer
	stderrBuf.ReadFrom(r)

	closeDebug()
	logFile = nil

	// Verify stderr got the message
	stderrOutput := stderrBuf.String()
	if !strings.Contains(stderrOutput, "[ERROR] fatal problem: crash") {
		t.Errorf("stderr should contain error, got: %q", stderrOutput)
	}

	// Verify log file also got the message
	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	if !strings.Contains(string(content), "[ERROR] fatal problem: crash") {
		t.Errorf("Log file should contain error, got: %q", string(content))
	}
}

func TestDebugPrint_FileOnly(t *testing.T) {
	defer resetDebugState()

	tmpFile := filepath.Join(t.TempDir(), "otel-helper-test.log")
	os.Setenv("OTEL_HELPER_LOG_FILE", tmpFile)
	defer os.Unsetenv("OTEL_HELPER_LOG_FILE")
	os.Setenv("DEBUG_MODE", "true")
	defer os.Unsetenv("DEBUG_MODE")

	initDebug()

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	debugPrint("file-only message")

	w.Close()
	os.Stderr = oldStderr

	var stderrBuf bytes.Buffer
	stderrBuf.ReadFrom(r)

	closeDebug()
	logFile = nil

	// Verify stderr did NOT get the debug message
	stderrOutput := stderrBuf.String()
	if strings.Contains(stderrOutput, "file-only message") {
		t.Errorf("stderr should NOT contain debug message when log file is active, got: %q", stderrOutput)
	}

	// Verify log file got the message
	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	if !strings.Contains(string(content), "[DEBUG] file-only message") {
		t.Errorf("Log file should contain debug message, got: %q", string(content))
	}
}

func TestLogInfo_FileOnly(t *testing.T) {
	defer resetDebugState()

	tmpFile := filepath.Join(t.TempDir(), "otel-helper-test.log")
	os.Setenv("OTEL_HELPER_LOG_FILE", tmpFile)
	defer os.Unsetenv("OTEL_HELPER_LOG_FILE")
	os.Setenv("DEBUG_MODE", "true")
	defer os.Unsetenv("DEBUG_MODE")

	initDebug()

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	logInfo("info-only message")

	w.Close()
	os.Stderr = oldStderr

	var stderrBuf bytes.Buffer
	stderrBuf.ReadFrom(r)

	closeDebug()
	logFile = nil

	// Verify stderr did NOT get the info message
	stderrOutput := stderrBuf.String()
	if strings.Contains(stderrOutput, "info-only message") {
		t.Errorf("stderr should NOT contain info message when log file is active, got: %q", stderrOutput)
	}

	// Verify log file got the message
	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	if !strings.Contains(string(content), "[INFO] info-only message") {
		t.Errorf("Log file should contain info message, got: %q", string(content))
	}
}

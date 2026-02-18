package update

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseSemver(t *testing.T) {
	tests := []struct {
		input    string
		wantNil  bool
		major    int
		minor    int
		patch    int
		preRel   string
	}{
		{"1.2.3", false, 1, 2, 3, ""},
		{"0.0.0", false, 0, 0, 0, ""},
		{"1.2.3-beta", false, 1, 2, 3, "beta"},
		{"1.2.3+sha123", false, 1, 2, 3, ""},
		{"1.2.3-rc1+sha123", false, 1, 2, 3, "rc1"},
		{"", true, 0, 0, 0, ""},
		{"dev", true, 0, 0, 0, ""},
		{"unknown", true, 0, 0, 0, ""},
		{"1.2", true, 0, 0, 0, ""},
		{"a.b.c", true, 0, 0, 0, ""},
		{"1.2.3.4", true, 0, 0, 0, ""},
	}

	for _, tt := range tests {
		v := parseSemver(tt.input)
		if tt.wantNil {
			if v != nil {
				t.Errorf("parseSemver(%q) = %+v, want nil", tt.input, v)
			}
			continue
		}
		if v == nil {
			t.Errorf("parseSemver(%q) = nil, want non-nil", tt.input)
			continue
		}
		if v.Major != tt.major || v.Minor != tt.minor || v.Patch != tt.patch {
			t.Errorf("parseSemver(%q) = %d.%d.%d, want %d.%d.%d",
				tt.input, v.Major, v.Minor, v.Patch, tt.major, tt.minor, tt.patch)
		}
		if v.PreRelease != tt.preRel {
			t.Errorf("parseSemver(%q).PreRelease = %q, want %q", tt.input, v.PreRelease, tt.preRel)
		}
	}
}

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		remote string
		local  string
		want   bool
	}{
		{"1.1.0", "1.0.0", true},
		{"1.0.1", "1.0.0", true},
		{"2.0.0", "1.9.9", true},
		{"1.0.0", "1.0.0", false},  // same version
		{"0.9.0", "1.0.0", false},  // older
		{"dev", "1.0.0", false},    // unparseable remote
		{"1.0.0", "dev", false},    // unparseable local
		{"", "1.0.0", false},       // empty remote
		{"1.0.0", "", false},       // empty local
		{"1.1.0-beta", "1.0.0", false}, // don't update stable to pre-release
		{"1.1.0-beta", "1.0.0-alpha", true}, // update pre-release to pre-release
		{"1.1.0+sha1", "1.0.0+sha2", true}, // build metadata stripped
	}

	for _, tt := range tests {
		got := isNewerVersion(tt.remote, tt.local)
		if got != tt.want {
			t.Errorf("isNewerVersion(%q, %q) = %v, want %v", tt.remote, tt.local, got, tt.want)
		}
	}
}

func TestStateFileRoundTrip(t *testing.T) {
	// Use temp directory for state file
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Ensure install dir exists (state file lives in ~/claude-code-with-bedrock/)
	os.MkdirAll(filepath.Join(tmpDir, "claude-code-with-bedrock"), 0700)

	// Initially empty
	state := loadState()
	if state.LastCheckTime != "" {
		t.Errorf("Expected empty LastCheckTime, got %q", state.LastCheckTime)
	}

	// Save and reload
	now := time.Now().UTC().Format(time.RFC3339)
	state.LastCheckTime = now
	state.PendingVersion = "1.2.0"
	state.PendingMessage = "Auto-updated to version 1.2.0"
	saveState(state)

	loaded := loadState()
	if loaded.LastCheckTime != now {
		t.Errorf("LastCheckTime = %q, want %q", loaded.LastCheckTime, now)
	}
	if loaded.PendingVersion != "1.2.0" {
		t.Errorf("PendingVersion = %q, want %q", loaded.PendingVersion, "1.2.0")
	}
}

func TestStateFileCorruptionRecovery(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	installDir := filepath.Join(tmpDir, "claude-code-with-bedrock")
	os.MkdirAll(installDir, 0700)

	// Write corrupted state file
	os.WriteFile(filepath.Join(installDir, "update-state.json"), []byte("{invalid json"), 0600)

	// Loading should not panic and should return empty state
	state := loadState()
	if state.LastCheckTime != "" {
		t.Error("Expected empty state after corruption recovery")
	}

	// Corrupted file should have been renamed
	if _, err := os.Stat(filepath.Join(installDir, "update-state.json.corrupted")); os.IsNotExist(err) {
		t.Error("Expected .corrupted file to exist")
	}
}

func TestConsumePendingNotification(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	os.MkdirAll(filepath.Join(tmpDir, "claude-code-with-bedrock"), 0700)

	// Record a pending update
	recordPendingUpdate("1.2.0")

	// First consumption should return the message
	msg := consumePendingNotification()
	if msg != "Auto-updated to version 1.2.0" {
		t.Errorf("Expected pending message, got %q", msg)
	}

	// Second consumption should return empty (already consumed)
	msg = consumePendingNotification()
	if msg != "" {
		t.Errorf("Expected empty message after consumption, got %q", msg)
	}
}

func TestSettingsMerge(t *testing.T) {
	tmpDir := t.TempDir()

	// Create existing settings with a custom key
	existing := map[string]interface{}{
		"env": map[string]interface{}{
			"AWS_REGION":             "us-east-1",
			"MY_CUSTOM_KEY":          "custom-value",
			"CLAUDE_CODE_USE_BEDROCK": "1",
		},
		"awsAuthRefresh": "old-path --profile Test",
	}
	existingPath := filepath.Join(tmpDir, "existing.json")
	data, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(existingPath, data, 0644)

	// Create new template with placeholders
	newTemplate := map[string]interface{}{
		"env": map[string]interface{}{
			"AWS_REGION":             "us-west-2",
			"CLAUDE_CODE_USE_BEDROCK": "1",
			"NEW_KEY":                "new-value",
		},
		"awsAuthRefresh":   "__CREDENTIAL_PROCESS_PATH__ --profile Test",
		"otelHeadersHelper": "__OTEL_HELPER_PATH__",
	}
	templatePath := filepath.Join(tmpDir, "template.json")
	data, _ = json.MarshalIndent(newTemplate, "", "  ")
	os.WriteFile(templatePath, data, 0644)

	// Merge
	err := mergeSettings(existingPath, templatePath, "/home/user/claude-code-with-bedrock")
	if err != nil {
		t.Fatalf("mergeSettings failed: %v", err)
	}

	// Read result
	resultData, _ := os.ReadFile(existingPath)
	var result map[string]interface{}
	json.Unmarshal(resultData, &result)

	env := result["env"].(map[string]interface{})

	// New key takes precedence for AWS_REGION
	if env["AWS_REGION"] != "us-west-2" {
		t.Errorf("AWS_REGION = %q, want us-west-2", env["AWS_REGION"])
	}

	// Custom key preserved
	if env["MY_CUSTOM_KEY"] != "custom-value" {
		t.Errorf("MY_CUSTOM_KEY = %q, want custom-value", env["MY_CUSTOM_KEY"])
	}

	// New key added
	if env["NEW_KEY"] != "new-value" {
		t.Errorf("NEW_KEY = %q, want new-value", env["NEW_KEY"])
	}

	// Placeholder replaced
	authRefresh := result["awsAuthRefresh"].(string)
	expected := "/home/user/claude-code-with-bedrock/credential-process --profile Test"
	if authRefresh != expected {
		t.Errorf("awsAuthRefresh = %q, want %q", authRefresh, expected)
	}
}

func TestExtractZipSafety(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a zip with a valid file
	zipPath := filepath.Join(tmpDir, "test.zip")
	zw, _ := os.Create(zipPath)
	w := zip.NewWriter(zw)

	// Add a normal file
	f, _ := w.Create("test-binary")
	f.Write([]byte("#!/bin/sh\necho hello"))

	w.Close()
	zw.Close()

	// Extract should succeed
	extractDir, err := extractZip(zipPath)
	if err != nil {
		t.Fatalf("extractZip failed: %v", err)
	}
	defer os.RemoveAll(extractDir)

	// Verify file exists
	if _, err := os.Stat(filepath.Join(extractDir, "test-binary")); os.IsNotExist(err) {
		t.Error("Expected test-binary to exist in extracted directory")
	}
}

func TestExtractZipRejectsPathTraversal(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a zip with path traversal
	zipPath := filepath.Join(tmpDir, "malicious.zip")
	zw, _ := os.Create(zipPath)
	w := zip.NewWriter(zw)

	// Add a file with path traversal
	f, _ := w.Create("../../../etc/passwd")
	f.Write([]byte("malicious"))

	w.Close()
	zw.Close()

	// Extract should fail
	_, err := extractZip(zipPath)
	if err == nil {
		t.Error("Expected extractZip to reject path traversal")
	}
}

func TestGetPlatformKey(t *testing.T) {
	key := getPlatformKey()
	if key == "" {
		t.Error("getPlatformKey() returned empty string on supported platform")
	}
}

func TestCheckLockPath(t *testing.T) {
	path := checkLockPath()
	if path == "" {
		t.Error("checkLockPath() returned empty string")
	}
}

func TestLockAcquireRelease(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	// First acquire should succeed
	if !tryAcquireCheckLock(lockPath) {
		t.Error("First lock acquire failed")
	}

	// Second acquire should fail
	if tryAcquireCheckLock(lockPath) {
		t.Error("Second lock acquire should have failed")
	}

	// Release
	releaseCheckLock(lockPath)

	// Third acquire should succeed
	if !tryAcquireCheckLock(lockPath) {
		t.Error("Third lock acquire failed after release")
	}
	releaseCheckLock(lockPath)
}

func TestStaleLockCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	// Create a lock file with old modification time
	f, _ := os.Create(lockPath)
	f.Close()
	oldTime := time.Now().Add(-10 * time.Minute)
	os.Chtimes(lockPath, oldTime, oldTime)

	// Acquire should succeed because lock is stale
	if !tryAcquireCheckLock(lockPath) {
		t.Error("Failed to acquire stale lock")
	}
	releaseCheckLock(lockPath)
}

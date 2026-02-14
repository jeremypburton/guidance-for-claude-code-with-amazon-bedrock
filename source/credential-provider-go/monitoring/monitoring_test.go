package monitoring

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestSaveAndGetMonitoringToken(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	// Clear env var to test file-based retrieval
	os.Unsetenv("CLAUDE_CODE_MONITORING_TOKEN")

	claims := jwt.MapClaims{
		"email": "test@example.com",
		"exp":   float64(time.Now().UTC().Add(2 * time.Hour).Unix()),
	}

	SaveMonitoringToken("test-id-token", "test-refresh-token", claims, "TestProfile")

	// Verify file was created with 0600 permissions
	tokenFile := filepath.Join(tmpHome, ".claude-code-session", "TestProfile-monitoring.json")
	info, err := os.Stat(tokenFile)
	if err != nil {
		t.Fatalf("token file not created: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected 0600 permissions, got %o", info.Mode().Perm())
	}

	// Clear env var set by SaveMonitoringToken
	os.Unsetenv("CLAUDE_CODE_MONITORING_TOKEN")

	// Retrieve
	token := GetMonitoringToken("TestProfile")
	if token != "test-id-token" {
		t.Errorf("expected test-id-token, got %s", token)
	}
}

func TestGetMonitoringToken_ExpiredToken(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)
	os.Unsetenv("CLAUDE_CODE_MONITORING_TOKEN")

	claims := jwt.MapClaims{
		"email": "test@example.com",
		"exp":   float64(time.Now().UTC().Add(-1 * time.Hour).Unix()), // expired
	}

	SaveMonitoringToken("expired-token", "refresh-token", claims, "TestProfile")
	os.Unsetenv("CLAUDE_CODE_MONITORING_TOKEN")

	token := GetMonitoringToken("TestProfile")
	if token != "" {
		t.Errorf("expected empty token for expired, got %s", token)
	}
}

func TestGetMonitoringToken_FromEnv(t *testing.T) {
	os.Setenv("CLAUDE_CODE_MONITORING_TOKEN", "env-token-value")
	defer os.Unsetenv("CLAUDE_CODE_MONITORING_TOKEN")

	token := GetMonitoringToken("AnyProfile")
	if token != "env-token-value" {
		t.Errorf("expected env-token-value, got %s", token)
	}
}

func TestGetMonitoringToken_NoToken(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)
	os.Unsetenv("CLAUDE_CODE_MONITORING_TOKEN")

	token := GetMonitoringToken("NonExistent")
	if token != "" {
		t.Errorf("expected empty token, got %s", token)
	}
}

func TestGetMonitoringToken_ExpiringWithin10Minutes(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)
	os.Unsetenv("CLAUDE_CODE_MONITORING_TOKEN")

	// Token expires in 5 minutes (less than 600s threshold)
	claims := jwt.MapClaims{
		"email": "test@example.com",
		"exp":   float64(time.Now().UTC().Add(5 * time.Minute).Unix()),
	}

	SaveMonitoringToken("almost-expired-token", "refresh-token", claims, "TestProfile")
	os.Unsetenv("CLAUDE_CODE_MONITORING_TOKEN")

	token := GetMonitoringToken("TestProfile")
	if token != "" {
		t.Errorf("expected empty token for nearly expired, got %s", token)
	}
}

func TestGetCachedTokenClaims(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	claims := jwt.MapClaims{
		"email": "user@company.com",
		"exp":   float64(time.Now().UTC().Add(1 * time.Hour).Unix()),
	}

	SaveMonitoringToken("id-token", "refresh-token", claims, "Prof1")

	cached := GetCachedTokenClaims("Prof1")
	if cached == nil {
		t.Fatal("expected non-nil claims")
	}
	if cached["email"] != "user@company.com" {
		t.Errorf("expected user@company.com, got %s", cached["email"])
	}
}

func TestGetRefreshToken(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	claims := jwt.MapClaims{
		"email": "test@example.com",
		"exp":   float64(time.Now().UTC().Add(2 * time.Hour).Unix()),
	}

	SaveMonitoringToken("id-token", "my-refresh-token", claims, "TestProfile")

	rt := GetRefreshToken("TestProfile")
	if rt != "my-refresh-token" {
		t.Errorf("expected my-refresh-token, got %s", rt)
	}
}

func TestGetRefreshToken_NoFile(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	rt := GetRefreshToken("NonExistent")
	if rt != "" {
		t.Errorf("expected empty string, got %s", rt)
	}
}

func TestGetRefreshToken_EmptyRefreshToken(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	claims := jwt.MapClaims{
		"email": "test@example.com",
		"exp":   float64(time.Now().UTC().Add(2 * time.Hour).Unix()),
	}

	// Save without refresh token
	SaveMonitoringToken("id-token", "", claims, "TestProfile")

	rt := GetRefreshToken("TestProfile")
	if rt != "" {
		t.Errorf("expected empty string, got %s", rt)
	}
}

func TestGetCachedTokenClaims_NoFile(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	cached := GetCachedTokenClaims("NonExistent")
	if cached != nil {
		t.Error("expected nil for missing file")
	}
}

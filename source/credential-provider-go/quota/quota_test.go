package quota

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestShouldCheck(t *testing.T) {
	if ShouldCheck("") {
		t.Error("expected false for empty endpoint")
	}
	if !ShouldCheck("https://api.example.com") {
		t.Error("expected true for non-empty endpoint")
	}
}

func TestCheck_Allowed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/check" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer test-token" {
			t.Errorf("unexpected auth header: %s", authHeader)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"allowed": true,
			"reason":  "within_limits",
		})
	}))
	defer server.Close()

	claims := jwt.MapClaims{"email": "test@example.com"}
	result := Check(server.URL, "test-token", claims, "open", 5)

	if !result.Allowed {
		t.Error("expected allowed=true")
	}
	if result.Reason != "within_limits" {
		t.Errorf("expected reason=within_limits, got %s", result.Reason)
	}
}

func TestCheck_Blocked(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"allowed": false,
			"reason":  "monthly_limit_exceeded",
			"message": "You have exceeded your monthly limit",
			"usage": map[string]interface{}{
				"monthly_tokens":  float64(1000000),
				"monthly_limit":   float64(500000),
				"monthly_percent": float64(200),
			},
		})
	}))
	defer server.Close()

	claims := jwt.MapClaims{"email": "test@example.com"}
	result := Check(server.URL, "test-token", claims, "open", 5)

	if result.Allowed {
		t.Error("expected allowed=false")
	}
	if result.Reason != "monthly_limit_exceeded" {
		t.Errorf("expected reason=monthly_limit_exceeded, got %s", result.Reason)
	}
}

func TestCheck_NoEmail(t *testing.T) {
	claims := jwt.MapClaims{"sub": "user123"}
	result := Check("https://api.example.com", "test-token", claims, "open", 5)

	if !result.Allowed {
		t.Error("expected allowed=true when no email")
	}
	if result.Reason != "no_email" {
		t.Errorf("expected reason=no_email, got %s", result.Reason)
	}
}

func TestCheck_ServerError_FailOpen(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	claims := jwt.MapClaims{"email": "test@example.com"}
	result := Check(server.URL, "test-token", claims, "open", 5)

	if !result.Allowed {
		t.Error("expected allowed=true for fail-open on server error")
	}
}

func TestCheck_ServerError_FailClosed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	claims := jwt.MapClaims{"email": "test@example.com"}
	result := Check(server.URL, "test-token", claims, "closed", 5)

	if result.Allowed {
		t.Error("expected allowed=false for fail-closed on server error")
	}
}

func TestCheck_401_FailOpen(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	claims := jwt.MapClaims{"email": "test@example.com"}
	result := Check(server.URL, "test-token", claims, "open", 5)

	if !result.Allowed {
		t.Error("expected allowed=true for fail-open on 401")
	}
	if result.Reason != "jwt_invalid" {
		t.Errorf("expected reason=jwt_invalid, got %s", result.Reason)
	}
}

func TestCheck_ConnectionError_FailOpen(t *testing.T) {
	claims := jwt.MapClaims{"email": "test@example.com"}
	// Use an invalid URL that will fail to connect
	result := Check("http://127.0.0.1:1", "test-token", claims, "open", 1)

	if !result.Allowed {
		t.Error("expected allowed=true for fail-open on connection error")
	}
}

func TestExtractGroups(t *testing.T) {
	claims := jwt.MapClaims{
		"groups":            []interface{}{"admins", "developers"},
		"cognito:groups":    []interface{}{"team-a"},
		"custom:department": "engineering",
	}

	groups := extractGroups(claims)

	expected := map[string]bool{
		"admins":                  false,
		"developers":             false,
		"team-a":                 false,
		"department:engineering": false,
	}
	for _, g := range groups {
		if _, ok := expected[g]; ok {
			expected[g] = true
		}
	}
	for name, found := range expected {
		if !found {
			t.Errorf("expected group %s not found", name)
		}
	}
}

func TestExtractGroups_StringValue(t *testing.T) {
	claims := jwt.MapClaims{
		"groups": "single-group",
	}

	groups := extractGroups(claims)
	if len(groups) != 1 || groups[0] != "single-group" {
		t.Errorf("expected [single-group], got %v", groups)
	}
}

func TestShouldRecheck(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	// No endpoint configured
	if ShouldRecheck("", 30, "test") {
		t.Error("should not recheck when no endpoint")
	}

	// Interval 0 = always check
	if !ShouldRecheck("https://api.example.com", 0, "test") {
		t.Error("should always recheck when interval=0")
	}

	// No previous check = should recheck
	if !ShouldRecheck("https://api.example.com", 30, "test") {
		t.Error("should recheck when no previous check")
	}

	// After saving timestamp, should not recheck within interval
	os.MkdirAll(filepath.Join(tmpHome, ".claude-code-session"), 0700)
	SaveQuotaCheckTimestamp("test")

	if ShouldRecheck("https://api.example.com", 30, "test") {
		t.Error("should not recheck immediately after saving timestamp")
	}
}

func TestSaveAndGetQuotaCheckTimestamp(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	os.MkdirAll(filepath.Join(tmpHome, ".claude-code-session"), 0700)

	SaveQuotaCheckTimestamp("test-profile")

	lastCheck := getLastQuotaCheckTime("test-profile")
	if lastCheck.IsZero() {
		t.Fatal("expected non-zero timestamp")
	}

	elapsed := time.Since(lastCheck)
	if elapsed > 5*time.Second {
		t.Errorf("timestamp too old: %v ago", elapsed)
	}
}

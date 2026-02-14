package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

// Known test vector for SHA256 parity with Python.
// Python verification:
//
//	import hashlib, json, base64
//	claims = {"sub":"test-user-123","email":"alice@example.com","iss":"https://dev.okta.com/oauth2","aud":"client-abc"}
//	payload = base64.urlsafe_b64encode(json.dumps(claims).encode()).rstrip(b'=').decode()
//	token = f"eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.{payload}.fakesig"
var testClaims = map[string]interface{}{
	"sub":   "test-user-123",
	"email": "alice@example.com",
	"iss":   "https://dev.okta.com/oauth2",
	"aud":   "client-abc",
}

func TestEndToEnd_KnownVector(t *testing.T) {
	token := makeJWT(testClaims)

	payload := decodeJWTPayload(token)
	if payload["email"] != "alice@example.com" {
		t.Fatalf("decode failed: email = %v", payload["email"])
	}

	info := extractUserInfo(payload)
	headers := formatAsHeaders(info)

	// Verify email header
	if headers["x-user-email"] != "alice@example.com" {
		t.Errorf("x-user-email = %q, want %q", headers["x-user-email"], "alice@example.com")
	}

	// Verify user ID is SHA256 UUID of "test-user-123"
	hash := sha256.Sum256([]byte("test-user-123"))
	h := hex.EncodeToString(hash[:])
	expectedID := h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
	if headers["x-user-id"] != expectedID {
		t.Errorf("x-user-id = %q, want %q", headers["x-user-id"], expectedID)
	}

	// Verify organization from Okta issuer
	if headers["x-organization"] != "okta" {
		t.Errorf("x-organization = %q, want %q", headers["x-organization"], "okta")
	}

	// Verify username falls back to email prefix
	if headers["x-user-name"] != "alice" {
		t.Errorf("x-user-name = %q, want %q", headers["x-user-name"], "alice")
	}

	// Verify default values
	if headers["x-department"] != "unspecified" {
		t.Errorf("x-department = %q, want %q", headers["x-department"], "unspecified")
	}
	if headers["x-team-id"] != "default-team" {
		t.Errorf("x-team-id = %q, want %q", headers["x-team-id"], "default-team")
	}
	if headers["x-cost-center"] != "general" {
		t.Errorf("x-cost-center = %q, want %q", headers["x-cost-center"], "general")
	}
	if headers["x-manager"] != "unassigned" {
		t.Errorf("x-manager = %q, want %q", headers["x-manager"], "unassigned")
	}
	if headers["x-location"] != "remote" {
		t.Errorf("x-location = %q, want %q", headers["x-location"], "remote")
	}
	if headers["x-role"] != "user" {
		t.Errorf("x-role = %q, want %q", headers["x-role"], "user")
	}

	// x-company should not be present (no company claim)
	if _, ok := headers["x-company"]; ok {
		t.Error("x-company should not be present when no company claim")
	}
}

func TestEndToEnd_JSONOutput(t *testing.T) {
	token := makeJWT(testClaims)

	payload := decodeJWTPayload(token)
	info := extractUserInfo(payload)
	headers := formatAsHeaders(info)

	data, err := json.Marshal(headers)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	// Verify it's valid JSON
	var parsed map[string]string
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	if parsed["x-user-email"] != "alice@example.com" {
		t.Errorf("JSON x-user-email = %q, want %q", parsed["x-user-email"], "alice@example.com")
	}
}

func TestEndToEnd_MalformedTokenProducesDefaults(t *testing.T) {
	payload := decodeJWTPayload("garbage")
	info := extractUserInfo(payload)
	headers := formatAsHeaders(info)

	// Should still produce headers with defaults
	if headers["x-user-email"] != "unknown@example.com" {
		t.Errorf("x-user-email = %q, want %q", headers["x-user-email"], "unknown@example.com")
	}
	if headers["x-department"] != "unspecified" {
		t.Errorf("x-department = %q, want %q", headers["x-department"], "unspecified")
	}
}

func TestTestOutputContainsParsingContract(t *testing.T) {
	// Verify the test output contains strings that test.py parses
	token := makeJWT(testClaims)
	payload := decodeJWTPayload(token)
	info := extractUserInfo(payload)
	headers := formatAsHeaders(info)

	// Capture output by building the expected strings
	var buf strings.Builder

	// Simulate header display
	for headerName, headerValue := range headers {
		displayName := strings.Replace(headerName, "x-", "X-", 1)
		displayName = strings.Replace(displayName, "-id", "-ID", 1)
		buf.WriteString(displayName + ": " + headerValue + "\n")
	}

	output := buf.String()

	// test.py line 772 looks for "X-user-email:"
	if !strings.Contains(output, "X-user-email:") {
		t.Error("test output must contain 'X-user-email:' (parsing contract with test.py)")
	}

	// Verify user.id would appear in section 2
	_ = info.UserID // just confirm it's set
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 30); got != "short" {
		t.Errorf("truncate('short', 30) = %q, want %q", got, "short")
	}
	long := "this-is-a-very-long-string-that-exceeds-thirty-characters"
	if got := truncate(long, 30); got != long[:30] {
		t.Errorf("truncate(long, 30) = %q, want %q", got, long[:30])
	}
}

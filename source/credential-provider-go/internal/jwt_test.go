package internal

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

// buildTestJWT creates an unsigned JWT with the given claims for testing.
func buildTestJWT(claims map[string]interface{}) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload, _ := json.Marshal(claims)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	return header + "." + payloadB64 + "."
}

func TestDecodeJWTUnverified(t *testing.T) {
	claims := map[string]interface{}{
		"sub":   "user123",
		"email": "test@example.com",
		"iss":   "https://example.okta.com",
		"exp":   float64(9999999999),
	}
	token := buildTestJWT(claims)

	decoded, err := DecodeJWTUnverified(token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decoded["sub"] != "user123" {
		t.Errorf("expected sub=user123, got %v", decoded["sub"])
	}
	if decoded["email"] != "test@example.com" {
		t.Errorf("expected email=test@example.com, got %v", decoded["email"])
	}
	if decoded["iss"] != "https://example.okta.com" {
		t.Errorf("expected iss=https://example.okta.com, got %v", decoded["iss"])
	}
}

func TestDecodeJWTUnverified_InvalidToken(t *testing.T) {
	_, err := DecodeJWTUnverified("not.a.valid.jwt")
	if err == nil {
		t.Error("expected error for invalid JWT")
	}
}

func TestDecodeJWTUnverified_EmptyToken(t *testing.T) {
	_, err := DecodeJWTUnverified("")
	if err == nil {
		t.Error("expected error for empty token")
	}
}

func TestDecodeJWTUnverified_WithGroups(t *testing.T) {
	claims := map[string]interface{}{
		"sub":    "user456",
		"email":  "admin@corp.com",
		"groups": []string{"admins", "developers"},
	}
	token := buildTestJWT(claims)

	decoded, err := DecodeJWTUnverified(token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	groups, ok := decoded["groups"].([]interface{})
	if !ok {
		t.Fatal("expected groups to be a slice")
	}
	if len(groups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(groups))
	}
}

func TestDecodeJWTUnverified_RealWorldFormat(t *testing.T) {
	// Test with a 3-segment token that has a non-empty signature segment
	claims := map[string]interface{}{
		"sub": "auth0|12345",
		"exp": float64(9999999999),
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload, _ := json.Marshal(claims)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	// Add a fake signature segment
	token := header + "." + payloadB64 + "." + strings.Repeat("a", 86)

	decoded, err := DecodeJWTUnverified(token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decoded["sub"] != "auth0|12345" {
		t.Errorf("expected sub=auth0|12345, got %v", decoded["sub"])
	}
}

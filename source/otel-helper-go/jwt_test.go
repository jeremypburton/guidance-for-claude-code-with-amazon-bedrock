package main

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

// makeJWT creates a minimal JWT from a claims map (no signature validation needed).
func makeJWT(claims map[string]interface{}) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload, _ := json.Marshal(claims)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	sig := base64.RawURLEncoding.EncodeToString([]byte("fake-signature"))
	return header + "." + payloadB64 + "." + sig
}

func TestDecodeJWTPayload_Valid(t *testing.T) {
	token := makeJWT(map[string]interface{}{
		"sub":   "user-123",
		"email": "test@example.com",
		"iss":   "https://dev.okta.com/oauth2",
	})

	result := decodeJWTPayload(token)

	if result["sub"] != "user-123" {
		t.Errorf("expected sub=user-123, got %v", result["sub"])
	}
	if result["email"] != "test@example.com" {
		t.Errorf("expected email=test@example.com, got %v", result["email"])
	}
}

func TestDecodeJWTPayload_MalformedToken(t *testing.T) {
	result := decodeJWTPayload("not-a-jwt")
	if len(result) != 0 {
		t.Errorf("expected empty map for malformed token, got %v", result)
	}
}

func TestDecodeJWTPayload_BadBase64(t *testing.T) {
	result := decodeJWTPayload("header.!!!invalid!!!.signature")
	if len(result) != 0 {
		t.Errorf("expected empty map for bad base64, got %v", result)
	}
}

func TestDecodeJWTPayload_InvalidJSON(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte("not json"))
	token := "header." + payload + ".sig"
	result := decodeJWTPayload(token)
	if len(result) != 0 {
		t.Errorf("expected empty map for invalid JSON, got %v", result)
	}
}

func TestDecodeJWTPayload_EmptyToken(t *testing.T) {
	result := decodeJWTPayload("")
	if len(result) != 0 {
		t.Errorf("expected empty map for empty token, got %v", result)
	}
}

func TestRedactClaims(t *testing.T) {
	claims := map[string]interface{}{
		"email":   "alice@example.com",
		"sub":     "user-123",
		"at_hash": "abc",
		"nonce":   "xyz",
		"iss":     "https://example.com",
	}

	redacted := redactClaims(claims)

	// Sensitive fields should be redacted
	if redacted["email"] != "<email-redacted>" {
		t.Errorf("email not redacted: %v", redacted["email"])
	}
	if redacted["sub"] != "<sub-redacted>" {
		t.Errorf("sub not redacted: %v", redacted["sub"])
	}

	// Non-sensitive field should be preserved
	if redacted["iss"] != "https://example.com" {
		t.Errorf("iss should not be redacted: %v", redacted["iss"])
	}

	// Original should not be modified
	if claims["email"] != "alice@example.com" {
		t.Error("original claims modified by redactClaims")
	}
}

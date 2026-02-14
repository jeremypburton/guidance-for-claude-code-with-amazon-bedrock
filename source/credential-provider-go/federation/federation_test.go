package federation

import (
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func TestBuildSessionName_FromSub(t *testing.T) {
	claims := jwt.MapClaims{"sub": "user123"}
	name := buildSessionName(claims)
	if name != "claude-code-user123" {
		t.Errorf("expected claude-code-user123, got %s", name)
	}
}

func TestBuildSessionName_FromEmail(t *testing.T) {
	claims := jwt.MapClaims{"email": "john.doe@example.com"}
	name := buildSessionName(claims)
	if name != "claude-code-john.doe" {
		t.Errorf("expected claude-code-john.doe, got %s", name)
	}
}

func TestBuildSessionName_Default(t *testing.T) {
	claims := jwt.MapClaims{}
	name := buildSessionName(claims)
	if name != "claude-code" {
		t.Errorf("expected claude-code, got %s", name)
	}
}

func TestBuildSessionName_SanitizesPipeChars(t *testing.T) {
	// Auth0 uses pipe-delimited sub like "auth0|12345"
	claims := jwt.MapClaims{"sub": "auth0|12345"}
	name := buildSessionName(claims)
	if name != "claude-code-auth0-12345" {
		t.Errorf("expected claude-code-auth0-12345, got %s", name)
	}
}

func TestBuildSessionName_TruncatesLongSub(t *testing.T) {
	claims := jwt.MapClaims{"sub": "abcdefghijklmnopqrstuvwxyz1234567890extra"}
	name := buildSessionName(claims)
	// sub should be truncated to 32 chars before prefixing
	expected := "claude-code-abcdefghijklmnopqrstuvwxyz123456"
	if name != expected {
		t.Errorf("expected %s, got %s", expected, name)
	}
}

func TestBuildSessionName_SubPreferredOverEmail(t *testing.T) {
	claims := jwt.MapClaims{
		"sub":   "user123",
		"email": "user@example.com",
	}
	name := buildSessionName(claims)
	if name != "claude-code-user123" {
		t.Errorf("expected sub to take precedence, got %s", name)
	}
}

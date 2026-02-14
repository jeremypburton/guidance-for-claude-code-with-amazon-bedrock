package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"

	"credential-provider-go/provider"
)

func TestGeneratePKCE(t *testing.T) {
	pkce, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verifier should be base64url encoded 32 bytes
	if len(pkce.Verifier) == 0 {
		t.Error("empty verifier")
	}

	// Verifier should not have padding
	if strings.Contains(pkce.Verifier, "=") {
		t.Error("verifier should not contain padding")
	}

	// Challenge should be SHA256 of verifier, base64url encoded without padding
	h := sha256.Sum256([]byte(pkce.Verifier))
	expectedChallenge := base64.RawURLEncoding.EncodeToString(h[:])
	if pkce.Challenge != expectedChallenge {
		t.Errorf("challenge mismatch: got %s, expected %s", pkce.Challenge, expectedChallenge)
	}

	// Challenge should not have padding
	if strings.Contains(pkce.Challenge, "=") {
		t.Error("challenge should not contain padding")
	}
}

func TestGeneratePKCE_Uniqueness(t *testing.T) {
	pkce1, _ := GeneratePKCE()
	pkce2, _ := GeneratePKCE()

	if pkce1.Verifier == pkce2.Verifier {
		t.Error("two PKCE generations should produce different verifiers")
	}
}

func TestGenerateState(t *testing.T) {
	s, err := GenerateState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s) == 0 {
		t.Error("empty state")
	}
	if strings.Contains(s, "=") {
		t.Error("state should not contain padding")
	}
}

func TestGenerateNonce(t *testing.T) {
	n, err := GenerateNonce()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(n) == 0 {
		t.Error("empty nonce")
	}
}

func TestBuildAuthURL(t *testing.T) {
	cfg := provider.ProviderConfigs["okta"]
	url := BuildAuthURL("mycompany.okta.com", "okta", cfg,
		"client123", "http://localhost:8400/callback", "state123", "nonce123", "challenge123")

	if !strings.HasPrefix(url, "https://mycompany.okta.com/oauth2/v1/authorize?") {
		t.Errorf("unexpected URL prefix: %s", url)
	}
	if !strings.Contains(url, "client_id=client123") {
		t.Error("URL missing client_id")
	}
	if !strings.Contains(url, "code_challenge=challenge123") {
		t.Error("URL missing code_challenge")
	}
	if !strings.Contains(url, "code_challenge_method=S256") {
		t.Error("URL missing code_challenge_method")
	}
	if !strings.Contains(url, "state=state123") {
		t.Error("URL missing state")
	}
}

func TestBuildAuthURL_Azure(t *testing.T) {
	cfg := provider.ProviderConfigs["azure"]
	url := BuildAuthURL("login.microsoftonline.com/tenant-id/v2.0", "azure", cfg,
		"client123", "http://localhost:8400/callback", "state123", "nonce123", "challenge123")

	// Should strip /v2.0 and add the endpoint
	if !strings.HasPrefix(url, "https://login.microsoftonline.com/tenant-id/oauth2/v2.0/authorize?") {
		t.Errorf("unexpected Azure URL: %s", url)
	}
	// Azure-specific params
	if !strings.Contains(url, "prompt=select_account") {
		t.Error("Azure URL missing prompt=select_account")
	}
	if !strings.Contains(url, "response_mode=query") {
		t.Error("Azure URL missing response_mode=query")
	}
}

func TestBuildAuthURL_Cognito(t *testing.T) {
	cfg := provider.ProviderConfigs["cognito"]
	url := BuildAuthURL("myapp.auth.us-east-1.amazoncognito.com", "cognito", cfg,
		"client123", "http://localhost:8400/callback", "state123", "nonce123", "challenge123")

	if !strings.HasPrefix(url, "https://myapp.auth.us-east-1.amazoncognito.com/oauth2/authorize?") {
		t.Errorf("unexpected Cognito URL: %s", url)
	}
	// Cognito uses "openid email" scopes
	if !strings.Contains(url, "scope=openid+email") && !strings.Contains(url, "scope=openid%20email") {
		t.Error("Cognito URL missing expected scopes")
	}
}

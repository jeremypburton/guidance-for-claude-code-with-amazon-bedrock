package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

// fakeJWT creates a minimal unsigned JWT with the given claims JSON for testing.
func fakeJWT(claimsJSON string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(claimsJSON))
	return header + "." + payload + "."
}

// withHTTPScheme sets urlScheme to "http" for the duration of a test, then restores it.
func withHTTPScheme(t *testing.T) {
	t.Helper()
	orig := urlScheme
	urlScheme = "http"
	t.Cleanup(func() { urlScheme = orig })
}

func TestExchangeCodeForTokens_Success(t *testing.T) {
	withHTTPScheme(t)

	jwt := fakeJWT(`{"sub":"user1","email":"u@test.com","exp":9999999999}`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.FormValue("grant_type") != "authorization_code" {
			t.Errorf("expected grant_type=authorization_code, got %s", r.FormValue("grant_type"))
		}
		if r.FormValue("code") != "authcode123" {
			t.Errorf("expected code=authcode123, got %s", r.FormValue("code"))
		}
		if r.FormValue("client_id") != "my-client" {
			t.Errorf("expected client_id=my-client, got %s", r.FormValue("client_id"))
		}
		if r.FormValue("code_verifier") != "verifier123" {
			t.Errorf("expected code_verifier=verifier123, got %s", r.FormValue("code_verifier"))
		}

		resp := map[string]string{
			"id_token":      jwt,
			"refresh_token": "rt-from-exchange",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	cfg := provider.Config{TokenEndpoint: "/token"}

	result, err := ExchangeCodeForTokens(host, "okta", cfg, "my-client", "http://localhost/cb", "authcode123", "verifier123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IDToken != jwt {
		t.Error("IDToken mismatch")
	}
	if result.RefreshToken != "rt-from-exchange" {
		t.Errorf("expected refresh token 'rt-from-exchange', got '%s'", result.RefreshToken)
	}
	if result.TokenClaims["email"] != "u@test.com" {
		t.Errorf("expected email claim 'u@test.com', got '%v'", result.TokenClaims["email"])
	}
}

func TestExchangeCodeForTokens_ErrorResponse(t *testing.T) {
	withHTTPScheme(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":"invalid_grant"}`)
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	cfg := provider.Config{TokenEndpoint: "/token"}

	_, err := ExchangeCodeForTokens(host, "okta", cfg, "client", "redirect", "code", "verifier")
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if !strings.Contains(err.Error(), "status 400") {
		t.Errorf("expected status 400 in error, got: %v", err)
	}
}

func TestRefreshTokens_Success(t *testing.T) {
	withHTTPScheme(t)

	jwt := fakeJWT(`{"sub":"user1","email":"u@test.com","exp":9999999999}`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.FormValue("grant_type") != "refresh_token" {
			t.Errorf("expected grant_type=refresh_token, got %s", r.FormValue("grant_type"))
		}
		if r.FormValue("refresh_token") != "my-refresh-token" {
			t.Errorf("expected refresh_token=my-refresh-token, got %s", r.FormValue("refresh_token"))
		}
		if r.FormValue("client_id") != "my-client" {
			t.Errorf("expected client_id=my-client, got %s", r.FormValue("client_id"))
		}

		resp := map[string]string{
			"id_token":      jwt,
			"refresh_token": "rotated-refresh-token",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	cfg := provider.Config{TokenEndpoint: "/token"}

	result, err := RefreshTokens(host, "okta", cfg, "my-client", "my-refresh-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IDToken != jwt {
		t.Error("IDToken mismatch")
	}
	if result.RefreshToken != "rotated-refresh-token" {
		t.Errorf("expected rotated refresh token, got '%s'", result.RefreshToken)
	}
	if result.TokenClaims["email"] != "u@test.com" {
		t.Errorf("expected email claim, got '%v'", result.TokenClaims["email"])
	}
}

func TestRefreshTokens_NoRotation_KeepsOriginal(t *testing.T) {
	withHTTPScheme(t)

	jwt := fakeJWT(`{"sub":"user1","exp":9999999999}`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Provider does NOT return a new refresh_token (no rotation)
		resp := map[string]string{
			"id_token": jwt,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	cfg := provider.Config{TokenEndpoint: "/token"}

	result, err := RefreshTokens(host, "okta", cfg, "client", "original-refresh-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RefreshToken != "original-refresh-token" {
		t.Errorf("expected original refresh token preserved, got '%s'", result.RefreshToken)
	}
}

func TestRefreshTokens_ErrorResponse(t *testing.T) {
	withHTTPScheme(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":"invalid_grant","error_description":"refresh token expired"}`)
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	cfg := provider.Config{TokenEndpoint: "/token"}

	_, err := RefreshTokens(host, "okta", cfg, "client", "old-refresh-token")
	if err == nil {
		t.Fatal("expected error for expired refresh token")
	}
	if !strings.Contains(err.Error(), "status 400") {
		t.Errorf("expected status 400 in error, got: %v", err)
	}
}

func TestBuildProviderURL_Azure(t *testing.T) {
	// Azure domains with /v2.0 should have it stripped
	url := buildProviderURL("login.microsoftonline.com/tenant-id/v2.0", "azure", "/oauth2/v2.0/token")
	expected := urlScheme + "://login.microsoftonline.com/tenant-id/oauth2/v2.0/token"
	if url != expected {
		t.Errorf("expected %s, got %s", expected, url)
	}
}

func TestBuildProviderURL_NonAzure(t *testing.T) {
	url := buildProviderURL("mycompany.okta.com", "okta", "/oauth2/v1/token")
	expected := urlScheme + "://mycompany.okta.com/oauth2/v1/token"
	if url != expected {
		t.Errorf("expected %s, got %s", expected, url)
	}
}

func TestBuildAuthURL_IncludesOfflineAccess(t *testing.T) {
	for name, cfg := range provider.ProviderConfigs {
		t.Run(name, func(t *testing.T) {
			url := BuildAuthURL("example.com", name, cfg,
				"client", "http://localhost/cb", "state", "nonce", "challenge")
			if !strings.Contains(url, "offline_access") {
				t.Errorf("provider %s URL missing offline_access scope", name)
			}
		})
	}
}

func TestStartCallbackServer_ReturnsCode(t *testing.T) {
	t.Setenv("CCWB_BIND_ADDRESS", "127.0.0.1")
	port := 48910

	go func() {
		// Give the server a moment to start
		time.Sleep(50 * time.Millisecond)
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/callback?state=test-state&code=auth-code-123", port))
		if err != nil {
			t.Errorf("callback request failed: %v", err)
			return
		}
		resp.Body.Close()
	}()

	code, err := StartCallbackServer(port, "test-state", 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != "auth-code-123" {
		t.Errorf("expected code 'auth-code-123', got '%s'", code)
	}
}

func TestStartCallbackServer_StateMismatch(t *testing.T) {
	t.Setenv("CCWB_BIND_ADDRESS", "127.0.0.1")
	port := 48911

	go func() {
		time.Sleep(50 * time.Millisecond)
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/callback?state=wrong-state&code=auth-code", port))
		if err != nil {
			t.Errorf("callback request failed: %v", err)
			return
		}
		resp.Body.Close()
	}()

	_, err := StartCallbackServer(port, "correct-state", 5*time.Second)
	if err == nil {
		t.Fatal("expected error for state mismatch")
	}
	if !strings.Contains(err.Error(), "invalid state") {
		t.Errorf("expected 'invalid state' in error, got: %v", err)
	}
}

func TestStartCallbackServer_ErrorParam(t *testing.T) {
	t.Setenv("CCWB_BIND_ADDRESS", "127.0.0.1")
	port := 48912

	go func() {
		time.Sleep(50 * time.Millisecond)
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/callback?error=access_denied&error_description=user+denied", port))
		if err != nil {
			t.Errorf("callback request failed: %v", err)
			return
		}
		resp.Body.Close()
	}()

	_, err := StartCallbackServer(port, "any-state", 5*time.Second)
	if err == nil {
		t.Fatal("expected error for error param")
	}
	if !strings.Contains(err.Error(), "user denied") {
		t.Errorf("expected 'user denied' in error, got: %v", err)
	}
}

func TestStartCallbackServer_Timeout(t *testing.T) {
	t.Setenv("CCWB_BIND_ADDRESS", "127.0.0.1")
	port := 48913

	start := time.Now()
	_, err := StartCallbackServer(port, "state", 500*time.Millisecond)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("expected 'timeout' in error, got: %v", err)
	}
	if elapsed < 400*time.Millisecond {
		t.Errorf("timeout too early: %v", elapsed)
	}
}

func TestStartCallbackServer_BindAddressOverride(t *testing.T) {
	t.Setenv("CCWB_BIND_ADDRESS", "0.0.0.0")
	port := 48914

	go func() {
		time.Sleep(50 * time.Millisecond)
		// Connect via 127.0.0.1 — should work because server is on 0.0.0.0
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/callback?state=s1&code=c1", port))
		if err != nil {
			t.Errorf("callback request failed: %v", err)
			return
		}
		resp.Body.Close()
	}()

	code, err := StartCallbackServer(port, "s1", 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != "c1" {
		t.Errorf("expected code 'c1', got '%s'", code)
	}
}

func TestBuildAuthURL_Cognito(t *testing.T) {
	cfg := provider.ProviderConfigs["cognito"]
	url := BuildAuthURL("myapp.auth.us-east-1.amazoncognito.com", "cognito", cfg,
		"client123", "http://localhost:8400/callback", "state123", "nonce123", "challenge123")

	if !strings.HasPrefix(url, "https://myapp.auth.us-east-1.amazoncognito.com/oauth2/authorize?") {
		t.Errorf("unexpected Cognito URL: %s", url)
	}
	if !strings.Contains(url, "offline_access") {
		t.Error("Cognito URL missing offline_access scope")
	}
	if !strings.Contains(url, "openid") {
		t.Error("Cognito URL missing openid scope")
	}
}

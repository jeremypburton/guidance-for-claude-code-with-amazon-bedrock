package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"credential-provider-go/internal"
	"credential-provider-go/provider"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/pkg/browser"
)

// OAuthResult holds the tokens returned from an OIDC authentication flow.
type OAuthResult struct {
	IDToken     string
	TokenClaims jwtlib.MapClaims
}

// PKCEParams holds the PKCE code verifier and challenge.
type PKCEParams struct {
	Verifier  string
	Challenge string
}

// GeneratePKCE creates a PKCE code verifier and S256 challenge.
func GeneratePKCE() (*PKCEParams, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}
	verifier := base64.RawURLEncoding.EncodeToString(b)

	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])

	return &PKCEParams{Verifier: verifier, Challenge: challenge}, nil
}

// GenerateState creates a random state parameter for OAuth.
func GenerateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// GenerateNonce creates a random nonce for OIDC.
func GenerateNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// callbackResult is passed through a channel from the HTTP callback handler.
type callbackResult struct {
	Code  string
	Error string
}

// StartCallbackServer starts an HTTP server on the given port and waits for
// the OAuth callback. It returns the authorization code or an error.
func StartCallbackServer(port int, expectedState string, timeout time.Duration) (string, error) {
	resultCh := make(chan callbackResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()

		if errParam := query.Get("error"); errParam != "" {
			desc := query.Get("error_description")
			if desc == "" {
				desc = "Unknown error"
			}
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, htmlPage("Authentication failed", desc))
			resultCh <- callbackResult{Error: desc}
			return
		}

		state := query.Get("state")
		code := query.Get("code")

		if state == expectedState && code != "" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, htmlPage("Authentication successful!", "You can close this window."))
			resultCh <- callbackResult{Code: code}
			return
		}

		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, htmlPage("Invalid response", "State mismatch or missing code."))
		resultCh <- callbackResult{Error: "invalid state or missing code"}
	})

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return "", fmt.Errorf("failed to listen on port %d: %w", port, err)
	}

	server := &http.Server{Handler: mux}

	go func() {
		_ = server.Serve(listener)
	}()

	select {
	case res := <-resultCh:
		time.Sleep(100 * time.Millisecond)
		_ = server.Close()
		if res.Error != "" {
			return "", fmt.Errorf("authentication error: %s", res.Error)
		}
		return res.Code, nil
	case <-time.After(timeout):
		_ = server.Close()
		return "", fmt.Errorf("authentication timeout - no authorization code received within %v", timeout)
	}
}

// BuildAuthURL constructs the OIDC authorization URL.
func BuildAuthURL(providerDomain, providerType string, providerCfg provider.Config,
	clientID, redirectURI, state, nonce, codeChallenge string) string {

	domain := providerDomain
	if providerType == "azure" && strings.HasSuffix(domain, "/v2.0") {
		domain = strings.TrimSuffix(domain, "/v2.0")
	}

	baseURL := "https://" + domain

	params := url.Values{
		"client_id":             {clientID},
		"response_type":        {providerCfg.ResponseType},
		"scope":                {providerCfg.Scopes},
		"redirect_uri":         {redirectURI},
		"state":                {state},
		"nonce":                {nonce},
		"code_challenge_method": {"S256"},
		"code_challenge":       {codeChallenge},
	}

	if providerType == "azure" {
		params.Set("response_mode", "query")
		params.Set("prompt", "select_account")
	}

	return baseURL + providerCfg.AuthorizeEndpoint + "?" + params.Encode()
}

// ExchangeCodeForTokens exchanges an authorization code for tokens.
func ExchangeCodeForTokens(providerDomain, providerType string, providerCfg provider.Config,
	clientID, redirectURI, code, codeVerifier string) (*OAuthResult, error) {

	domain := providerDomain
	if providerType == "azure" && strings.HasSuffix(domain, "/v2.0") {
		domain = strings.TrimSuffix(domain, "/v2.0")
	}

	tokenURL := "https://" + domain + providerCfg.TokenEndpoint

	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {clientID},
		"code_verifier": {codeVerifier},
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(tokenURL, "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed (status %d): %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		IDToken string `json:"id_token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	if tokenResp.IDToken == "" {
		return nil, fmt.Errorf("no id_token in token response")
	}

	claims, err := internal.DecodeJWTUnverified(tokenResp.IDToken)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ID token: %w", err)
	}

	return &OAuthResult{
		IDToken:     tokenResp.IDToken,
		TokenClaims: claims,
	}, nil
}

// OpenBrowser opens the system browser to the given URL.
func OpenBrowser(url string) error {
	return browser.OpenURL(url)
}

func htmlPage(title, body string) string {
	return fmt.Sprintf(`<html>
<head><title>Authentication</title></head>
<body style="font-family: sans-serif; text-align: center; padding: 50px;">
    <h1>%s</h1>
    <p>%s</p>
    <p>Return to your terminal to continue.</p>
</body>
</html>`, title, body)
}

package monitoring

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"credential-provider-go/internal"

	"github.com/golang-jwt/jwt/v5"
)

// tokenData is the JSON structure stored in the monitoring token file.
type tokenData struct {
	Token   string `json:"token"`
	Expires int64  `json:"expires"`
	Email   string `json:"email"`
	Profile string `json:"profile"`
}

// SaveMonitoringToken saves the ID token for monitoring authentication.
func SaveMonitoringToken(idToken string, claims jwt.MapClaims, profile string) {
	exp := int64(0)
	if v, ok := claims["exp"].(float64); ok {
		exp = int64(v)
	}
	email := ""
	if v, ok := claims["email"].(string); ok {
		email = v
	}

	data := tokenData{
		Token:   idToken,
		Expires: exp,
		Email:   email,
		Profile: profile,
	}

	home, err := os.UserHomeDir()
	if err != nil {
		internal.DebugPrint("Warning: Could not save monitoring token: %v", err)
		return
	}

	sessionDir := filepath.Join(home, ".claude-code-session")
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		internal.DebugPrint("Warning: Could not create session directory: %v", err)
		return
	}

	tokenFile := filepath.Join(sessionDir, profile+"-monitoring.json")
	jsonData, err := json.Marshal(data)
	if err != nil {
		internal.DebugPrint("Warning: Could not marshal monitoring token: %v", err)
		return
	}

	if err := os.WriteFile(tokenFile, jsonData, 0600); err != nil {
		internal.DebugPrint("Warning: Could not write monitoring token: %v", err)
		return
	}

	// Also set environment variable for this session
	os.Setenv("CLAUDE_CODE_MONITORING_TOKEN", idToken)

	internal.DebugPrint("Saved monitoring token for %s", email)
}

// GetMonitoringToken retrieves a valid monitoring token from storage.
// Returns empty string if no valid token is available.
func GetMonitoringToken(profile string) string {
	// Check environment first
	if token := os.Getenv("CLAUDE_CODE_MONITORING_TOKEN"); token != "" {
		return token
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	tokenFile := filepath.Join(home, ".claude-code-session", profile+"-monitoring.json")
	raw, err := os.ReadFile(tokenFile)
	if err != nil {
		return ""
	}

	var data tokenData
	if err := json.Unmarshal(raw, &data); err != nil {
		return ""
	}

	// Check expiration: return token if it expires in more than 600 seconds (10 minutes)
	now := time.Now().UTC().Unix()
	if data.Expires-now > 600 {
		os.Setenv("CLAUDE_CODE_MONITORING_TOKEN", data.Token)
		return data.Token
	}

	return ""
}

// GetCachedTokenClaims returns basic claims from the cached monitoring token.
func GetCachedTokenClaims(profile string) map[string]string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	tokenFile := filepath.Join(home, ".claude-code-session", profile+"-monitoring.json")
	raw, err := os.ReadFile(tokenFile)
	if err != nil {
		return nil
	}

	var data tokenData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil
	}

	return map[string]string{
		"email": data.Email,
	}
}

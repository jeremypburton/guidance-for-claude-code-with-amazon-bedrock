package quota

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"credential-provider-go/internal"

	jwtlib "github.com/golang-jwt/jwt/v5"
)

// Result holds the response from the quota check API.
type Result struct {
	Allowed bool              `json:"allowed"`
	Reason  string            `json:"reason"`
	Message string            `json:"message"`
	Usage   map[string]interface{} `json:"usage"`
	Policy  map[string]interface{} `json:"policy"`
}

// ShouldCheck returns true if quota checking is configured.
func ShouldCheck(quotaAPIEndpoint string) bool {
	return quotaAPIEndpoint != ""
}

// ShouldRecheck returns true if it's time for a periodic quota re-check.
func ShouldRecheck(quotaAPIEndpoint string, intervalMinutes int, profile string) bool {
	if !ShouldCheck(quotaAPIEndpoint) {
		return false
	}
	if intervalMinutes == 0 {
		return true
	}

	lastCheck := getLastQuotaCheckTime(profile)
	if lastCheck.IsZero() {
		return true
	}

	elapsed := time.Since(lastCheck).Minutes()
	internal.DebugPrint("Quota check: %.1f min since last check, interval=%d min", elapsed, intervalMinutes)
	return elapsed >= float64(intervalMinutes)
}

// Check performs a quota check by calling the quota API with the Bearer JWT.
func Check(endpoint string, idToken string, claims jwtlib.MapClaims, failMode string, timeout int) *Result {
	email, _ := claims["email"].(string)
	if email == "" {
		internal.DebugPrint("No email in token claims, skipping quota check")
		return &Result{Allowed: true, Reason: "no_email"}
	}

	groups := extractGroups(claims)
	internal.DebugPrint("Checking quota for %s (groups: %v)", email, groups)

	client := &http.Client{Timeout: time.Duration(timeout) * time.Second}
	req, err := http.NewRequest("GET", endpoint+"/check", nil)
	if err != nil {
		internal.DebugPrint("Quota check request creation failed: %v", err)
		return failResult(failMode, "error", fmt.Sprintf("Quota check failed: %v", err))
	}
	req.Header.Set("Authorization", "Bearer "+idToken)

	resp, err := client.Do(req)
	if err != nil {
		if isTimeout(err) {
			internal.DebugPrint("Quota check timed out")
			return failResult(failMode, "timeout", "Quota check timed out. Please try again.")
		}
		internal.DebugPrint("Quota check request failed: %v", err)
		return failResult(failMode, "connection_error", fmt.Sprintf("Could not connect to quota service: %v", err))
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == 200 {
		var result Result
		if err := json.Unmarshal(body, &result); err != nil {
			internal.DebugPrint("Quota check: failed to parse response: %v", err)
			return failResult(failMode, "error", "Quota check: invalid response format")
		}
		internal.DebugPrint("Quota check result: allowed=%v, reason=%s", result.Allowed, result.Reason)
		return &result
	}

	if resp.StatusCode == 401 {
		internal.DebugPrint("Quota check JWT validation failed (401)")
		return failResult(failMode, "jwt_invalid", "Quota check authentication failed - invalid or expired token")
	}

	internal.DebugPrint("Quota check returned status %d", resp.StatusCode)
	return failResult(failMode, "api_error", fmt.Sprintf("Quota check failed with status %d", resp.StatusCode))
}

// SaveQuotaCheckTimestamp records the current time as the last quota check time.
func SaveQuotaCheckTimestamp(profile string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	sessionDir := filepath.Join(home, ".claude-code-session")
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		return
	}

	data := map[string]string{"last_check": time.Now().UTC().Format(time.RFC3339)}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return
	}

	file := filepath.Join(sessionDir, profile+"-quota-check.json")
	os.WriteFile(file, jsonData, 0600)
	internal.DebugPrint("Saved quota check timestamp")
}

func getLastQuotaCheckTime(profile string) time.Time {
	home, err := os.UserHomeDir()
	if err != nil {
		return time.Time{}
	}

	file := filepath.Join(home, ".claude-code-session", profile+"-quota-check.json")
	raw, err := os.ReadFile(file)
	if err != nil {
		return time.Time{}
	}

	var data map[string]string
	if err := json.Unmarshal(raw, &data); err != nil {
		return time.Time{}
	}

	t, err := time.Parse(time.RFC3339, data["last_check"])
	if err != nil {
		return time.Time{}
	}
	return t
}

func extractGroups(claims jwtlib.MapClaims) []string {
	seen := make(map[string]bool)
	var groups []string

	addGroups := func(key string) {
		val, ok := claims[key]
		if !ok {
			return
		}
		switch v := val.(type) {
		case []interface{}:
			for _, item := range v {
				if s, ok := item.(string); ok && !seen[s] {
					seen[s] = true
					groups = append(groups, s)
				}
			}
		case string:
			if !seen[v] {
				seen[v] = true
				groups = append(groups, v)
			}
		}
	}

	addGroups("groups")
	addGroups("cognito:groups")

	if dept, ok := claims["custom:department"].(string); ok && dept != "" {
		g := "department:" + dept
		if !seen[g] {
			groups = append(groups, g)
		}
	}

	return groups
}

func failResult(failMode, reason, message string) *Result {
	if failMode == "closed" {
		return &Result{Allowed: false, Reason: reason, Message: message}
	}
	return &Result{Allowed: true, Reason: reason}
}

func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	// net.Error has a Timeout() method
	type timeoutErr interface {
		Timeout() bool
	}
	if te, ok := err.(timeoutErr); ok {
		return te.Timeout()
	}
	return false
}

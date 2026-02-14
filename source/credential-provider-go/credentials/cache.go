package credentials

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"credential-provider-go/internal"

	"gopkg.in/ini.v1"
)

// ReadFromCredentialsFile reads cached credentials from ~/.aws/credentials.
func ReadFromCredentialsFile(profile string) *AWSCredentialOutput {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	credPath := filepath.Join(home, ".aws", "credentials")
	if _, err := os.Stat(credPath); os.IsNotExist(err) {
		return nil
	}

	cfg, err := ini.LoadSources(ini.LoadOptions{
		// Preserve inline comments/special chars in values
		IgnoreInlineComment: true,
	}, credPath)
	if err != nil {
		internal.DebugPrint("Error reading credentials file: %v", err)
		return nil
	}

	sec, err := cfg.GetSection(profile)
	if err != nil {
		return nil
	}

	accessKeyID := sec.Key("aws_access_key_id").String()
	secretAccessKey := sec.Key("aws_secret_access_key").String()
	sessionToken := sec.Key("aws_session_token").String()
	expiration := sec.Key("x-expiration").String()

	if accessKeyID == "" || secretAccessKey == "" || sessionToken == "" {
		return nil
	}

	return &AWSCredentialOutput{
		Version:         1,
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		SessionToken:    sessionToken,
		Expiration:      expiration,
	}
}

// SaveToCredentialsFile writes credentials to ~/.aws/credentials using atomic write.
func SaveToCredentialsFile(creds *AWSCredentialOutput, profile string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}

	credPath := filepath.Join(home, ".aws", "credentials")

	// Create ~/.aws if needed
	if err := os.MkdirAll(filepath.Dir(credPath), 0700); err != nil {
		return fmt.Errorf("failed to create .aws directory: %w", err)
	}

	cfg, err := ini.LoadSources(ini.LoadOptions{
		IgnoreInlineComment: true,
	}, credPath)
	if err != nil {
		// File might not exist yet, create new
		cfg = ini.Empty()
	}

	sec, err := cfg.GetSection(profile)
	if err != nil {
		sec, err = cfg.NewSection(profile)
		if err != nil {
			return fmt.Errorf("failed to create section '%s': %w", profile, err)
		}
	}

	sec.Key("aws_access_key_id").SetValue(creds.AccessKeyID)
	sec.Key("aws_secret_access_key").SetValue(creds.SecretAccessKey)
	sec.Key("aws_session_token").SetValue(creds.SessionToken)
	if creds.Expiration != "" {
		sec.Key("x-expiration").SetValue(creds.Expiration)
	}

	// Atomic write: write to temp file then rename
	tmpFile, err := os.CreateTemp(filepath.Dir(credPath), ".credentials.*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := cfg.WriteTo(tmpFile); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write credentials: %w", err)
	}
	tmpFile.Close()

	if err := os.Chmod(tmpPath, 0600); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	if err := os.Rename(tmpPath, credPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	internal.DebugPrint("Saved credentials to %s for profile '%s'", credPath, profile)
	return nil
}

// GetCachedCredentials retrieves valid (non-expired) credentials from the cache.
func GetCachedCredentials(profile string) *AWSCredentialOutput {
	creds := ReadFromCredentialsFile(profile)
	if creds == nil {
		return nil
	}

	// Check for dummy/cleared credentials
	if creds.AccessKeyID == "EXPIRED" {
		internal.DebugPrint("Found cleared dummy credentials in credentials file, need re-authentication")
		return nil
	}

	// Validate expiration
	if creds.Expiration == "" {
		return nil
	}

	expTime, err := parseExpiration(creds.Expiration)
	if err != nil {
		internal.DebugPrint("Error parsing expiration: %v", err)
		return nil
	}

	// 30-second buffer
	remaining := time.Until(expTime)
	if remaining <= 30*time.Second {
		return nil
	}

	return creds
}

// CheckExpiration returns true if credentials are expired or missing.
func CheckExpiration(profile string) bool {
	creds := ReadFromCredentialsFile(profile)
	if creds == nil {
		return true
	}

	if creds.Expiration == "" {
		return true
	}

	expTime, err := parseExpiration(creds.Expiration)
	if err != nil {
		internal.DebugPrint("Error parsing expiration: %v", err)
		return true
	}

	remaining := time.Until(expTime)
	return remaining <= 30*time.Second
}

// ClearCredentials replaces cached credentials with expired dummy data.
func ClearCredentials(profile string) []string {
	var cleared []string

	// Clear credentials file
	expired := &AWSCredentialOutput{
		Version:         1,
		AccessKeyID:     "EXPIRED",
		SecretAccessKey: "EXPIRED",
		SessionToken:    "EXPIRED",
		Expiration:      "2000-01-01T00:00:00Z",
	}

	if err := SaveToCredentialsFile(expired, profile); err == nil {
		cleared = append(cleared, "credentials file")
	}

	// Clear monitoring token file
	home, err := os.UserHomeDir()
	if err == nil {
		sessionDir := filepath.Join(home, ".claude-code-session")
		monFile := filepath.Join(sessionDir, profile+"-monitoring.json")
		if _, err := os.Stat(monFile); err == nil {
			if os.Remove(monFile) == nil {
				cleared = append(cleared, "monitoring token file")
			}
		}

		// Remove session dir if empty
		entries, err := os.ReadDir(sessionDir)
		if err == nil && len(entries) == 0 {
			os.Remove(sessionDir)
		}
	}

	return cleared
}

func parseExpiration(s string) (time.Time, error) {
	// Handle both "Z" and "+00:00" suffixes
	s = strings.Replace(s, "+00:00", "Z", 1)
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

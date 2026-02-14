package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"credential-provider-go/internal"
)

// ProfileConfig holds the configuration for a single authentication profile.
type ProfileConfig struct {
	ProviderDomain     string `json:"provider_domain"`
	ClientID           string `json:"client_id"`
	AWSRegion          string `json:"aws_region"`
	ProviderType       string `json:"provider_type"`
	CredentialStorage  string `json:"credential_storage"`
	FederationType     string `json:"federation_type"`
	IdentityPoolID     string `json:"identity_pool_id"`
	IdentityPoolName   string `json:"identity_pool_name"`
	FederatedRoleARN   string `json:"federated_role_arn"`
	RoleARN            string `json:"role_arn"`
	CognitoUserPoolID  string `json:"cognito_user_pool_id"`
	MaxSessionDuration int    `json:"max_session_duration"`

	// Quota fields
	QuotaAPIEndpoint   string `json:"quota_api_endpoint"`
	QuotaCheckInterval int    `json:"quota_check_interval"`
	QuotaFailMode      string `json:"quota_fail_mode"`
	QuotaCheckTimeout  int    `json:"quota_check_timeout"`

	// Cross-region
	CrossRegionProfile string `json:"cross_region_profile"`
	SelectedModel      string `json:"selected_model"`

	// Compatibility fields (old format)
	OktaDomain   string `json:"okta_domain"`
	OktaClientID string `json:"okta_client_id"`
}

// LoadConfig loads and validates the profile configuration from config.json.
// It searches for config.json next to the binary first, then falls back to
// ~/claude-code-with-bedrock/config.json.
func LoadConfig(profile string) (*ProfileConfig, error) {
	configPath, err := findConfigFile()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	return parseConfigData(data, profile)
}

// parseConfigData parses raw JSON config data and returns the profile config.
func parseConfigData(data []byte, profile string) (*ProfileConfig, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	var cfg ProfileConfig

	if _, hasProfiles := raw["profiles"]; hasProfiles {
		// New format: {"profiles": {"Name": {...}}}
		var wrapper struct {
			Profiles map[string]json.RawMessage `json:"profiles"`
		}
		if err := json.Unmarshal(data, &wrapper); err != nil {
			return nil, fmt.Errorf("failed to parse profiles: %w", err)
		}
		profileData, ok := wrapper.Profiles[profile]
		if !ok {
			return nil, fmt.Errorf("profile '%s' not found in configuration", profile)
		}
		if err := json.Unmarshal(profileData, &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse profile '%s': %w", profile, err)
		}
	} else {
		// Old format: {"Name": {...}}
		profileData, ok := raw[profile]
		if !ok {
			return nil, fmt.Errorf("profile '%s' not found in configuration", profile)
		}
		if err := json.Unmarshal(profileData, &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse profile '%s': %w", profile, err)
		}
	}

	// Map old field names to new ones
	if cfg.ProviderDomain == "" && cfg.OktaDomain != "" {
		cfg.ProviderDomain = cfg.OktaDomain
	}
	if cfg.ClientID == "" && cfg.OktaClientID != "" {
		cfg.ClientID = cfg.OktaClientID
	}

	// Handle identity_pool_name -> identity_pool_id (only if not direct STS mode)
	if cfg.IdentityPoolName != "" && cfg.FederatedRoleARN == "" && cfg.IdentityPoolID == "" {
		cfg.IdentityPoolID = cfg.IdentityPoolName
	}

	// Auto-detect federation type
	detectFederationType(&cfg)

	// Validate required fields
	var required []string
	if cfg.FederationType == "direct" {
		required = []string{"provider_domain", "client_id", "federated_role_arn"}
	} else {
		required = []string{"provider_domain", "client_id", "identity_pool_id"}
	}

	var missing []string
	for _, key := range required {
		switch key {
		case "provider_domain":
			if cfg.ProviderDomain == "" {
				missing = append(missing, key)
			}
		case "client_id":
			if cfg.ClientID == "" {
				missing = append(missing, key)
			}
		case "federated_role_arn":
			if cfg.FederatedRoleARN == "" {
				missing = append(missing, key)
			}
		case "identity_pool_id":
			if cfg.IdentityPoolID == "" {
				missing = append(missing, key)
			}
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required configuration: %s", joinStrings(missing, ", "))
	}

	// Set defaults
	if cfg.AWSRegion == "" {
		cfg.AWSRegion = "us-east-1"
	}
	if cfg.ProviderType == "" {
		cfg.ProviderType = "auto"
	}
	if cfg.CredentialStorage == "" {
		cfg.CredentialStorage = "session"
	}
	if cfg.MaxSessionDuration == 0 {
		if cfg.FederationType == "direct" {
			cfg.MaxSessionDuration = 43200
		} else {
			cfg.MaxSessionDuration = 28800
		}
	}
	if cfg.QuotaCheckInterval == 0 {
		cfg.QuotaCheckInterval = 30
	}
	if cfg.QuotaFailMode == "" {
		cfg.QuotaFailMode = "open"
	}
	if cfg.QuotaCheckTimeout == 0 {
		cfg.QuotaCheckTimeout = 5
	}

	return &cfg, nil
}

// AutoDetectProfile returns the profile name when only one profile exists in config.json.
func AutoDetectProfile() string {
	configPath, err := findConfigFile()
	if err != nil {
		return ""
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return ""
	}

	return detectProfileFromData(data)
}

// detectProfileFromData extracts the single profile name from raw config JSON.
func detectProfileFromData(data []byte) string {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return ""
	}

	var profileNames []string

	if _, hasProfiles := raw["profiles"]; hasProfiles {
		var wrapper struct {
			Profiles map[string]json.RawMessage `json:"profiles"`
		}
		if err := json.Unmarshal(data, &wrapper); err != nil {
			return ""
		}
		for name := range wrapper.Profiles {
			profileNames = append(profileNames, name)
		}
	} else {
		for name := range raw {
			profileNames = append(profileNames, name)
		}
	}

	if len(profileNames) == 1 {
		internal.DebugPrint("Auto-detected profile: %s", profileNames[0])
		return profileNames[0]
	}
	if len(profileNames) > 1 {
		internal.DebugPrint("Multiple profiles found: %v. Use --profile to specify.", profileNames)
	}
	return ""
}

func findConfigFile() (string, error) {
	// Try same directory as the binary first
	exe, err := os.Executable()
	if err == nil {
		binDir := filepath.Dir(exe)
		p := filepath.Join(binDir, "config.json")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	// Fall back to ~/claude-code-with-bedrock/config.json
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	p := filepath.Join(home, "claude-code-with-bedrock", "config.json")
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}

	return "", fmt.Errorf("configuration file not found next to binary or in %s",
		filepath.Join(home, "claude-code-with-bedrock"))
}

func detectFederationType(cfg *ProfileConfig) {
	if cfg.FederationType != "" {
		return
	}
	if cfg.FederatedRoleARN != "" {
		cfg.FederationType = "direct"
		internal.DebugPrint("Detected Direct STS federation mode (federated_role_arn found)")
	} else if cfg.IdentityPoolID != "" || cfg.IdentityPoolName != "" {
		cfg.FederationType = "cognito"
		internal.DebugPrint("Detected Cognito Identity Pool federation mode")
	} else {
		cfg.FederationType = "cognito"
		internal.DebugPrint("Defaulting to Cognito Identity Pool federation mode")
	}
}

func joinStrings(ss []string, sep string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

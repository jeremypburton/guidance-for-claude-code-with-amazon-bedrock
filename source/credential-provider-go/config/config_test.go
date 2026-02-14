package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_NewFormat(t *testing.T) {
	dir := t.TempDir()
	configJSON := `{
		"profiles": {
			"TestProfile": {
				"provider_domain": "test.okta.com",
				"client_id": "abc123",
				"aws_region": "us-west-2",
				"federated_role_arn": "arn:aws:iam::123456789012:role/TestRole",
				"max_session_duration": 3600
			}
		}
	}`
	configPath := filepath.Join(dir, "config.json")
	os.WriteFile(configPath, []byte(configJSON), 0644)

	// Override executable path to point to our temp dir
	origExec := os.Args[0]
	defer func() { os.Args[0] = origExec }()

	// Create a symlink or just test the loading directly
	cfg, err := loadConfigFromPath(configPath, "TestProfile")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ProviderDomain != "test.okta.com" {
		t.Errorf("expected provider_domain=test.okta.com, got %s", cfg.ProviderDomain)
	}
	if cfg.ClientID != "abc123" {
		t.Errorf("expected client_id=abc123, got %s", cfg.ClientID)
	}
	if cfg.AWSRegion != "us-west-2" {
		t.Errorf("expected aws_region=us-west-2, got %s", cfg.AWSRegion)
	}
	if cfg.FederationType != "direct" {
		t.Errorf("expected federation_type=direct, got %s", cfg.FederationType)
	}
	if cfg.MaxSessionDuration != 3600 {
		t.Errorf("expected max_session_duration=3600, got %d", cfg.MaxSessionDuration)
	}
}

func TestLoadConfig_OldFormat(t *testing.T) {
	dir := t.TempDir()
	configJSON := `{
		"MyProfile": {
			"provider_domain": "corp.auth0.com",
			"client_id": "xyz789",
			"identity_pool_id": "us-east-1:abc-def-123"
		}
	}`
	configPath := filepath.Join(dir, "config.json")
	os.WriteFile(configPath, []byte(configJSON), 0644)

	cfg, err := loadConfigFromPath(configPath, "MyProfile")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ProviderDomain != "corp.auth0.com" {
		t.Errorf("expected provider_domain=corp.auth0.com, got %s", cfg.ProviderDomain)
	}
	if cfg.FederationType != "cognito" {
		t.Errorf("expected federation_type=cognito, got %s", cfg.FederationType)
	}
	if cfg.IdentityPoolID != "us-east-1:abc-def-123" {
		t.Errorf("expected identity_pool_id, got %s", cfg.IdentityPoolID)
	}
}

func TestLoadConfig_OldFieldNames(t *testing.T) {
	dir := t.TempDir()
	configJSON := `{
		"profiles": {
			"Legacy": {
				"okta_domain": "legacy.okta.com",
				"okta_client_id": "old-client",
				"identity_pool_name": "us-east-1:pool-id"
			}
		}
	}`
	configPath := filepath.Join(dir, "config.json")
	os.WriteFile(configPath, []byte(configJSON), 0644)

	cfg, err := loadConfigFromPath(configPath, "Legacy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ProviderDomain != "legacy.okta.com" {
		t.Errorf("expected provider_domain=legacy.okta.com, got %s", cfg.ProviderDomain)
	}
	if cfg.ClientID != "old-client" {
		t.Errorf("expected client_id=old-client, got %s", cfg.ClientID)
	}
	if cfg.IdentityPoolID != "us-east-1:pool-id" {
		t.Errorf("expected identity_pool_id from name mapping, got %s", cfg.IdentityPoolID)
	}
}

func TestLoadConfig_MissingProfile(t *testing.T) {
	dir := t.TempDir()
	configJSON := `{"profiles": {"Exists": {"provider_domain": "x", "client_id": "y", "identity_pool_id": "z"}}}`
	configPath := filepath.Join(dir, "config.json")
	os.WriteFile(configPath, []byte(configJSON), 0644)

	_, err := loadConfigFromPath(configPath, "DoesNotExist")
	if err == nil {
		t.Error("expected error for missing profile")
	}
}

func TestLoadConfig_MissingRequiredFields(t *testing.T) {
	dir := t.TempDir()
	configJSON := `{"profiles": {"Bad": {"provider_domain": "x.okta.com"}}}`
	configPath := filepath.Join(dir, "config.json")
	os.WriteFile(configPath, []byte(configJSON), 0644)

	_, err := loadConfigFromPath(configPath, "Bad")
	if err == nil {
		t.Error("expected error for missing required fields")
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	dir := t.TempDir()
	configJSON := `{
		"profiles": {
			"Defaults": {
				"provider_domain": "test.okta.com",
				"client_id": "abc",
				"identity_pool_id": "us-east-1:pool"
			}
		}
	}`
	configPath := filepath.Join(dir, "config.json")
	os.WriteFile(configPath, []byte(configJSON), 0644)

	cfg, err := loadConfigFromPath(configPath, "Defaults")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.AWSRegion != "us-east-1" {
		t.Errorf("expected default aws_region=us-east-1, got %s", cfg.AWSRegion)
	}
	if cfg.ProviderType != "auto" {
		t.Errorf("expected default provider_type=auto, got %s", cfg.ProviderType)
	}
	if cfg.CredentialStorage != "session" {
		t.Errorf("expected default credential_storage=session, got %s", cfg.CredentialStorage)
	}
	if cfg.MaxSessionDuration != 28800 {
		t.Errorf("expected default max_session_duration=28800 for cognito, got %d", cfg.MaxSessionDuration)
	}
}

func TestLoadConfig_DirectSTSDefaults(t *testing.T) {
	dir := t.TempDir()
	configJSON := `{
		"profiles": {
			"Direct": {
				"provider_domain": "test.okta.com",
				"client_id": "abc",
				"federated_role_arn": "arn:aws:iam::123:role/R"
			}
		}
	}`
	configPath := filepath.Join(dir, "config.json")
	os.WriteFile(configPath, []byte(configJSON), 0644)

	cfg, err := loadConfigFromPath(configPath, "Direct")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.MaxSessionDuration != 43200 {
		t.Errorf("expected default max_session_duration=43200 for direct, got %d", cfg.MaxSessionDuration)
	}
}

func TestLoadConfig_IdentityPoolNameNotConvertedForDirect(t *testing.T) {
	dir := t.TempDir()
	configJSON := `{
		"profiles": {
			"DirectWithName": {
				"provider_domain": "test.okta.com",
				"client_id": "abc",
				"federated_role_arn": "arn:aws:iam::123:role/R",
				"identity_pool_name": "should-not-become-pool-id"
			}
		}
	}`
	configPath := filepath.Join(dir, "config.json")
	os.WriteFile(configPath, []byte(configJSON), 0644)

	cfg, err := loadConfigFromPath(configPath, "DirectWithName")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// identity_pool_name should NOT be converted to identity_pool_id when federated_role_arn is present
	if cfg.IdentityPoolID != "" {
		t.Errorf("expected empty identity_pool_id for direct with role_arn, got %s", cfg.IdentityPoolID)
	}
}

func TestAutoDetectProfile_SingleProfile(t *testing.T) {
	dir := t.TempDir()
	configJSON := `{"profiles": {"OnlyOne": {"provider_domain": "x", "client_id": "y", "identity_pool_id": "z"}}}`
	configPath := filepath.Join(dir, "config.json")
	os.WriteFile(configPath, []byte(configJSON), 0644)

	result := autoDetectProfileFromPath(configPath)
	if result != "OnlyOne" {
		t.Errorf("expected OnlyOne, got %s", result)
	}
}

func TestAutoDetectProfile_MultipleProfiles(t *testing.T) {
	dir := t.TempDir()
	configJSON := `{"profiles": {"A": {}, "B": {}}}`
	configPath := filepath.Join(dir, "config.json")
	os.WriteFile(configPath, []byte(configJSON), 0644)

	result := autoDetectProfileFromPath(configPath)
	if result != "" {
		t.Errorf("expected empty string for multiple profiles, got %s", result)
	}
}

// loadConfigFromPath is a test helper that loads config from a specific path.
func loadConfigFromPath(configPath, profile string) (*ProfileConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	return parseConfigData(data, profile)
}

// autoDetectProfileFromPath is a test helper for auto-detection from a specific path.
func autoDetectProfileFromPath(configPath string) string {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return ""
	}
	return detectProfileFromData(data)
}

package provider

import (
	"testing"
)

func TestDetermineProviderType_ExplicitType(t *testing.T) {
	pt, err := DetermineProviderType("anything.com", "okta")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pt != "okta" {
		t.Errorf("expected okta, got %s", pt)
	}
}

func TestDetermineProviderType_AutoDetect(t *testing.T) {
	tests := []struct {
		domain   string
		expected string
	}{
		// Okta
		{"mycompany.okta.com", "okta"},
		{"https://mycompany.okta.com", "okta"},
		{"dev-123456.okta.com", "okta"},

		// Auth0
		{"myapp.auth0.com", "auth0"},
		{"https://myapp.auth0.com", "auth0"},

		// Azure / Microsoft
		{"login.microsoftonline.com", "azure"},
		{"https://login.microsoftonline.com/tenant-id", "azure"},
		{"sts.windows.net", "azure"},

		// JumpCloud
		{"oauth.id.jumpcloud.com", "jumpcloud"},
		{"https://oauth.id.jumpcloud.com", "jumpcloud"},

		// Cognito
		{"mydomain.auth.us-east-1.amazoncognito.com", "cognito"},
		{"https://mydomain.auth.us-west-2.amazoncognito.com", "cognito"},
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			pt, err := DetermineProviderType(tt.domain, "auto")
			if err != nil {
				t.Fatalf("unexpected error for %s: %v", tt.domain, err)
			}
			if pt != tt.expected {
				t.Errorf("for %s: expected %s, got %s", tt.domain, tt.expected, pt)
			}
		})
	}
}

func TestDetermineProviderType_EmptyDomain(t *testing.T) {
	_, err := DetermineProviderType("", "auto")
	if err == nil {
		t.Error("expected error for empty domain")
	}
}

func TestDetermineProviderType_UnknownDomain(t *testing.T) {
	_, err := DetermineProviderType("unknown-provider.example.com", "auto")
	if err == nil {
		t.Error("expected error for unknown domain")
	}
}

func TestDetermineProviderType_AutoStringDefault(t *testing.T) {
	// Empty provider type should behave like "auto"
	pt, err := DetermineProviderType("test.okta.com", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pt != "okta" {
		t.Errorf("expected okta, got %s", pt)
	}
}

func TestProviderConfigs_AllHaveRequiredFields(t *testing.T) {
	for name, cfg := range ProviderConfigs {
		if cfg.Name == "" {
			t.Errorf("provider %s has empty Name", name)
		}
		if cfg.AuthorizeEndpoint == "" {
			t.Errorf("provider %s has empty AuthorizeEndpoint", name)
		}
		if cfg.TokenEndpoint == "" {
			t.Errorf("provider %s has empty TokenEndpoint", name)
		}
		if cfg.Scopes == "" {
			t.Errorf("provider %s has empty Scopes", name)
		}
		if cfg.ResponseType != "code" {
			t.Errorf("provider %s has unexpected ResponseType: %s", name, cfg.ResponseType)
		}
	}
}

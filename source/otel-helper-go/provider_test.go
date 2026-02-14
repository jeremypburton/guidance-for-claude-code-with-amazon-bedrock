package main

import "testing"

func TestDetectProvider(t *testing.T) {
	tests := []struct {
		issuer   string
		expected string
	}{
		// Okta
		{"https://dev-123.okta.com/oauth2/default", "okta"},
		{"https://company.okta.com", "okta"},
		{"dev.okta.com", "okta"},

		// Auth0
		{"https://tenant.auth0.com/", "auth0"},
		{"tenant.auth0.com", "auth0"},

		// Azure
		{"https://login.microsoftonline.com/tenant-id/v2.0", "azure"},
		{"login.microsoftonline.com", "azure"},

		// JumpCloud
		{"https://oauth.id.jumpcloud.com", "jc_org"},
		{"oauth.id.jumpcloud.com", "jc_org"},

		// Cognito (falls through to default)
		{"https://cognito-idp.us-east-1.amazonaws.com/us-east-1_abc123", "amazon-internal"},

		// Unknown / default
		{"https://custom-idp.example.com", "amazon-internal"},
		{"", "amazon-internal"},

		// Domain-only inputs
		{"my-company.okta.com", "okta"},
		{"my-company.auth0.com", "auth0"},
	}

	for _, tt := range tests {
		t.Run(tt.issuer, func(t *testing.T) {
			result := detectProvider(tt.issuer)
			if result != tt.expected {
				t.Errorf("detectProvider(%q) = %q, want %q", tt.issuer, result, tt.expected)
			}
		})
	}
}

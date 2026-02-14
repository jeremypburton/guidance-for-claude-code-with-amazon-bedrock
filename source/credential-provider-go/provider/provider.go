package provider

import (
	"fmt"
	"net/url"
	"strings"
)

// Config holds the OIDC provider endpoint configuration.
type Config struct {
	Name              string
	AuthorizeEndpoint string
	TokenEndpoint     string
	Scopes            string
	ResponseType      string
	ResponseMode      string
}

// ProviderConfigs maps provider type names to their endpoint configurations.
var ProviderConfigs = map[string]Config{
	"okta": {
		Name:              "Okta",
		AuthorizeEndpoint: "/oauth2/v1/authorize",
		TokenEndpoint:     "/oauth2/v1/token",
		Scopes:            "openid profile email offline_access",
		ResponseType:      "code",
		ResponseMode:      "query",
	},
	"auth0": {
		Name:              "Auth0",
		AuthorizeEndpoint: "/authorize",
		TokenEndpoint:     "/oauth/token",
		Scopes:            "openid profile email offline_access",
		ResponseType:      "code",
		ResponseMode:      "query",
	},
	"azure": {
		Name:              "Azure AD",
		AuthorizeEndpoint: "/oauth2/v2.0/authorize",
		TokenEndpoint:     "/oauth2/v2.0/token",
		Scopes:            "openid profile email offline_access",
		ResponseType:      "code",
		ResponseMode:      "query",
	},
	"jumpcloud": {
		Name:              "JumpCloud",
		AuthorizeEndpoint: "/oauth2/auth",
		TokenEndpoint:     "/oauth2/token",
		Scopes:            "openid profile email offline_access",
		ResponseType:      "code",
		ResponseMode:      "query",
	},
	"cognito": {
		Name:              "AWS Cognito User Pool",
		AuthorizeEndpoint: "/oauth2/authorize",
		TokenEndpoint:     "/oauth2/token",
		Scopes:            "openid email offline_access",
		ResponseType:      "code",
		ResponseMode:      "query",
	},
}

// DetermineProviderType auto-detects the OIDC provider from the domain string.
// It handles both full URLs (https://foo.okta.com) and bare domains (foo.okta.com).
// If providerType is set to something other than "auto", it is returned as-is.
func DetermineProviderType(domain, providerType string) (string, error) {
	if providerType != "" && providerType != "auto" {
		return providerType, nil
	}

	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return "", fmt.Errorf(
			"unable to auto-detect provider type for empty domain. "+
				"Known providers: Okta, Auth0, Microsoft/Azure, JumpCloud, AWS Cognito User Pool. "+
				"Please check your provider domain configuration")
	}

	urlStr := domain
	if !strings.HasPrefix(domain, "http://") && !strings.HasPrefix(domain, "https://") {
		urlStr = "https://" + domain
	}

	parsed, err := url.Parse(urlStr)
	if err != nil || parsed.Hostname() == "" {
		return "", fmt.Errorf(
			"unable to auto-detect provider type for domain '%s'. "+
				"Known providers: Okta, Auth0, Microsoft/Azure, JumpCloud, AWS Cognito User Pool. "+
				"Please check your provider domain configuration", domain)
	}

	hostname := strings.ToLower(parsed.Hostname())

	switch {
	case hostname == "okta.com" || strings.HasSuffix(hostname, ".okta.com"):
		return "okta", nil
	case hostname == "auth0.com" || strings.HasSuffix(hostname, ".auth0.com"):
		return "auth0", nil
	case hostname == "microsoftonline.com" || strings.HasSuffix(hostname, ".microsoftonline.com"):
		return "azure", nil
	case hostname == "windows.net" || strings.HasSuffix(hostname, ".windows.net"):
		return "azure", nil
	case hostname == "jumpcloud.com" || strings.HasSuffix(hostname, ".jumpcloud.com"):
		return "jumpcloud", nil
	case hostname == "amazoncognito.com" || strings.HasSuffix(hostname, ".amazoncognito.com"):
		return "cognito", nil
	default:
		return "", fmt.Errorf(
			"unable to auto-detect provider type for domain '%s'. "+
				"Known providers: Okta, Auth0, Microsoft/Azure, JumpCloud, AWS Cognito User Pool. "+
				"Please check your provider domain configuration", domain)
	}
}

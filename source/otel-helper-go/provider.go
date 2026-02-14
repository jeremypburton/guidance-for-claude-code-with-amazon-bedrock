package main

import (
	"net/url"
	"strings"
)

// detectProvider maps an issuer URL to a provider organization label.
func detectProvider(issuer string) string {
	if issuer == "" {
		return "amazon-internal"
	}

	// Handle both full URLs and domain-only inputs
	urlToParse := issuer
	if !strings.HasPrefix(issuer, "http://") && !strings.HasPrefix(issuer, "https://") {
		urlToParse = "https://" + issuer
	}

	parsed, err := url.Parse(urlToParse)
	if err != nil {
		return "amazon-internal"
	}

	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "" {
		return "amazon-internal"
	}

	switch {
	case hostname == "okta.com" || strings.HasSuffix(hostname, ".okta.com"):
		return "okta"
	case hostname == "auth0.com" || strings.HasSuffix(hostname, ".auth0.com"):
		return "auth0"
	case hostname == "microsoftonline.com" || strings.HasSuffix(hostname, ".microsoftonline.com"):
		return "azure"
	case hostname == "jumpcloud.com" || strings.HasSuffix(hostname, ".jumpcloud.com"):
		return "jc_org"
	default:
		return "amazon-internal"
	}
}

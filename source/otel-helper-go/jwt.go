package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// decodeJWTPayload extracts and decodes the payload from a JWT token.
// On any error, it logs to stderr and returns an empty map (never fails).
func decodeJWTPayload(token string) map[string]interface{} {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		logError("Error decoding JWT: token does not have 3 parts")
		return map[string]interface{}{}
	}

	decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		logError("Error decoding JWT: %v", err)
		return map[string]interface{}{}
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(decoded, &payload); err != nil {
		logError("Error decoding JWT: %v", err)
		return map[string]interface{}{}
	}

	if debugMode {
		redacted := redactClaims(payload)
		data, _ := json.MarshalIndent(redacted, "", "  ")
		debugPrint("JWT Payload (redacted): %s", string(data))
	}

	return payload
}

// redactClaims returns a copy of claims with sensitive fields redacted.
func redactClaims(claims map[string]interface{}) map[string]interface{} {
	redacted := make(map[string]interface{}, len(claims))
	for k, v := range claims {
		redacted[k] = v
	}
	for _, field := range []string{"email", "sub", "at_hash", "nonce"} {
		if _, ok := redacted[field]; ok {
			redacted[field] = fmt.Sprintf("<%s-redacted>", field)
		}
	}
	return redacted
}

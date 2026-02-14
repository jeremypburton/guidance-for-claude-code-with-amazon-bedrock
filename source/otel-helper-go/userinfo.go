package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// UserInfo holds extracted user attributes from JWT claims.
type UserInfo struct {
	Email          string
	UserID         string
	Username       string
	OrganizationID string
	Department     string
	Team           string
	CostCenter     string
	Manager        string
	Location       string
	Role           string
	Company        string
	AccountUUID    string
	Issuer         string
	Subject        string
}

// extractUserInfo extracts user information from JWT claims using provider-specific fallback chains.
func extractUserInfo(claims map[string]interface{}) UserInfo {
	email := firstString(claims, "email", "preferred_username", "mail")
	if email == "" {
		email = "unknown@example.com"
	}

	// Hash user ID for privacy
	rawUserID := firstString(claims, "sub", "user_id")
	userID := ""
	if rawUserID != "" {
		hash := sha256.Sum256([]byte(rawUserID))
		h := hex.EncodeToString(hash[:])
		// Format as UUID-like: 8-4-4-4-12 (uses first 32 hex chars)
		userID = h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
	}

	username := firstString(claims, "cognito:username", "username", "preferred_username", "upn", "name")
	if username == "" {
		// Fall back to email prefix
		if idx := strings.Index(email, "@"); idx >= 0 {
			username = email[:idx]
		} else {
			username = email
		}
	}

	// Organization from issuer
	orgID := "amazon-internal"
	if iss := firstString(claims, "iss"); iss != "" {
		orgID = detectProvider(iss)
	}

	department := firstString(claims, "department", "dept", "division", "organizationalUnit")
	if department == "" {
		department = "unspecified"
	}

	// Team: string claims first, then groups array
	team := firstString(claims, "team", "team_id", "group")
	if team == "" {
		groups := claimStringSlice(claims, "groups")
		if len(groups) == 1 {
			team = groups[0]
		} else if len(groups) > 1 {
			limit := 3
			if len(groups) < limit {
				limit = len(groups)
			}
			team = strings.Join(groups[:limit], ",")
		} else {
			team = "default-team"
		}
	}

	costCenter := firstString(claims, "cost_center", "costCenter", "cost_code", "costcenter")
	if costCenter == "" {
		costCenter = "general"
	}

	manager := firstString(claims, "manager", "manager_email", "managerId")
	if manager == "" {
		manager = "unassigned"
	}

	location := firstString(claims, "location", "office_location", "office", "physicalDeliveryOfficeName", "l")
	if location == "" {
		location = "remote"
	}

	role := firstString(claims, "role", "job_title", "title", "jobTitle")
	if role == "" {
		role = "user"
	}

	company := firstString(claims, "company")

	return UserInfo{
		Email:          email,
		UserID:         userID,
		Username:       username,
		OrganizationID: orgID,
		Department:     department,
		Team:           team,
		CostCenter:     costCenter,
		Manager:        manager,
		Location:       location,
		Role:           role,
		Company:        company,
		AccountUUID:    claimString(claims, "aud"),
		Issuer:         firstString(claims, "iss"),
		Subject:        firstString(claims, "sub"),
	}
}

// firstString returns the first non-empty string value found for the given claim keys.
func firstString(claims map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if val, ok := claims[key]; ok {
			if s, ok := val.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// claimString extracts a single claim as a string, handling the case where
// the value may be a string or a JSON array (e.g. JWT "aud" claim).
func claimString(claims map[string]interface{}, key string) string {
	val, ok := claims[key]
	if !ok {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case []interface{}:
		if len(v) > 0 {
			if s, ok := v[0].(string); ok {
				return s
			}
		}
	}
	return ""
}

// claimStringSlice extracts a claim as a string slice from a JSON array.
func claimStringSlice(claims map[string]interface{}, key string) []string {
	val, ok := claims[key]
	if !ok {
		return nil
	}
	arr, ok := val.([]interface{})
	if !ok {
		return nil
	}
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

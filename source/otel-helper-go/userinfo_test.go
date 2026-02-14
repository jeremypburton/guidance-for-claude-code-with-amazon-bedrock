package main

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestExtractUserInfo_BasicClaims(t *testing.T) {
	claims := map[string]interface{}{
		"email": "alice@example.com",
		"sub":   "user-123",
		"iss":   "https://dev.okta.com/oauth2",
		"aud":   "client-id-456",
	}

	info := extractUserInfo(claims)

	if info.Email != "alice@example.com" {
		t.Errorf("Email = %q, want %q", info.Email, "alice@example.com")
	}

	// Verify SHA256 UUID format
	hash := sha256.Sum256([]byte("user-123"))
	h := hex.EncodeToString(hash[:])
	expectedID := h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
	if info.UserID != expectedID {
		t.Errorf("UserID = %q, want %q", info.UserID, expectedID)
	}

	if info.OrganizationID != "okta" {
		t.Errorf("OrganizationID = %q, want %q", info.OrganizationID, "okta")
	}

	if info.AccountUUID != "client-id-456" {
		t.Errorf("AccountUUID = %q, want %q", info.AccountUUID, "client-id-456")
	}

	if info.Subject != "user-123" {
		t.Errorf("Subject = %q, want %q", info.Subject, "user-123")
	}
}

func TestExtractUserInfo_Defaults(t *testing.T) {
	info := extractUserInfo(map[string]interface{}{})

	if info.Email != "unknown@example.com" {
		t.Errorf("Email = %q, want %q", info.Email, "unknown@example.com")
	}
	if info.UserID != "" {
		t.Errorf("UserID = %q, want empty", info.UserID)
	}
	if info.Username != "unknown" {
		t.Errorf("Username = %q, want %q", info.Username, "unknown")
	}
	if info.OrganizationID != "amazon-internal" {
		t.Errorf("OrganizationID = %q, want %q", info.OrganizationID, "amazon-internal")
	}
	if info.Department != "unspecified" {
		t.Errorf("Department = %q, want %q", info.Department, "unspecified")
	}
	if info.Team != "default-team" {
		t.Errorf("Team = %q, want %q", info.Team, "default-team")
	}
	if info.CostCenter != "general" {
		t.Errorf("CostCenter = %q, want %q", info.CostCenter, "general")
	}
	if info.Manager != "unassigned" {
		t.Errorf("Manager = %q, want %q", info.Manager, "unassigned")
	}
	if info.Location != "remote" {
		t.Errorf("Location = %q, want %q", info.Location, "remote")
	}
	if info.Role != "user" {
		t.Errorf("Role = %q, want %q", info.Role, "user")
	}
}

func TestExtractUserInfo_EmailFallbackChain(t *testing.T) {
	// preferred_username fallback
	info := extractUserInfo(map[string]interface{}{
		"preferred_username": "bob@company.com",
	})
	if info.Email != "bob@company.com" {
		t.Errorf("Email = %q, want %q", info.Email, "bob@company.com")
	}

	// mail fallback
	info = extractUserInfo(map[string]interface{}{
		"mail": "carol@company.com",
	})
	if info.Email != "carol@company.com" {
		t.Errorf("Email = %q, want %q", info.Email, "carol@company.com")
	}
}

func TestExtractUserInfo_UsernameFallbackChain(t *testing.T) {
	// cognito:username
	info := extractUserInfo(map[string]interface{}{
		"cognito:username": "cognitouser",
		"email":            "test@test.com",
	})
	if info.Username != "cognitouser" {
		t.Errorf("Username = %q, want %q", info.Username, "cognitouser")
	}

	// upn fallback (Azure)
	info = extractUserInfo(map[string]interface{}{
		"upn":   "user@ad.company.com",
		"email": "test@test.com",
	})
	if info.Username != "user@ad.company.com" {
		t.Errorf("Username = %q, want %q", info.Username, "user@ad.company.com")
	}

	// email prefix fallback
	info = extractUserInfo(map[string]interface{}{
		"email": "alice@example.com",
	})
	if info.Username != "alice" {
		t.Errorf("Username = %q, want %q", info.Username, "alice")
	}
}

func TestExtractUserInfo_GroupsArray(t *testing.T) {
	// Single group
	info := extractUserInfo(map[string]interface{}{
		"groups": []interface{}{"engineering"},
	})
	if info.Team != "engineering" {
		t.Errorf("Team = %q, want %q", info.Team, "engineering")
	}

	// Multiple groups (join first 3)
	info = extractUserInfo(map[string]interface{}{
		"groups": []interface{}{"eng", "platform", "infra", "extra"},
	})
	if info.Team != "eng,platform,infra" {
		t.Errorf("Team = %q, want %q", info.Team, "eng,platform,infra")
	}

	// Two groups
	info = extractUserInfo(map[string]interface{}{
		"groups": []interface{}{"team-a", "team-b"},
	})
	if info.Team != "team-a,team-b" {
		t.Errorf("Team = %q, want %q", info.Team, "team-a,team-b")
	}
}

func TestExtractUserInfo_AudAsArray(t *testing.T) {
	info := extractUserInfo(map[string]interface{}{
		"aud": []interface{}{"client-id-1", "client-id-2"},
	})
	if info.AccountUUID != "client-id-1" {
		t.Errorf("AccountUUID = %q, want %q", info.AccountUUID, "client-id-1")
	}
}

func TestExtractUserInfo_Company(t *testing.T) {
	info := extractUserInfo(map[string]interface{}{
		"company": "Acme Corp",
	})
	if info.Company != "Acme Corp" {
		t.Errorf("Company = %q, want %q", info.Company, "Acme Corp")
	}

	// No company
	info = extractUserInfo(map[string]interface{}{})
	if info.Company != "" {
		t.Errorf("Company = %q, want empty", info.Company)
	}
}

func TestFirstString(t *testing.T) {
	claims := map[string]interface{}{
		"a": "",
		"b": "value-b",
		"c": "value-c",
	}

	// Skips empty, returns first non-empty
	if got := firstString(claims, "a", "b", "c"); got != "value-b" {
		t.Errorf("firstString = %q, want %q", got, "value-b")
	}

	// Missing key returns empty
	if got := firstString(claims, "missing"); got != "" {
		t.Errorf("firstString = %q, want empty", got)
	}
}

func TestClaimString_StringValue(t *testing.T) {
	claims := map[string]interface{}{"aud": "client-123"}
	if got := claimString(claims, "aud"); got != "client-123" {
		t.Errorf("claimString = %q, want %q", got, "client-123")
	}
}

func TestClaimString_ArrayValue(t *testing.T) {
	claims := map[string]interface{}{"aud": []interface{}{"client-1", "client-2"}}
	if got := claimString(claims, "aud"); got != "client-1" {
		t.Errorf("claimString = %q, want %q", got, "client-1")
	}
}

func TestClaimString_Missing(t *testing.T) {
	claims := map[string]interface{}{}
	if got := claimString(claims, "aud"); got != "" {
		t.Errorf("claimString = %q, want empty", got)
	}
}

func TestClaimStringSlice(t *testing.T) {
	claims := map[string]interface{}{
		"groups": []interface{}{"a", "b", "c"},
	}
	got := claimStringSlice(claims, "groups")
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("claimStringSlice = %v, want [a b c]", got)
	}

	// Missing key
	got = claimStringSlice(claims, "missing")
	if got != nil {
		t.Errorf("claimStringSlice for missing key = %v, want nil", got)
	}

	// Non-array value
	claims["notarray"] = "string"
	got = claimStringSlice(claims, "notarray")
	if got != nil {
		t.Errorf("claimStringSlice for string = %v, want nil", got)
	}
}

// TestSHA256Parity verifies the Go SHA256 UUID matches the Python output.
// Python: hashlib.sha256("test-user-123".encode()).hexdigest()[:36]
// then formatted as 8-4-4-4-12.
func TestSHA256Parity(t *testing.T) {
	claims := map[string]interface{}{
		"sub":   "test-user-123",
		"email": "alice@example.com",
		"iss":   "https://dev.okta.com/oauth2",
	}

	info := extractUserInfo(claims)

	// Pre-computed from Python:
	// import hashlib
	// h = hashlib.sha256(b"test-user-123").hexdigest()
	// f"{h[:8]}-{h[8:12]}-{h[12:16]}-{h[16:20]}-{h[20:32]}"
	// = "a4349ef2-d541-cfb2-be25-2bd5f30bdac3"
	hash := sha256.Sum256([]byte("test-user-123"))
	h := hex.EncodeToString(hash[:])
	expected := h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]

	if info.UserID != expected {
		t.Errorf("UserID = %q, want %q", info.UserID, expected)
	}
}

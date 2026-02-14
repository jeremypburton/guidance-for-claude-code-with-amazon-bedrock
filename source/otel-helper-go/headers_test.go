package main

import "testing"

func TestFormatAsHeaders_AllFields(t *testing.T) {
	info := UserInfo{
		Email:          "alice@example.com",
		UserID:         "abc-def-123",
		Username:       "alice",
		Department:     "engineering",
		Team:           "platform",
		CostCenter:     "cc-100",
		OrganizationID: "okta",
		Location:       "seattle",
		Role:           "engineer",
		Manager:        "bob@example.com",
		Company:        "Acme Corp",
	}

	headers := formatAsHeaders(info)

	expected := map[string]string{
		"x-user-email":  "alice@example.com",
		"x-user-id":     "abc-def-123",
		"x-user-name":   "alice",
		"x-department":  "engineering",
		"x-team-id":     "platform",
		"x-cost-center": "cc-100",
		"x-organization": "okta",
		"x-location":    "seattle",
		"x-role":        "engineer",
		"x-manager":     "bob@example.com",
		"x-company":     "Acme Corp",
	}

	for k, v := range expected {
		if headers[k] != v {
			t.Errorf("headers[%q] = %q, want %q", k, headers[k], v)
		}
	}

	if len(headers) != len(expected) {
		t.Errorf("headers has %d entries, want %d", len(headers), len(expected))
	}
}

func TestFormatAsHeaders_EmptyCompanyOmitted(t *testing.T) {
	info := UserInfo{
		Email:          "alice@example.com",
		UserID:         "abc",
		Username:       "alice",
		Department:     "eng",
		Team:           "team",
		CostCenter:     "cc",
		OrganizationID: "okta",
		Location:       "remote",
		Role:           "user",
		Manager:        "bob",
		Company:        "", // empty — should be omitted
	}

	headers := formatAsHeaders(info)

	if _, ok := headers["x-company"]; ok {
		t.Error("x-company should not be present when Company is empty")
	}
}

func TestFormatAsHeaders_EmptyFieldsOmitted(t *testing.T) {
	info := UserInfo{} // all empty
	headers := formatAsHeaders(info)

	if len(headers) != 0 {
		t.Errorf("expected empty headers for empty UserInfo, got %v", headers)
	}
}

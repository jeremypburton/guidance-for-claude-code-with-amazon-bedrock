package main

// formatAsHeaders converts UserInfo into a map of HTTP header names to values.
// Only non-empty values are included.
func formatAsHeaders(info UserInfo) map[string]string {
	// Ordered list to control iteration (though json.Marshal sorts alphabetically)
	type mapping struct {
		field      string
		headerName string
	}

	mappings := []mapping{
		{info.Email, "x-user-email"},
		{info.UserID, "x-user-id"},
		{info.Username, "x-user-name"},
		{info.Department, "x-department"},
		{info.Team, "x-team-id"},
		{info.CostCenter, "x-cost-center"},
		{info.OrganizationID, "x-organization"},
		{info.Location, "x-location"},
		{info.Role, "x-role"},
		{info.Manager, "x-manager"},
		{info.Company, "x-company"},
	}

	headers := make(map[string]string)
	for _, m := range mappings {
		if m.field != "" {
			headers[m.headerName] = m.field
		}
	}
	return headers
}

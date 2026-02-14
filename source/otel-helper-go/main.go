package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

func main() {
	os.Exit(run())
}

func run() int {
	testFlag := flag.Bool("test", false, "Run in test mode with verbose output")
	verboseFlag := flag.Bool("verbose", false, "Show verbose output")
	flag.Parse()

	initDebug()
	defer closeDebug()

	testMode = *testFlag

	// --test or --verbose implies debug mode
	if *testFlag || *verboseFlag {
		debugMode = true
	}

	// Try environment variable first
	token := os.Getenv("CLAUDE_CODE_MONITORING_TOKEN")
	if token != "" {
		logInfo("Using token from environment variable CLAUDE_CODE_MONITORING_TOKEN")
	} else {
		token = getTokenViaCredentialProcess()
		if token == "" {
			logWarning("Could not obtain authentication token")
			return 1
		}
	}

	// Decode JWT (returns empty map on error, never fails)
	payload := decodeJWTPayload(token)
	userInfo := extractUserInfo(payload)
	headersMap := formatAsHeaders(userInfo)

	if testMode {
		printTestOutput(headersMap, userInfo)
	} else {
		data, err := json.Marshal(headersMap)
		if err != nil {
			logError("Error processing token: %v", err)
			return 1
		}
		fmt.Println(string(data))
	}

	if debugMode || testMode {
		logInfo("Generated OTEL resource attributes:")
		if debugMode {
			data, _ := json.MarshalIndent(userInfoToMap(userInfo), "", "  ")
			debugPrint("Attributes: %s", string(data))
		}
	}

	return 0
}

// printTestOutput produces the same test mode output as the Python implementation.
// PARSING CONTRACT: test.py parses for "X-user-email:" and "user.id:" exactly.
func printTestOutput(headers map[string]string, info UserInfo) {
	fmt.Println("===== TEST MODE OUTPUT =====")
	fmt.Println()
	fmt.Println("Generated HTTP Headers:")

	// Sort header keys for deterministic output
	headerKeys := make([]string, 0, len(headers))
	for k := range headers {
		headerKeys = append(headerKeys, k)
	}
	sort.Strings(headerKeys)

	for _, headerName := range headerKeys {
		displayName := strings.Replace(headerName, "x-", "X-", 1)
		displayName = strings.Replace(displayName, "-id", "-ID", 1)
		fmt.Printf("  %s: %s\n", displayName, headers[headerName])
	}

	fmt.Println()
	fmt.Println("===== Extracted Attributes =====")
	fmt.Println()

	// Section 1: loop over fields, skip technical fields, replace _ with .
	type kv struct {
		key   string
		value string
	}
	attrs := []kv{
		{"email", info.Email},
		{"user_id", info.UserID},
		{"username", info.Username},
		{"organization_id", info.OrganizationID},
		{"department", info.Department},
		{"team", info.Team},
		{"cost_center", info.CostCenter},
		{"manager", info.Manager},
		{"location", info.Location},
		{"role", info.Role},
		{"company", info.Company},
	}
	for _, attr := range attrs {
		displayKey := strings.ReplaceAll(attr.key, "_", ".")
		displayValue := attr.value
		if len(displayValue) > 30 {
			displayValue = displayValue[:30] + "..."
		}
		fmt.Printf("  %s: %s\n", displayKey, displayValue)
	}

	// Section 2: explicit attribute lines
	fmt.Println()
	fmt.Printf("  user.email: %s\n", info.Email)
	fmt.Printf("  user.id: %s...\n", truncate(info.UserID, 30))
	fmt.Printf("  user.name: %s\n", info.Username)
	fmt.Printf("  organization.id: %s\n", info.OrganizationID)
	fmt.Println("  service.name: claude-code")
	fmt.Printf("  user.account_uuid: %s\n", info.AccountUUID)
	fmt.Printf("  oidc.issuer: %s...\n", truncate(info.Issuer, 30))
	fmt.Printf("  oidc.subject: %s...\n", truncate(info.Subject, 30))
	fmt.Printf("  department: %s\n", info.Department)
	fmt.Printf("  team.id: %s\n", info.Team)
	fmt.Printf("  cost_center: %s\n", info.CostCenter)
	fmt.Printf("  manager: %s\n", info.Manager)
	fmt.Printf("  location: %s\n", info.Location)
	fmt.Printf("  role: %s\n", info.Role)

	fmt.Println()
	fmt.Println("========================")
}

// truncate returns the first n characters of s (or s itself if shorter).
// This unconditionally truncates — the caller appends "..." separately.
func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// userInfoToMap converts UserInfo to a map for debug JSON output.
func userInfoToMap(info UserInfo) map[string]interface{} {
	m := map[string]interface{}{
		"email":           info.Email,
		"user_id":         info.UserID,
		"username":        info.Username,
		"organization_id": info.OrganizationID,
		"department":      info.Department,
		"team":            info.Team,
		"cost_center":     info.CostCenter,
		"manager":         info.Manager,
		"location":        info.Location,
		"role":            info.Role,
		"account_uuid":    info.AccountUUID,
		"issuer":          info.Issuer,
		"subject":         info.Subject,
	}
	if info.Company != "" {
		m["company"] = info.Company
	}
	return m
}

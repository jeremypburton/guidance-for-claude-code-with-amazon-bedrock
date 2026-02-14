package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"credential-provider-go/auth"
	"credential-provider-go/config"
	"credential-provider-go/credentials"
	"credential-provider-go/federation"
	"credential-provider-go/internal"
	"credential-provider-go/locking"
	"credential-provider-go/monitoring"
	"credential-provider-go/provider"
	"credential-provider-go/quota"

	"github.com/golang-jwt/jwt/v5"
)

const version = "1.0.0"

func main() {
	os.Exit(run())
}

func run() int {
	// Parse flags
	profileFlag := flag.String("profile", "", "Configuration profile to use")
	flag.StringVar(profileFlag, "p", "", "Configuration profile to use (shorthand)")
	versionFlag := flag.Bool("version", false, "Print version and exit")
	flag.BoolVar(versionFlag, "v", false, "Print version and exit (shorthand)")
	getMonitoringToken := flag.Bool("get-monitoring-token", false, "Get cached monitoring token")
	clearCache := flag.Bool("clear-cache", false, "Clear cached credentials")
	checkExpiration := flag.Bool("check-expiration", false, "Check if credentials need refresh")
	refreshIfNeeded := flag.Bool("refresh-if-needed", false, "Refresh credentials if expired")
	flag.Parse()

	internal.InitDebug()
	defer internal.CloseDebug()

	if *versionFlag {
		fmt.Printf("credential-provider-go %s\n", version)
		return 0
	}

	// Determine profile: flag > env > auto-detect > default
	profile := *profileFlag
	if profile == "" {
		profile = os.Getenv("CCWB_PROFILE")
	}
	if profile == "" {
		profile = config.AutoDetectProfile()
	}
	if profile == "" {
		profile = "ClaudeCode"
	}

	// Load configuration
	cfg, err := config.LoadConfig(profile)
	if err != nil {
		internal.StatusPrint("Error: %v\n", err)
		return 1
	}

	// Resolve provider type
	providerType, err := provider.DetermineProviderType(cfg.ProviderDomain, cfg.ProviderType)
	if err != nil {
		internal.StatusPrint("Error: %v\n", err)
		return 1
	}
	providerCfg, ok := provider.ProviderConfigs[providerType]
	if !ok {
		internal.StatusPrint("Error: unknown provider type '%s'\n", providerType)
		return 1
	}
	// Store resolved provider type back into config for federation to use
	cfg.ProviderType = providerType

	// Handle --clear-cache
	if *clearCache {
		cleared := credentials.ClearCredentials(profile)
		if len(cleared) > 0 {
			internal.StatusPrint("Cleared cached credentials for profile '%s':\n", profile)
			for _, item := range cleared {
				internal.StatusPrint("  - %s\n", item)
			}
		} else {
			internal.StatusPrint("No cached credentials found for profile '%s'\n", profile)
		}
		return 0
	}

	// Handle --get-monitoring-token
	if *getMonitoringToken {
		token := monitoring.GetMonitoringToken(profile)
		if token != "" {
			fmt.Println(token)
			return 0
		}
		// No cached token, trigger authentication
		internal.DebugPrint("No valid monitoring token found, triggering authentication...")
		token, code := authenticateForMonitoring(cfg, profile, providerType, providerCfg)
		if code != 0 {
			return code
		}
		if token != "" {
			fmt.Println(token)
			return 0
		}
		return 1
	}

	// Handle --check-expiration
	if *checkExpiration {
		if credentials.CheckExpiration(profile) {
			internal.StatusPrint("Credentials expired or missing for profile '%s'\n", profile)
			return 1
		}
		internal.StatusPrint("Credentials valid for profile '%s'\n", profile)
		return 0
	}

	// Handle --refresh-if-needed
	if *refreshIfNeeded {
		if cfg.CredentialStorage != "session" {
			internal.StatusPrint("Error: --refresh-if-needed only works with session storage mode\n")
			return 1
		}
		if !credentials.CheckExpiration(profile) {
			internal.DebugPrint("Credentials still valid for profile '%s', no refresh needed", profile)
			return 0
		}
		// Fall through to normal auth flow
	}

	// Normal credential flow
	return runCredentialFlow(cfg, profile, providerType, providerCfg)
}

func runCredentialFlow(cfg *config.ProfileConfig, profile, providerType string, providerCfg provider.Config) int {
	redirectPort := 8400
	if p := os.Getenv("REDIRECT_PORT"); p != "" {
		fmt.Sscanf(p, "%d", &redirectPort)
	}
	redirectURI := fmt.Sprintf("http://localhost:%d/callback", redirectPort)

	// Check cache first
	cached := credentials.GetCachedCredentials(profile)
	if cached != nil {
		// Periodic quota re-check
		if quota.ShouldRecheck(cfg.QuotaAPIEndpoint, cfg.QuotaCheckInterval, profile) {
			internal.DebugPrint("Performing periodic quota re-check...")
			idToken := monitoring.GetMonitoringToken(profile)
			tokenClaims := monitoring.GetCachedTokenClaims(profile)
			if idToken != "" && tokenClaims != nil {
				claims := jwt.MapClaims{}
				for k, v := range tokenClaims {
					claims[k] = v
				}
				result := quota.Check(cfg.QuotaAPIEndpoint, idToken, claims, cfg.QuotaFailMode, cfg.QuotaCheckTimeout)
				quota.SaveQuotaCheckTimestamp(profile)
				if !result.Allowed {
					return quota.HandleBlocked(result)
				}
				quota.HandleWarning(result)
			} else {
				internal.DebugPrint("No cached token for quota re-check, skipping")
			}
		}

		printJSON(cached)
		return 0
	}

	// Try port lock
	if !locking.TryAcquirePort(redirectPort) {
		// Another auth in progress, wait
		locking.WaitForPort(redirectPort, 60*time.Second)
		cached = credentials.GetCachedCredentials(profile)
		if cached != nil {
			printJSON(cached)
			return 0
		}
		internal.DebugPrint("Authentication timeout or failed in another process")
		return 1
	}

	// Double-check cache (another process may have just finished)
	cached = credentials.GetCachedCredentials(profile)
	if cached != nil {
		printJSON(cached)
		return 0
	}

	// Perform OIDC authentication
	internal.DebugPrint("Authenticating with %s for profile '%s'...", providerCfg.Name, profile)

	idToken, tokenClaims, err := performOIDCAuth(cfg, providerType, providerCfg, redirectPort, redirectURI)
	if err != nil {
		internal.StatusPrint("Error: %v\n", err)
		return 1
	}

	// Check quota before issuing credentials
	if quota.ShouldCheck(cfg.QuotaAPIEndpoint) {
		internal.DebugPrint("Checking quota before credential issuance...")
		result := quota.Check(cfg.QuotaAPIEndpoint, idToken, tokenClaims, cfg.QuotaFailMode, cfg.QuotaCheckTimeout)
		quota.SaveQuotaCheckTimestamp(profile)
		if !result.Allowed {
			return quota.HandleBlocked(result)
		}
		quota.HandleWarning(result)
	}

	// Exchange token for AWS credentials
	internal.DebugPrint("Exchanging token for AWS credentials...")
	creds, err := federation.GetAWSCredentials(cfg, idToken, tokenClaims)
	if err != nil {
		internal.StatusPrint("Error: %v\n", err)
		return 1
	}

	// Cache credentials
	if err := credentials.SaveToCredentialsFile(creds, profile); err != nil {
		internal.DebugPrint("Warning: failed to cache credentials: %v", err)
	}

	// Save monitoring token (non-fatal)
	monitoring.SaveMonitoringToken(idToken, tokenClaims, profile)

	// Output credentials to stdout
	printJSON(creds)
	return 0
}

func authenticateForMonitoring(cfg *config.ProfileConfig, profile, providerType string, providerCfg provider.Config) (string, int) {
	redirectPort := 8400
	if p := os.Getenv("REDIRECT_PORT"); p != "" {
		fmt.Sscanf(p, "%d", &redirectPort)
	}
	redirectURI := fmt.Sprintf("http://localhost:%d/callback", redirectPort)

	// Try port lock
	if !locking.TryAcquirePort(redirectPort) {
		internal.DebugPrint("Another authentication is in progress, waiting...")
		locking.WaitForPort(redirectPort, 60*time.Second)
		token := monitoring.GetMonitoringToken(profile)
		if token != "" {
			return token, 0
		}
		internal.DebugPrint("Authentication timeout or failed in another process")
		return "", 1
	}

	internal.DebugPrint("Authenticating with %s for monitoring token...", providerCfg.Name)

	idToken, tokenClaims, err := performOIDCAuth(cfg, providerType, providerCfg, redirectPort, redirectURI)
	if err != nil {
		internal.DebugPrint("Error during monitoring authentication: %v", err)
		return "", 1
	}

	// Get AWS credentials (needed for caching, not output)
	creds, err := federation.GetAWSCredentials(cfg, idToken, tokenClaims)
	if err != nil {
		internal.DebugPrint("Error exchanging token: %v", err)
		return "", 1
	}

	// Cache credentials
	credentials.SaveToCredentialsFile(creds, profile)

	// Save monitoring token
	monitoring.SaveMonitoringToken(idToken, tokenClaims, profile)

	return idToken, 0
}

func performOIDCAuth(cfg *config.ProfileConfig, providerType string, providerCfg provider.Config,
	redirectPort int, redirectURI string) (string, jwt.MapClaims, error) {

	pkce, err := auth.GeneratePKCE()
	if err != nil {
		return "", nil, err
	}

	state, err := auth.GenerateState()
	if err != nil {
		return "", nil, err
	}

	nonce, err := auth.GenerateNonce()
	if err != nil {
		return "", nil, err
	}

	// Validate Cognito domain
	if providerType == "cognito" {
		if !containsString(cfg.ProviderDomain, "amazoncognito.com") {
			return "", nil, fmt.Errorf(
				"for Cognito User Pool, please provide the User Pool domain " +
					"(e.g., 'my-domain.auth.us-east-1.amazoncognito.com'), " +
					"not the identity pool endpoint")
		}
	}

	authURL := auth.BuildAuthURL(cfg.ProviderDomain, providerType, providerCfg,
		cfg.ClientID, redirectURI, state, nonce, pkce.Challenge)

	// Start callback server in background
	type serverResult struct {
		code string
		err  error
	}
	resultCh := make(chan serverResult, 1)
	go func() {
		code, err := auth.StartCallbackServer(redirectPort, state, 5*time.Minute)
		resultCh <- serverResult{code, err}
	}()

	// Open browser
	internal.DebugPrint("Opening browser for %s authentication...", providerCfg.Name)
	internal.DebugPrint("If browser doesn't open, visit: %s", authURL)
	if err := auth.OpenBrowser(authURL); err != nil {
		internal.DebugPrint("Failed to open browser: %v", err)
	}

	// Wait for callback
	res := <-resultCh
	if res.err != nil {
		return "", nil, res.err
	}

	// Exchange code for tokens
	oauthResult, err := auth.ExchangeCodeForTokens(cfg.ProviderDomain, providerType, providerCfg,
		cfg.ClientID, redirectURI, res.code, pkce.Verifier)
	if err != nil {
		return "", nil, err
	}

	// Validate nonce
	if nonceVal, ok := oauthResult.TokenClaims["nonce"].(string); ok {
		if nonceVal != nonce {
			return "", nil, fmt.Errorf("invalid nonce in ID token")
		}
	}

	if internal.Debug {
		internal.DebugPrint("\n=== ID Token Claims ===")
		claimsJSON, _ := json.MarshalIndent(oauthResult.TokenClaims, "", "  ")
		internal.DebugPrint("%s", string(claimsJSON))
	}

	return oauthResult.IDToken, oauthResult.TokenClaims, nil
}

func printJSON(v interface{}) {
	data, _ := json.Marshal(v)
	fmt.Println(string(data))
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && contains(s, substr))
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

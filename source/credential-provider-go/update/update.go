package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"credential-provider-go/config"
	"credential-provider-go/credentials"
	"credential-provider-go/internal"
)

// ShouldCheck returns true if a background update check should be performed.
// Returns false if auto-update is disabled or the cooldown hasn't expired.
func ShouldCheck(cfg *config.ProfileConfig) bool {
	if !cfg.AutoUpdateEnabled {
		return false
	}

	interval := time.Duration(cfg.AutoUpdateIntervalHrs) * time.Hour
	last := lastCheckTime()
	if time.Since(last) < interval {
		return false
	}

	return true
}

// NotifyIfPending prints a pending update notification to stderr, if one exists.
func NotifyIfPending() {
	msg := consumePendingNotification()
	if msg != "" {
		fmt.Fprintf(os.Stderr, "[auto-update] %s\n", msg)
	}
}

// TryCheckAndSpawn performs the check-and-spawn cycle synchronously.
// It returns quickly if no check is needed or another process holds the lock.
// credsJSON should be the raw credential output (with AccessKeyId, SecretAccessKey, etc.).
func TryCheckAndSpawn(cfg *config.ProfileConfig, currentVersion string, creds interface{}) {
	if !ShouldCheck(cfg) {
		return
	}

	// Try to acquire the update-check lock
	lockPath := checkLockPath()
	if lockPath == "" {
		return
	}

	if !tryAcquireCheckLock(lockPath) {
		internal.DebugPrint("Another process holds the update-check lock, skipping")
		return
	}
	defer releaseCheckLock(lockPath)

	// Record that we checked (advances cooldown even on failure)
	recordCheckTime()

	// Create S3 client using static credentials
	accessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	sessionToken := os.Getenv("AWS_SESSION_TOKEN")

	// If env vars aren't set, try to extract from the creds output
	if accessKeyID == "" {
		accessKeyID, secretKey, sessionToken = extractCredsFromOutput(creds)
	}

	if accessKeyID == "" {
		internal.DebugPrint("No credentials available for update check")
		return
	}

	// Check credential expiration — skip if <10 min remaining
	if !hasEnoughTimeRemaining(creds) {
		internal.DebugPrint("Credentials expire too soon for update check")
		return
	}

	client, err := newS3Client(cfg.UpdateRegion, accessKeyID, secretKey, sessionToken)
	if err != nil {
		internal.DebugPrint("Failed to create S3 client for update: %v", err)
		return
	}

	m, err := fetchManifest(client, cfg.UpdateBucket, cfg.UpdatePrefix)
	if err != nil {
		internal.DebugPrint("Failed to fetch manifest: %v", err)
		recordUpdateError("s3_unreachable")
		return
	}

	if !isNewerVersion(m.Version, currentVersion) {
		internal.DebugPrint("Already at latest version %s", currentVersion)
		return
	}

	internal.DebugPrint("New version available: %s (current: %s)", m.Version, currentVersion)

	// Spawn detached self-update process
	spawnSelfUpdate(cfg, m.Version, accessKeyID, secretKey, sessionToken)
}

// RunSelfUpdate performs the actual update. Called when credential-process --self-update is invoked.
func RunSelfUpdate(targetVersion string) int {
	internal.DebugPrint("Starting self-update to version %s", targetVersion)

	// Load update config from config.json (primary source for bucket/region/prefix)
	profile, cfg := loadUpdateConfig()

	bucket := os.Getenv("CCWB_UPDATE_BUCKET")
	prefix := os.Getenv("CCWB_UPDATE_PREFIX")
	region := os.Getenv("AWS_REGION")

	// Config.json values take precedence — they're always correct for this installation
	if cfg != nil {
		if cfg.UpdateBucket != "" {
			bucket = cfg.UpdateBucket
		}
		if cfg.UpdateRegion != "" {
			region = cfg.UpdateRegion
		}
		if cfg.UpdatePrefix != "" {
			prefix = cfg.UpdatePrefix
		}
	}

	if prefix == "" {
		prefix = "packages"
	}

	if bucket == "" || region == "" {
		var missing []string
		if bucket == "" {
			missing = append(missing, "bucket")
		}
		if region == "" {
			missing = append(missing, "region")
		}
		internal.DebugPrint("Missing update config (%s) — not in config.json or env vars", strings.Join(missing, ", "))
		recordUpdateError("missing_config")
		return 1
	}

	// Resolve AWS credentials: cached credentials file first, then env vars
	accessKeyID, secretKey, sessionToken := loadCachedCredentials(profile)
	if accessKeyID == "" {
		accessKeyID = os.Getenv("AWS_ACCESS_KEY_ID")
		secretKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
		sessionToken = os.Getenv("AWS_SESSION_TOKEN")
	}

	if accessKeyID == "" {
		internal.DebugPrint("No AWS credentials available for self-update (checked credentials file and env vars)")
		recordUpdateError("missing_credentials")
		return 1
	}

	// Acquire binary replacement lock
	lockPath := binaryLockPath()
	if lockPath == "" {
		recordUpdateError("lock_failed")
		return 1
	}
	if !tryAcquireBinaryLock(lockPath) {
		internal.DebugPrint("Another self-update is in progress")
		recordUpdateError("lock_contention")
		return 1
	}
	defer releaseBinaryLock(lockPath)

	// Create S3 client
	client, err := newS3Client(region, accessKeyID, secretKey, sessionToken)
	if err != nil {
		internal.DebugPrint("Failed to create S3 client: %v", err)
		recordUpdateError("s3_client_failed")
		return 1
	}

	// Fetch manifest
	m, err := fetchManifest(client, bucket, prefix)
	if err != nil {
		internal.DebugPrint("Failed to fetch manifest: %v", err)
		recordUpdateError("manifest_fetch_failed")
		return 1
	}

	// Determine platform key
	platformKey := getPlatformKey()
	if platformKey == "" {
		internal.DebugPrint("Unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
		recordUpdateError("unsupported_platform")
		return 1
	}

	details, ok := m.Binaries[platformKey]
	if !ok {
		internal.DebugPrint("No binary for platform %s in manifest", platformKey)
		recordUpdateError("platform_not_in_manifest")
		return 1
	}

	// Download zip
	zipPath, err := downloadZip(client, bucket, details)
	if err != nil {
		internal.DebugPrint("Download failed: %v", err)
		recordUpdateError("download_failed")
		return 1
	}
	defer os.Remove(zipPath)

	// Extract
	extractDir, err := extractZip(zipPath)
	if err != nil {
		internal.DebugPrint("Extraction failed: %v", err)
		recordUpdateError("extract_failed")
		return 1
	}
	defer os.RemoveAll(extractDir)

	// Validate required files
	requiredBinary := getCredentialProcessBinaryName()
	baseDir, err := validateExtractedFiles(extractDir, requiredBinary)
	if err != nil {
		internal.DebugPrint("Validation failed: %v", err)
		recordUpdateError("validation_failed")
		return 1
	}

	// Install files
	installDir := getInstallDir()
	if installDir == "" {
		internal.DebugPrint("Could not determine install directory")
		recordUpdateError("install_dir_unknown")
		return 1
	}

	// Replace credential-process binary
	srcBinary := filepath.Join(baseDir, requiredBinary)
	destBinary := filepath.Join(installDir, getLocalBinaryName("credential-process"))
	if err := replaceBinary(srcBinary, destBinary); err != nil {
		internal.DebugPrint("Binary replacement failed: %v", err)
		recordUpdateError("replace_failed")
		return 1
	}

	// Replace otel-helper if present
	otelBinary := getOtelHelperBinaryName()
	srcOtel := filepath.Join(baseDir, otelBinary)
	if _, err := os.Stat(srcOtel); err == nil {
		destOtel := filepath.Join(installDir, getLocalBinaryName("otel-helper"))
		if err := replaceBinary(srcOtel, destOtel); err != nil {
			internal.DebugPrint("OTEL helper replacement failed: %v (continuing)", err)
		}
	}

	// Merge settings.json if present in the extracted files
	settingsTemplate := filepath.Join(baseDir, "claude-settings", "settings.json")
	if _, err := os.Stat(settingsTemplate); err == nil {
		home, err := os.UserHomeDir()
		if err == nil {
			existingSettings := filepath.Join(home, ".claude", "settings.json")
			if err := mergeSettings(existingSettings, settingsTemplate, installDir); err != nil {
				internal.DebugPrint("Settings merge failed: %v (continuing)", err)
			} else {
				internal.DebugPrint("Settings.json merged successfully")
			}
		}
	}

	// Replace config.json if present
	srcConfig := filepath.Join(baseDir, "config.json")
	if _, err := os.Stat(srcConfig); err == nil {
		destConfig := filepath.Join(installDir, "config.json")
		data, err := os.ReadFile(srcConfig)
		if err == nil {
			os.WriteFile(destConfig, data, 0644)
			internal.DebugPrint("Config.json updated")
		}
	}

	// Record success
	recordPendingUpdate(targetVersion)
	internal.DebugPrint("Self-update to version %s completed successfully", targetVersion)
	return 0
}

// spawnSelfUpdate spawns a detached background process for self-update.
func spawnSelfUpdate(cfg *config.ProfileConfig, version, accessKeyID, secretKey, sessionToken string) {
	exePath, err := os.Executable()
	if err != nil {
		internal.DebugPrint("Could not determine executable path: %v", err)
		return
	}

	cmd := exec.Command(exePath, "--self-update", "--update-version", version)
	cmd.Env = []string{
		"AWS_ACCESS_KEY_ID=" + accessKeyID,
		"AWS_SECRET_ACCESS_KEY=" + secretKey,
		"AWS_SESSION_TOKEN=" + sessionToken,
		"AWS_REGION=" + cfg.UpdateRegion,
		"AWS_PROFILE=", // Explicitly empty to prevent recursive credential_process
		"CCWB_UPDATE_BUCKET=" + cfg.UpdateBucket,
		"CCWB_UPDATE_PREFIX=" + cfg.UpdatePrefix,
		"HOME=" + os.Getenv("HOME"),
		"USERPROFILE=" + os.Getenv("USERPROFILE"),
		"PATH=" + os.Getenv("PATH"),
		"DEBUG_MODE=" + os.Getenv("DEBUG_MODE"),
		"CREDENTIAL_PROCESS_LOG_FILE=" + os.Getenv("CREDENTIAL_PROCESS_LOG_FILE"),
		"CCWB_PROFILE=" + os.Getenv("CCWB_PROFILE"),
	}
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = setSysProcAttr()

	if err := cmd.Start(); err != nil {
		internal.DebugPrint("Failed to spawn self-update: %v", err)
		return
	}

	// Release the process — parent doesn't wait
	cmd.Process.Release()
	internal.DebugPrint("Spawned self-update process (PID %d) for version %s", cmd.Process.Pid, version)
}

// getPlatformKey returns the platform key for the current OS/arch (matches manifest keys).
func getPlatformKey() string {
	switch runtime.GOOS {
	case "darwin":
		if runtime.GOARCH == "arm64" {
			return "macos-arm64"
		}
		return "macos-intel"
	case "linux":
		if runtime.GOARCH == "arm64" {
			return "linux-arm64"
		}
		return "linux-x64"
	case "windows":
		return "windows"
	}
	return ""
}

// getCredentialProcessBinaryName returns the expected binary name in the zip for the current platform.
func getCredentialProcessBinaryName() string {
	key := getPlatformKey()
	switch key {
	case "windows":
		return "credential-process-windows.exe"
	default:
		return "credential-process-" + key
	}
}

// getOtelHelperBinaryName returns the expected otel-helper name in the zip.
func getOtelHelperBinaryName() string {
	key := getPlatformKey()
	switch key {
	case "windows":
		return "otel-helper-windows.exe"
	default:
		return "otel-helper-" + key
	}
}

// getLocalBinaryName returns the local installed name for a binary.
func getLocalBinaryName(baseName string) string {
	if runtime.GOOS == "windows" {
		return baseName + ".exe"
	}
	return baseName
}

// getInstallDir returns the installation directory (~/claude-code-with-bedrock).
func getInstallDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "claude-code-with-bedrock")
}

// extractCredsFromOutput extracts AWS credentials from the credential output.
func extractCredsFromOutput(creds interface{}) (accessKeyID, secretKey, sessionToken string) {
	type credShape struct {
		AccessKeyID     string `json:"AccessKeyId"`
		SecretAccessKey string `json:"SecretAccessKey"`
		SessionToken    string `json:"SessionToken"`
		Expiration      string `json:"Expiration"`
	}

	// Try type assertion for known types
	switch c := creds.(type) {
	case *credShape:
		if c != nil {
			return c.AccessKeyID, c.SecretAccessKey, c.SessionToken
		}
	case map[string]interface{}:
		if id, ok := c["AccessKeyId"].(string); ok {
			accessKeyID = id
		}
		if sk, ok := c["SecretAccessKey"].(string); ok {
			secretKey = sk
		}
		if st, ok := c["SessionToken"].(string); ok {
			sessionToken = st
		}
		return
	}

	// Try to use reflection-free approach for the credentials output type
	// We'll rely on env vars being set by the caller
	return "", "", ""
}

// hasEnoughTimeRemaining checks if credentials have at least 10 minutes remaining.
func hasEnoughTimeRemaining(creds interface{}) bool {
	type credShape struct {
		Expiration string `json:"Expiration"`
	}

	var expStr string

	switch c := creds.(type) {
	case map[string]interface{}:
		if exp, ok := c["Expiration"].(string); ok {
			expStr = exp
		}
	default:
		// For other types, assume enough time (conservative approach)
		return true
	}

	if expStr == "" {
		return true
	}

	// Try to parse expiration
	expStr = strings.Replace(expStr, "+00:00", "Z", 1)
	expTime, err := time.Parse(time.RFC3339, expStr)
	if err != nil {
		return true // Can't parse, assume OK
	}

	return time.Until(expTime) > 10*time.Minute
}

// Lock file management for update checks

func checkLockPath() string {
	installDir := getInstallDir()
	if installDir == "" {
		return ""
	}
	os.MkdirAll(installDir, 0700)
	return filepath.Join(installDir, ".update-check.lock")
}

func binaryLockPath() string {
	installDir := getInstallDir()
	if installDir == "" {
		return ""
	}
	return filepath.Join(installDir, ".update.lock")
}

// tryAcquireCheckLock attempts to create the lock file atomically.
// Also cleans up stale locks older than 5 minutes.
func tryAcquireCheckLock(path string) bool {
	// Check for stale lock
	info, err := os.Stat(path)
	if err == nil {
		if time.Since(info.ModTime()) > 5*time.Minute {
			internal.DebugPrint("Removing stale update-check lock")
			os.Remove(path)
		}
	}

	// Atomic creation
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return false
	}

	// Write PID and timestamp
	fmt.Fprintf(f, "%d %s", os.Getpid(), time.Now().UTC().Format(time.RFC3339))
	f.Close()
	return true
}

func releaseCheckLock(path string) {
	os.Remove(path)
}

func tryAcquireBinaryLock(path string) bool {
	// Check for stale lock (10 minute timeout for binary operations)
	info, err := os.Stat(path)
	if err == nil {
		if time.Since(info.ModTime()) > 10*time.Minute {
			internal.DebugPrint("Removing stale binary lock")
			os.Remove(path)
		}
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return false
	}
	fmt.Fprintf(f, "%d %s", os.Getpid(), time.Now().UTC().Format(time.RFC3339))
	f.Close()
	return true
}

func releaseBinaryLock(path string) {
	os.Remove(path)
}

// credentialsHaveEnoughTime checks if the credential expiration is far enough in the future.
func credentialsHaveEnoughTime(expiration string) bool {
	if expiration == "" {
		return true
	}
	expiration = strings.Replace(expiration, "+00:00", "Z", 1)
	t, err := time.Parse(time.RFC3339, expiration)
	if err != nil {
		return true
	}
	remaining := time.Until(t)
	return remaining > 10*time.Minute
}

// cacheKeyForExpiration extracts the Expiration field from credentials in various formats.
func extractExpiration(creds interface{}) string {
	switch c := creds.(type) {
	case map[string]interface{}:
		if exp, ok := c["Expiration"].(string); ok {
			return exp
		}
	}
	return ""
}

// FormatEnvCredentials extracts credential values from the output for passing as env vars.
func FormatEnvCredentials(creds interface{}) (accessKeyID, secretKey, sessionToken, expiration string) {
	switch c := creds.(type) {
	case map[string]interface{}:
		accessKeyID, _ = c["AccessKeyId"].(string)
		secretKey, _ = c["SecretAccessKey"].(string)
		sessionToken, _ = c["SessionToken"].(string)
		expiration, _ = c["Expiration"].(string)
	}
	return
}

// SetCredentialEnvVars sets AWS credential environment variables for the current process.
func SetCredentialEnvVars(accessKeyID, secretKey, sessionToken string) {
	os.Setenv("AWS_ACCESS_KEY_ID", accessKeyID)
	os.Setenv("AWS_SECRET_ACCESS_KEY", secretKey)
	os.Setenv("AWS_SESSION_TOKEN", sessionToken)
}

// ParseExpiration parses a credential expiration string into remaining minutes.
func ParseExpiration(expiration string) (remainingMinutes int, err error) {
	if expiration == "" {
		return 0, fmt.Errorf("empty expiration")
	}
	expiration = strings.Replace(expiration, "+00:00", "Z", 1)
	t, err := time.Parse(time.RFC3339, expiration)
	if err != nil {
		return 0, err
	}
	remaining := time.Until(t)
	return int(remaining.Minutes()), nil
}

// resolveProfile determines the profile name from env var, auto-detection, or default.
func resolveProfile() string {
	profile := os.Getenv("CCWB_PROFILE")
	if profile == "" {
		profile = config.AutoDetectProfile()
	}
	if profile == "" {
		profile = "ClaudeCode"
	}
	return profile
}

// loadUpdateConfig loads the profile config from config.json for the self-update process.
// Returns the resolved profile name and config (config may be nil on error).
func loadUpdateConfig() (string, *config.ProfileConfig) {
	profile := resolveProfile()
	cfg, err := config.LoadConfig(profile)
	if err != nil {
		internal.DebugPrint("Failed to load config.json for profile %q: %v", profile, err)
		return profile, nil
	}
	internal.DebugPrint("Loaded config.json: profile=%q bucket=%q region=%q", profile, cfg.UpdateBucket, cfg.UpdateRegion)
	return profile, cfg
}

// loadCachedCredentials reads AWS credentials from ~/.aws/credentials for the given profile.
func loadCachedCredentials(profile string) (accessKeyID, secretKey, sessionToken string) {
	creds := credentials.ReadFromCredentialsFile(profile)
	if creds == nil {
		internal.DebugPrint("No cached credentials found in credentials file for profile %q", profile)
		return "", "", ""
	}
	if creds.AccessKeyID == "EXPIRED" {
		internal.DebugPrint("Cached credentials are expired/cleared for profile %q", profile)
		return "", "", ""
	}
	internal.DebugPrint("Using cached credentials from credentials file for profile %q", profile)
	return creds.AccessKeyID, creds.SecretAccessKey, creds.SessionToken
}

// staleMinutes is the staleness threshold for the check lock in minutes
const staleMinutes = 5

func init() {
	// Validate staleMinutes at compile time
	_ = strconv.Itoa(staleMinutes)
}

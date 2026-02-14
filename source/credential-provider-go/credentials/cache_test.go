package credentials

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndReadCredentials(t *testing.T) {
	// Use temp dir for HOME to avoid touching real credentials
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	// Create .aws directory
	os.MkdirAll(filepath.Join(tmpHome, ".aws"), 0700)

	creds := &AWSCredentialOutput{
		Version:         1,
		AccessKeyID:     "ASIATESTACCESSKEY",
		SecretAccessKey: "testsecretkey123456",
		SessionToken:    "FwoGZXIvYXdzE..." + "longtoken",
		Expiration:      time.Now().UTC().Add(1 * time.Hour).Format("2006-01-02T15:04:05Z"),
	}

	err := SaveToCredentialsFile(creds, "TestProfile")
	if err != nil {
		t.Fatalf("failed to save credentials: %v", err)
	}

	// Read back
	read := ReadFromCredentialsFile("TestProfile")
	if read == nil {
		t.Fatal("expected credentials, got nil")
	}

	if read.AccessKeyID != creds.AccessKeyID {
		t.Errorf("AccessKeyID mismatch: got %s, want %s", read.AccessKeyID, creds.AccessKeyID)
	}
	if read.SecretAccessKey != creds.SecretAccessKey {
		t.Errorf("SecretAccessKey mismatch: got %s, want %s", read.SecretAccessKey, creds.SecretAccessKey)
	}
	if read.SessionToken != creds.SessionToken {
		t.Errorf("SessionToken mismatch: got %s, want %s", read.SessionToken, creds.SessionToken)
	}
	if read.Expiration != creds.Expiration {
		t.Errorf("Expiration mismatch: got %s, want %s", read.Expiration, creds.Expiration)
	}
	if read.Version != 1 {
		t.Errorf("Version mismatch: got %d, want 1", read.Version)
	}
}

func TestReadCredentials_NoFile(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	result := ReadFromCredentialsFile("NonExistent")
	if result != nil {
		t.Error("expected nil for missing credentials file")
	}
}

func TestReadCredentials_MissingProfile(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	// Create a credentials file with a different profile
	awsDir := filepath.Join(tmpHome, ".aws")
	os.MkdirAll(awsDir, 0700)
	content := "[OtherProfile]\naws_access_key_id = AKIATEST\naws_secret_access_key = secret\naws_session_token = token\n"
	os.WriteFile(filepath.Join(awsDir, "credentials"), []byte(content), 0600)

	result := ReadFromCredentialsFile("MissingProfile")
	if result != nil {
		t.Error("expected nil for missing profile section")
	}
}

func TestGetCachedCredentials_Valid(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	os.MkdirAll(filepath.Join(tmpHome, ".aws"), 0700)

	futureExp := time.Now().UTC().Add(2 * time.Hour).Format("2006-01-02T15:04:05Z")
	creds := &AWSCredentialOutput{
		Version:         1,
		AccessKeyID:     "ASIATESTACCESSKEY",
		SecretAccessKey: "secret",
		SessionToken:    "token",
		Expiration:      futureExp,
	}
	SaveToCredentialsFile(creds, "Test")

	cached := GetCachedCredentials("Test")
	if cached == nil {
		t.Fatal("expected valid cached credentials")
	}
	if cached.AccessKeyID != "ASIATESTACCESSKEY" {
		t.Errorf("unexpected AccessKeyID: %s", cached.AccessKeyID)
	}
}

func TestGetCachedCredentials_Expired(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	os.MkdirAll(filepath.Join(tmpHome, ".aws"), 0700)

	pastExp := time.Now().UTC().Add(-1 * time.Hour).Format("2006-01-02T15:04:05Z")
	creds := &AWSCredentialOutput{
		Version:         1,
		AccessKeyID:     "ASIATESTACCESSKEY",
		SecretAccessKey: "secret",
		SessionToken:    "token",
		Expiration:      pastExp,
	}
	SaveToCredentialsFile(creds, "Test")

	cached := GetCachedCredentials("Test")
	if cached != nil {
		t.Error("expected nil for expired credentials")
	}
}

func TestGetCachedCredentials_DummyExpired(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	os.MkdirAll(filepath.Join(tmpHome, ".aws"), 0700)

	creds := &AWSCredentialOutput{
		Version:         1,
		AccessKeyID:     "EXPIRED",
		SecretAccessKey: "EXPIRED",
		SessionToken:    "EXPIRED",
		Expiration:      "2000-01-01T00:00:00Z",
	}
	SaveToCredentialsFile(creds, "Test")

	cached := GetCachedCredentials("Test")
	if cached != nil {
		t.Error("expected nil for dummy expired credentials")
	}
}

func TestGetCachedCredentials_ExpiringWithin30Seconds(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	os.MkdirAll(filepath.Join(tmpHome, ".aws"), 0700)

	// Expires in 20 seconds - within 30s buffer
	nearExp := time.Now().UTC().Add(20 * time.Second).Format("2006-01-02T15:04:05Z")
	creds := &AWSCredentialOutput{
		Version:         1,
		AccessKeyID:     "ASIATESTACCESSKEY",
		SecretAccessKey: "secret",
		SessionToken:    "token",
		Expiration:      nearExp,
	}
	SaveToCredentialsFile(creds, "Test")

	cached := GetCachedCredentials("Test")
	if cached != nil {
		t.Error("expected nil for credentials expiring within 30s buffer")
	}
}

func TestCheckExpiration(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	os.MkdirAll(filepath.Join(tmpHome, ".aws"), 0700)

	// Valid credentials
	futureExp := time.Now().UTC().Add(2 * time.Hour).Format("2006-01-02T15:04:05Z")
	creds := &AWSCredentialOutput{
		Version:         1,
		AccessKeyID:     "ASIATESTACCESSKEY",
		SecretAccessKey: "secret",
		SessionToken:    "token",
		Expiration:      futureExp,
	}
	SaveToCredentialsFile(creds, "Test")

	if CheckExpiration("Test") {
		t.Error("expected not expired for future credentials")
	}

	// Missing profile
	if !CheckExpiration("NonExistent") {
		t.Error("expected expired for missing profile")
	}
}

func TestClearCredentials(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	os.MkdirAll(filepath.Join(tmpHome, ".aws"), 0700)

	creds := &AWSCredentialOutput{
		Version:         1,
		AccessKeyID:     "ASIATESTACCESSKEY",
		SecretAccessKey: "secret",
		SessionToken:    "token",
		Expiration:      time.Now().UTC().Add(1 * time.Hour).Format("2006-01-02T15:04:05Z"),
	}
	SaveToCredentialsFile(creds, "Test")

	cleared := ClearCredentials("Test")
	if len(cleared) == 0 {
		t.Error("expected at least one cleared item")
	}

	// Should now read as EXPIRED
	read := ReadFromCredentialsFile("Test")
	if read == nil {
		t.Fatal("expected credentials entry to still exist")
	}
	if read.AccessKeyID != "EXPIRED" {
		t.Errorf("expected EXPIRED, got %s", read.AccessKeyID)
	}
}

func TestAtomicWrite_Permissions(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	os.MkdirAll(filepath.Join(tmpHome, ".aws"), 0700)

	creds := &AWSCredentialOutput{
		Version:         1,
		AccessKeyID:     "ASIATEST",
		SecretAccessKey: "secret",
		SessionToken:    "token",
		Expiration:      "2030-01-01T00:00:00Z",
	}
	SaveToCredentialsFile(creds, "Test")

	credPath := filepath.Join(tmpHome, ".aws", "credentials")
	info, err := os.Stat(credPath)
	if err != nil {
		t.Fatalf("failed to stat credentials file: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("expected 0600 permissions, got %o", perm)
	}
}

func TestParseExpiration_BothFormats(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"2030-01-01T00:00:00Z", true},
		{"2030-01-01T00:00:00+00:00", true},
		{"2030-01-01T12:30:45Z", true},
		{"not-a-date", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, err := parseExpiration(tt.input)
			if tt.valid && err != nil {
				t.Errorf("expected valid parse for %q, got error: %v", tt.input, err)
			}
			if !tt.valid && err == nil {
				t.Errorf("expected error for %q", tt.input)
			}
		})
	}
}

func TestSaveCredentials_PreservesOtherProfiles(t *testing.T) {
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	awsDir := filepath.Join(tmpHome, ".aws")
	os.MkdirAll(awsDir, 0700)

	// Write profile A
	credsA := &AWSCredentialOutput{
		Version: 1, AccessKeyID: "AKIA_A", SecretAccessKey: "secA", SessionToken: "tokA", Expiration: "2030-01-01T00:00:00Z",
	}
	SaveToCredentialsFile(credsA, "ProfileA")

	// Write profile B
	credsB := &AWSCredentialOutput{
		Version: 1, AccessKeyID: "AKIA_B", SecretAccessKey: "secB", SessionToken: "tokB", Expiration: "2030-01-01T00:00:00Z",
	}
	SaveToCredentialsFile(credsB, "ProfileB")

	// Verify both profiles exist
	readA := ReadFromCredentialsFile("ProfileA")
	readB := ReadFromCredentialsFile("ProfileB")

	if readA == nil || readA.AccessKeyID != "AKIA_A" {
		t.Error("ProfileA was not preserved")
	}
	if readB == nil || readB.AccessKeyID != "AKIA_B" {
		t.Error("ProfileB was not written correctly")
	}
}

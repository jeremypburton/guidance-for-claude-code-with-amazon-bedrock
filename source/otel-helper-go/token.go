package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// getTokenViaCredentialProcess runs the credential-process binary to retrieve
// a monitoring token. Returns empty string on failure.
func getTokenViaCredentialProcess() string {
	logInfo("Getting token via credential-process...")

	binaryName := "credential-process"
	if runtime.GOOS == "windows" {
		binaryName = "credential-process.exe"
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		logWarning("Could not determine home directory: %v", err)
		return ""
	}

	credentialProcess := filepath.Join(homeDir, "claude-code-with-bedrock", binaryName)

	if _, err := os.Stat(credentialProcess); os.IsNotExist(err) {
		logWarning("Credential process not found at %s", credentialProcess)
		return ""
	}

	profile := os.Getenv("AWS_PROFILE")
	if profile == "" {
		profile = "ClaudeCode"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, credentialProcess, "--profile", profile, "--get-monitoring-token")
	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			logWarning("Credential process timed out")
		} else {
			logWarning("Failed to get token via credential-process: %v", err)
		}
		return ""
	}

	token := strings.TrimSpace(string(output))
	if token == "" {
		logWarning("Could not get token via credential-process")
		return ""
	}

	logInfo("Successfully retrieved token via credential-process")
	return token
}

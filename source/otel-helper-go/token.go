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

	// Use same profile resolution order as credential-process:
	// CCWB_PROFILE > AWS_PROFILE > let credential-process auto-detect
	profile := os.Getenv("CCWB_PROFILE")
	if profile == "" {
		profile = os.Getenv("AWS_PROFILE")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	args := []string{"--get-monitoring-token"}
	if profile != "" {
		args = append([]string{"--profile", profile}, args...)
	}
	cmd := exec.CommandContext(ctx, credentialProcess, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			logWarning("Credential process timed out")
		} else {
			errMsg := strings.TrimSpace(stderr.String())
			if errMsg != "" {
				logWarning("Failed to get token via credential-process: %s", errMsg)
			} else {
				logWarning("Failed to get token via credential-process: %v", err)
			}
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

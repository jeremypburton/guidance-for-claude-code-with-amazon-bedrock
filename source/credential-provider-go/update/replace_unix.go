//go:build !windows

package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"credential-provider-go/internal"
)

// replaceBinary atomically replaces a binary file on Unix systems.
// Writes to .tmp, chmod 0755, then os.Rename to target.
// On macOS, strips quarantine attribute after replacement.
func replaceBinary(srcPath, destPath string) error {
	tmpPath := destPath + ".tmp"
	backupPath := destPath + ".backup"

	// Read source
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("failed to read source binary: %w", err)
	}

	// Write to .tmp
	if err := os.WriteFile(tmpPath, data, 0755); err != nil {
		return fmt.Errorf("failed to write temp binary: %w", err)
	}

	// Backup current binary (best-effort)
	if _, err := os.Stat(destPath); err == nil {
		currentData, err := os.ReadFile(destPath)
		if err == nil {
			os.WriteFile(backupPath, currentData, 0755)
		}
	}

	// Atomic rename
	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename binary: %w", err)
	}

	// macOS: remove quarantine attribute and ad-hoc codesign (required on Apple Silicon)
	if runtime.GOOS == "darwin" {
		cmd := exec.Command("xattr", "-d", "com.apple.quarantine", destPath)
		if err := cmd.Run(); err != nil {
			// Ignore errors — attribute may not exist
			internal.DebugPrint("xattr quarantine removal: %v (ignored)", err)
		}
		cmd = exec.Command("codesign", "-s", "-", "-f", destPath)
		if err := cmd.Run(); err != nil {
			internal.DebugPrint("ad-hoc codesign: %v (ignored)", err)
		}
	}

	return nil
}

// CleanupOldBinaries removes .backup files from the install directory.
func CleanupOldBinaries() {
	installDir := getInstallDir()
	if installDir == "" {
		return
	}

	entries, err := os.ReadDir(installDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".backup" {
			path := filepath.Join(installDir, entry.Name())
			if err := os.Remove(path); err == nil {
				internal.DebugPrint("Cleaned up backup file: %s", path)
			}
		}
	}
}

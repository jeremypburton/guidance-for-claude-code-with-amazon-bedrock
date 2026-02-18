//go:build windows

package update

import (
	"fmt"
	"os"
	"path/filepath"

	"credential-provider-go/internal"
)

// replaceBinary replaces a binary on Windows using the rename-to-.old strategy
// since running executables cannot be overwritten.
func replaceBinary(srcPath, destPath string) error {
	oldPath := destPath + ".old"

	// Remove any pre-existing .old file
	os.Remove(oldPath)

	// Rename current binary to .old (allows overwrite even if running)
	if _, err := os.Stat(destPath); err == nil {
		if err := os.Rename(destPath, oldPath); err != nil {
			return fmt.Errorf("failed to rename current binary to .old: %w", err)
		}
	}

	// Read source and write to destination
	data, err := os.ReadFile(srcPath)
	if err != nil {
		// Try to restore from .old
		os.Rename(oldPath, destPath)
		return fmt.Errorf("failed to read source binary: %w", err)
	}

	if err := os.WriteFile(destPath, data, 0755); err != nil {
		// Try to restore from .old
		os.Rename(oldPath, destPath)
		return fmt.Errorf("failed to write new binary: %w", err)
	}

	return nil
}

// CleanupOldBinaries removes .old files from previous Windows updates.
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
		if filepath.Ext(entry.Name()) == ".old" {
			path := filepath.Join(installDir, entry.Name())
			if err := os.Remove(path); err == nil {
				internal.DebugPrint("Cleaned up old binary: %s", path)
			}
		}
	}
}

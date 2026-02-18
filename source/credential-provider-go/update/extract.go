package update

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"credential-provider-go/internal"
)

const (
	maxFileSize        = 100 * 1024 * 1024 // 100 MB per file
	maxTotalDecompress = 500 * 1024 * 1024  // 500 MB total
)

// extractZip extracts a zip file to a temporary directory with safety checks.
// Returns the path to the temp directory containing extracted files.
func extractZip(zipPath string) (string, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("failed to open zip: %w", err)
	}
	defer reader.Close()

	// Create temp directory for extraction
	tmpDir, err := os.MkdirTemp("", "ccwb-extract-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	var totalBytes int64

	for _, f := range reader.File {
		// Safety: reject absolute paths
		if filepath.IsAbs(f.Name) {
			os.RemoveAll(tmpDir)
			return "", fmt.Errorf("zip contains absolute path: %s", f.Name)
		}

		// Safety: reject path traversal
		cleanName := filepath.Clean(f.Name)
		if strings.Contains(cleanName, "..") {
			os.RemoveAll(tmpDir)
			return "", fmt.Errorf("zip contains path traversal: %s", f.Name)
		}

		// Safety: reject files larger than limit (uncompressed)
		if f.UncompressedSize64 > maxFileSize {
			os.RemoveAll(tmpDir)
			return "", fmt.Errorf("zip entry too large: %s (%d bytes)", f.Name, f.UncompressedSize64)
		}

		targetPath := filepath.Join(tmpDir, cleanName)

		// Ensure the target path is within tmpDir
		if !strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(tmpDir)) {
			os.RemoveAll(tmpDir)
			return "", fmt.Errorf("zip entry escapes target directory: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				os.RemoveAll(tmpDir)
				return "", fmt.Errorf("failed to create directory: %w", err)
			}
			continue
		}

		// Create parent directories
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			os.RemoveAll(tmpDir)
			return "", fmt.Errorf("failed to create parent dir: %w", err)
		}

		// Extract file with size limit
		if err := extractFile(f, targetPath, &totalBytes); err != nil {
			os.RemoveAll(tmpDir)
			return "", err
		}
	}

	internal.DebugPrint("Extracted %d bytes total to %s", totalBytes, tmpDir)
	return tmpDir, nil
}

// extractFile extracts a single file from the zip with decompression limits.
func extractFile(f *zip.File, targetPath string, totalBytes *int64) error {
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("failed to open zip entry %s: %w", f.Name, err)
	}
	defer rc.Close()

	outFile, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", targetPath, err)
	}
	defer outFile.Close()

	// Limit read to prevent decompression bombs
	remaining := maxTotalDecompress - *totalBytes
	if remaining <= 0 {
		return fmt.Errorf("total decompression limit exceeded (%d bytes)", maxTotalDecompress)
	}

	limited := io.LimitReader(rc, remaining+1) // +1 to detect overflow
	n, err := io.Copy(outFile, limited)
	if err != nil {
		return fmt.Errorf("failed to extract %s: %w", f.Name, err)
	}

	*totalBytes += n
	if *totalBytes > maxTotalDecompress {
		return fmt.Errorf("total decompression limit exceeded (%d bytes)", maxTotalDecompress)
	}

	return nil
}

// validateExtractedFiles checks that all required files exist in the extraction directory.
// The extraction might contain a nested directory (e.g., "claude-code-package/").
func validateExtractedFiles(extractDir string, requiredBinary string) (string, error) {
	// Check if files are directly in extractDir or in a subdirectory
	baseDir := extractDir

	// Look for the binary in the base directory first
	if _, err := os.Stat(filepath.Join(baseDir, requiredBinary)); err != nil {
		// Check one level of subdirectory (e.g., claude-code-package/)
		entries, err := os.ReadDir(extractDir)
		if err != nil {
			return "", fmt.Errorf("failed to read extract dir: %w", err)
		}

		found := false
		for _, entry := range entries {
			if entry.IsDir() {
				subDir := filepath.Join(extractDir, entry.Name())
				if _, err := os.Stat(filepath.Join(subDir, requiredBinary)); err == nil {
					baseDir = subDir
					found = true
					break
				}
			}
		}

		if !found {
			return "", fmt.Errorf("required binary not found: %s", requiredBinary)
		}
	}

	return baseDir, nil
}

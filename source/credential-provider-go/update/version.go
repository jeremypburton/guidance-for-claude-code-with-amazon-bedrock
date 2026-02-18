package update

import (
	"strconv"
	"strings"
)

// semver represents a parsed semantic version.
type semver struct {
	Major      int
	Minor      int
	Patch      int
	PreRelease string
}

// parseSemver parses a version string like "1.2.3", "1.2.3-beta", or "1.2.3+sha".
// Returns nil if the version cannot be parsed (e.g., "dev", "unknown", "").
func parseSemver(version string) *semver {
	if version == "" || version == "dev" || version == "unknown" {
		return nil
	}

	// Strip build metadata (everything after +)
	if idx := strings.Index(version, "+"); idx >= 0 {
		version = version[:idx]
	}

	// Separate pre-release tag (everything after -)
	preRelease := ""
	if idx := strings.Index(version, "-"); idx >= 0 {
		preRelease = version[idx+1:]
		version = version[:idx]
	}

	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return nil
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil || major < 0 {
		return nil
	}

	minor, err := strconv.Atoi(parts[1])
	if err != nil || minor < 0 {
		return nil
	}

	patch, err := strconv.Atoi(parts[2])
	if err != nil || patch < 0 {
		return nil
	}

	return &semver{
		Major:      major,
		Minor:      minor,
		Patch:      patch,
		PreRelease: preRelease,
	}
}

// isNewerVersion returns true if remote is a newer version than local.
// Returns false if either version cannot be parsed.
// Does not auto-update to pre-release versions if local is a stable release.
func isNewerVersion(remote, local string) bool {
	r := parseSemver(remote)
	l := parseSemver(local)

	if r == nil || l == nil {
		return false
	}

	// Don't auto-update from stable to pre-release
	if r.PreRelease != "" && l.PreRelease == "" {
		return false
	}

	// Compare major.minor.patch
	if r.Major != l.Major {
		return r.Major > l.Major
	}
	if r.Minor != l.Minor {
		return r.Minor > l.Minor
	}
	return r.Patch > l.Patch
}

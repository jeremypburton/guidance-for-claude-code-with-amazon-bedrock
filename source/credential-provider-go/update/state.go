package update

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"credential-provider-go/internal"
)

// updateState represents the persisted auto-update state.
type updateState struct {
	LastCheckTime  string `json:"last_check_time"`
	PendingVersion string `json:"pending_version"`
	PendingMessage string `json:"pending_message"`
	UpdateError    string `json:"update_error"`
	LastUpdateTime string `json:"last_update_time"`
}

// stateFilePath returns the path to the update state file.
// Stored in the install directory (~/claude-code-with-bedrock/) alongside
// config.json and the binaries it tracks.
func stateFilePath() string {
	installDir := getInstallDir()
	if installDir == "" {
		return ""
	}
	return filepath.Join(installDir, "update-state.json")
}

// loadState reads the update state from disk.
// Returns a zero-value state if the file doesn't exist or is corrupted.
func loadState() updateState {
	path := stateFilePath()
	if path == "" {
		return updateState{}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return updateState{}
	}

	var state updateState
	if err := json.Unmarshal(data, &state); err != nil {
		// Corruption recovery: rename to .corrupted and start fresh
		internal.DebugPrint("Corrupted update state file, resetting: %v", err)
		corruptedPath := path + ".corrupted"
		os.Rename(path, corruptedPath)
		return updateState{}
	}

	return state
}

// saveState writes the update state to disk with 0600 permissions.
func saveState(state updateState) {
	path := stateFilePath()
	if path == "" {
		return
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		internal.DebugPrint("Failed to create state directory: %v", err)
		return
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		internal.DebugPrint("Failed to marshal update state: %v", err)
		return
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		internal.DebugPrint("Failed to write update state: %v", err)
	}
}

// lastCheckTime returns the time of the last update check, or zero time if never checked.
func lastCheckTime() time.Time {
	state := loadState()
	if state.LastCheckTime == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, state.LastCheckTime)
	if err != nil {
		return time.Time{}
	}
	return t
}

// recordCheckTime updates the last check time to now.
func recordCheckTime() {
	state := loadState()
	state.LastCheckTime = time.Now().UTC().Format(time.RFC3339)
	saveState(state)
}

// recordPendingUpdate records that a background update completed successfully.
func recordPendingUpdate(version string) {
	state := loadState()
	state.PendingVersion = version
	state.PendingMessage = "Auto-updated to version " + version
	state.UpdateError = ""
	state.LastUpdateTime = time.Now().UTC().Format(time.RFC3339)
	saveState(state)
}

// recordUpdateError records an update error (sanitized category, not raw error).
func recordUpdateError(category string) {
	state := loadState()
	state.UpdateError = category
	saveState(state)
}

// consumePendingNotification reads and clears any pending update notification.
// Returns the message to display, or empty string if none.
func consumePendingNotification() string {
	state := loadState()
	if state.PendingMessage == "" {
		return ""
	}

	msg := state.PendingMessage
	state.PendingMessage = ""
	state.PendingVersion = ""
	saveState(state)
	return msg
}

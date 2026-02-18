package update

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"credential-provider-go/internal"
)

// mergeSettings merges a new settings.json template into the user's existing settings.
// For the "env" dict: new keys take precedence, but user-added custom keys are preserved.
// For top-level keys (awsAuthRefresh, otelHeadersHelper): new version wins.
func mergeSettings(existingPath, newTemplatePath, installDir string) error {
	// Read new template
	newData, err := os.ReadFile(newTemplatePath)
	if err != nil {
		return err
	}

	var newSettings map[string]interface{}
	if err := json.Unmarshal(newData, &newSettings); err != nil {
		return err
	}

	// Replace placeholders in new settings
	replacePlaceholders(newSettings, installDir)

	// Check if existing settings file exists
	existingData, err := os.ReadFile(existingPath)
	if err != nil {
		// No existing file — just write the new one
		return writeSettings(existingPath, newSettings)
	}

	var existingSettings map[string]interface{}
	if err := json.Unmarshal(existingData, &existingSettings); err != nil {
		// Existing file is corrupted — overwrite with new
		internal.DebugPrint("Existing settings.json is corrupted, overwriting")
		return writeSettings(existingPath, newSettings)
	}

	// Merge env dictionaries
	mergeEnvDicts(existingSettings, newSettings)

	// For top-level keys that have placeholders, new version wins
	topLevelKeys := []string{"awsAuthRefresh", "otelHeadersHelper"}
	for _, key := range topLevelKeys {
		if val, ok := newSettings[key]; ok {
			existingSettings[key] = val
		}
	}

	return writeSettings(existingPath, existingSettings)
}

// mergeEnvDicts merges the "env" dictionary from newSettings into existingSettings.
// New keys take precedence, but user-added custom keys in existing are preserved.
func mergeEnvDicts(existing, new map[string]interface{}) {
	newEnv, ok := new["env"].(map[string]interface{})
	if !ok {
		return
	}

	existingEnv, ok := existing["env"].(map[string]interface{})
	if !ok {
		// No existing env — use new one entirely
		existing["env"] = newEnv
		return
	}

	// New keys take precedence
	for k, v := range newEnv {
		existingEnv[k] = v
	}
	// User-added custom keys are preserved (they remain in existingEnv
	// since we only overwrote keys that exist in newEnv)

	existing["env"] = existingEnv
}

// replacePlaceholders replaces __CREDENTIAL_PROCESS_PATH__ and __OTEL_HELPER_PATH__
// in string values throughout the settings map.
func replacePlaceholders(settings map[string]interface{}, installDir string) {
	credPath := filepath.Join(installDir, "credential-process")
	otelPath := filepath.Join(installDir, "otel-helper")

	replacer := strings.NewReplacer(
		"__CREDENTIAL_PROCESS_PATH__", credPath,
		"__OTEL_HELPER_PATH__", otelPath,
	)

	replaceInMap(settings, replacer)
}

// replaceInMap recursively replaces placeholder strings in a map.
func replaceInMap(m map[string]interface{}, replacer *strings.Replacer) {
	for k, v := range m {
		switch val := v.(type) {
		case string:
			replaced := replacer.Replace(val)
			m[k] = replaced
			// Warn if any placeholders remain
			if strings.Contains(replaced, "__") && strings.Count(replaced, "__") >= 2 {
				internal.DebugPrint("Unresolved placeholder in settings key %q: %s", k, replaced)
			}
		case map[string]interface{}:
			replaceInMap(val, replacer)
		}
	}
}

// writeSettings writes settings to a JSON file with 0644 permissions.
func writeSettings(path string, settings map[string]interface{}) error {
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

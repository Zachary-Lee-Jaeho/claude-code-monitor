package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)


const ccmoHookURL = "http://localhost:7777/hooks/event"

// Hook event names that CCMO monitors.
var ccmoHookEvents = []string{
	"PreToolUse", "PostToolUse", "PostToolUseFailure",
	"Stop", "Notification",
}

type hookEntry struct {
	Type    string `json:"type"`
	URL     string `json:"url"`
	Timeout int    `json:"timeout"`
}

// InstallHooks adds CCMO hooks to ~/.claude/settings.json.
// Merges with existing hooks — does not overwrite.
func InstallHooks() (added int, err error) {
	settingsPath := claudeSettingsPath()
	settings, err := readSettingsJSON(settingsPath)
	if err != nil {
		return 0, err
	}

	hooks, _ := settings["hooks"].(map[string]interface{})
	if hooks == nil {
		hooks = make(map[string]interface{})
	}

	ccmoHook := map[string]interface{}{
		"type":    "http",
		"url":     ccmoHookURL,
		"timeout": 5,
	}

	for _, event := range ccmoHookEvents {
		eventHooks, _ := hooks[event].([]interface{})
		if hasHookURL(eventHooks, ccmoHookURL) {
			continue
		}
		eventHooks = append(eventHooks, ccmoHook)
		hooks[event] = eventHooks
		added++
	}

	settings["hooks"] = hooks
	return added, writeSettingsJSON(settingsPath, settings)
}

// RemoveHooks removes CCMO hooks from ~/.claude/settings.json.
func RemoveHooks() (removed int, err error) {
	settingsPath := claudeSettingsPath()
	settings, err := readSettingsJSON(settingsPath)
	if err != nil {
		return 0, err
	}

	hooks, _ := settings["hooks"].(map[string]interface{})
	if hooks == nil {
		return 0, nil
	}

	for event, val := range hooks {
		arr, ok := val.([]interface{})
		if !ok {
			continue
		}
		var filtered []interface{}
		for _, h := range arr {
			m, ok := h.(map[string]interface{})
			if ok {
				url, _ := m["url"].(string)
				if url == ccmoHookURL {
					removed++
					continue
				}
			}
			filtered = append(filtered, h)
		}
		if len(filtered) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = filtered
		}
	}

	if len(hooks) == 0 {
		delete(settings, "hooks")
	} else {
		settings["hooks"] = hooks
	}

	return removed, writeSettingsJSON(settingsPath, settings)
}

// HookStatus returns which CCMO hook events are currently installed.
func HookStatus() ([]string, error) {
	settingsPath := claudeSettingsPath()
	settings, err := readSettingsJSON(settingsPath)
	if err != nil {
		return nil, err
	}

	hooks, _ := settings["hooks"].(map[string]interface{})
	if hooks == nil {
		return nil, nil
	}

	var installed []string
	for event, val := range hooks {
		arr, ok := val.([]interface{})
		if !ok {
			continue
		}
		if hasHookURL(arr, ccmoHookURL) {
			installed = append(installed, event)
		}
	}
	return installed, nil
}

func hasHookURL(hooks []interface{}, url string) bool {
	for _, h := range hooks {
		m, ok := h.(map[string]interface{})
		if ok {
			u, _ := m["url"].(string)
			if u == url {
				return true
			}
		}
	}
	return false
}

func claudeSettingsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "settings.json")
}

func readSettingsJSON(path string) (map[string]interface{}, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return make(map[string]interface{}), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var settings map[string]interface{}
	if err := json.Unmarshal(raw, &settings); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return settings, nil
}

func writeSettingsJSON(path string, settings map[string]interface{}) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create directory %s: %w", filepath.Dir(path), err)
	}
	raw, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o600)
}

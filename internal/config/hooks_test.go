package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func setupTempSettings(t *testing.T, content string) (string, func()) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if content != "" {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	origHome := os.Getenv("HOME")
	// Create .claude dir structure
	claudeDir := filepath.Join(dir, ".claude")
	os.MkdirAll(claudeDir, 0o700)
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if content != "" {
		os.WriteFile(settingsPath, []byte(content), 0o600)
	}
	os.Setenv("HOME", dir)
	return settingsPath, func() { os.Setenv("HOME", origHome) }
}

func TestInstallHooksEmpty(t *testing.T) {
	_, cleanup := setupTempSettings(t, "")
	defer cleanup()

	added, err := InstallHooks()
	if err != nil {
		t.Fatal(err)
	}
	if added != len(ccmoHookEvents) {
		t.Errorf("expected %d hooks added, got %d", len(ccmoHookEvents), added)
	}
}

func TestInstallHooksIdempotent(t *testing.T) {
	_, cleanup := setupTempSettings(t, "")
	defer cleanup()

	InstallHooks()
	added, err := InstallHooks()
	if err != nil {
		t.Fatal(err)
	}
	if added != 0 {
		t.Errorf("expected 0 hooks added on second install, got %d", added)
	}
}

func TestRemoveHooks(t *testing.T) {
	_, cleanup := setupTempSettings(t, "")
	defer cleanup()

	InstallHooks()
	removed, err := RemoveHooks()
	if err != nil {
		t.Fatal(err)
	}
	if removed != len(ccmoHookEvents) {
		t.Errorf("expected %d hooks removed, got %d", len(ccmoHookEvents), removed)
	}
}

func TestRemoveHooksNotInstalled(t *testing.T) {
	_, cleanup := setupTempSettings(t, "")
	defer cleanup()

	removed, err := RemoveHooks()
	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 {
		t.Errorf("expected 0 hooks removed, got %d", removed)
	}
}

func TestPreserveExistingHooks(t *testing.T) {
	existing := `{"hooks":{"PreToolUse":[{"type":"http","url":"http://other-service/hook","timeout":10}]}}`
	_, cleanup := setupTempSettings(t, existing)
	defer cleanup()

	added, err := InstallHooks()
	if err != nil {
		t.Fatal(err)
	}
	if added != len(ccmoHookEvents) {
		t.Errorf("expected %d hooks added, got %d", len(ccmoHookEvents), added)
	}

	// Verify existing hook is preserved
	settingsPath := claudeSettingsPath()
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]interface{}
	json.Unmarshal(raw, &settings)
	hooks := settings["hooks"].(map[string]interface{})
	preToolUse := hooks["PreToolUse"].([]interface{})
	if len(preToolUse) != 2 {
		t.Errorf("expected 2 PreToolUse hooks (existing + ccmo), got %d", len(preToolUse))
	}
}

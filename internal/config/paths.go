package config

import (
	"os"
	"path/filepath"
)

// AppDir returns ~/.ccmo
func AppDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ccmo")
}

// ConfigPath returns ~/.ccmo/config.json
func ConfigPath() string {
	return filepath.Join(AppDir(), "config.json")
}

// UsageCachePath returns ~/.ccmo/usage.json
func UsageCachePath() string {
	return filepath.Join(AppDir(), "usage.json")
}

// ServersPath returns ~/.ccmo/servers.json
func ServersPath() string {
	return filepath.Join(AppDir(), "servers.json")
}

// UIStatePath returns ~/.ccmo/ui-state.json
func UIStatePath() string {
	return filepath.Join(AppDir(), "ui-state.json")
}

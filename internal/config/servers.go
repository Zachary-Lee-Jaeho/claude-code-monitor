package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/jaeho/ccmo/internal/security"
)

// ServerConfig represents a remote server connection.
type ServerConfig struct {
	Name         string `json:"name"`
	Host         string `json:"host"` // user@hostname
	Port         int    `json:"port"`
	IdentityFile string `json:"identityFile,omitempty"`
	ClaudeDir    string `json:"claudeDir,omitempty"` // defaults to ~/.claude
	Enabled      bool   `json:"enabled"`
}

type serversFile struct {
	Servers []ServerConfig `json:"servers"`
}

// LoadServers loads the server list from ~/.ccmo/servers.json.
func LoadServers() ([]ServerConfig, error) {
	raw, err := os.ReadFile(ServersPath())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var sf serversFile
	if err := json.Unmarshal(raw, &sf); err != nil {
		return nil, fmt.Errorf("parse servers.json: %w", err)
	}
	// Default claudeDir
	for i := range sf.Servers {
		if sf.Servers[i].ClaudeDir == "" {
			sf.Servers[i].ClaudeDir = "~/.claude"
		}
		if sf.Servers[i].Port == 0 {
			sf.Servers[i].Port = 22
		}
	}
	return sf.Servers, nil
}

// SaveServers writes the server list to ~/.ccmo/servers.json.
func SaveServers(servers []ServerConfig) error {
	if err := security.EnsureDir(AppDir()); err != nil {
		return err
	}
	sf := serversFile{Servers: servers}
	raw, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return err
	}
	path := ServersPath()
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return err
	}
	return security.EnsureFilePermissions(path, 0o600)
}

// AddServer adds a server to the config. Returns error if name already exists.
func AddServer(name, host string, port int, keyFile string) error {
	servers, err := LoadServers()
	if err != nil {
		return err
	}
	for _, s := range servers {
		if s.Name == name {
			return fmt.Errorf("server %q already exists", name)
		}
	}
	servers = append(servers, ServerConfig{
		Name:         name,
		Host:         host,
		Port:         port,
		IdentityFile: keyFile,
		Enabled:      true,
	})
	return SaveServers(servers)
}

// RemoveServer removes a server by name.
func RemoveServer(name string) error {
	servers, err := LoadServers()
	if err != nil {
		return err
	}
	var filtered []ServerConfig
	found := false
	for _, s := range servers {
		if s.Name == name {
			found = true
		} else {
			filtered = append(filtered, s)
		}
	}
	if !found {
		return fmt.Errorf("server %q not found", name)
	}
	return SaveServers(filtered)
}

// GetServer returns a server by name.
func GetServer(name string) (*ServerConfig, error) {
	servers, err := LoadServers()
	if err != nil {
		return nil, err
	}
	for i := range servers {
		if servers[i].Name == name {
			return &servers[i], nil
		}
	}
	return nil, fmt.Errorf("server %q not found", name)
}

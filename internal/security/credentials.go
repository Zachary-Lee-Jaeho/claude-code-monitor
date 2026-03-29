package security

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type oauthCreds struct {
	ClaudeAiOauth struct {
		AccessToken string `json:"accessToken"`
	} `json:"claudeAiOauth"`
}

// credentialsMtime tracks the last known mtime of credentials.json
// to detect when Claude Code refreshes the token.
var (
	credMu       sync.Mutex
	credLastMtime time.Time
)

func credentialsPath() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".claude", ".credentials.json")
}

// CredentialsChanged reports whether credentials.json has been modified
// since the last call to ReadOAuthToken. Used for 401 retry logic.
func CredentialsChanged() bool {
	credPath := credentialsPath()
	if credPath == "" {
		return false
	}
	info, err := os.Stat(credPath)
	if err != nil {
		return false
	}
	credMu.Lock()
	defer credMu.Unlock()
	return info.ModTime().After(credLastMtime)
}

// ReadOAuthToken reads the OAuth token from ~/.claude/.credentials.json.
// Returns token as []byte for secure zeroing after use.
// Caller MUST zero the returned slice: `for i := range t { t[i] = 0 }`
func ReadOAuthToken() ([]byte, error) {
	credPath := credentialsPath()
	if credPath == "" {
		return nil, fmt.Errorf("cannot determine home dir")
	}

	// Check permissions
	if err := CheckFilePermissions(credPath, 0o600); err != nil {
		// Warn but don't fail — not our file to fix
	}

	info, err := os.Stat(credPath)
	if err != nil {
		return nil, fmt.Errorf("cannot stat credentials: %w", err)
	}

	data, err := os.ReadFile(credPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read credentials: %w", err)
	}

	var creds oauthCreds
	if err := json.Unmarshal(data, &creds); err != nil {
		// Zero the raw data before returning error
		for i := range data {
			data[i] = 0
		}
		return nil, fmt.Errorf("cannot parse credentials: %w", err)
	}

	token := []byte(creds.ClaudeAiOauth.AccessToken)

	// Zero the raw data — token is now in its own slice
	for i := range data {
		data[i] = 0
	}

	if len(token) == 0 {
		return nil, fmt.Errorf("empty OAuth token in %s", credPath)
	}

	// Track mtime for change detection
	credMu.Lock()
	credLastMtime = info.ModTime()
	credMu.Unlock()

	return token, nil
}

// HasCredentials checks if the credentials file exists.
func HasCredentials() bool {
	credPath := credentialsPath()
	if credPath == "" {
		return false
	}
	_, err := os.Stat(credPath)
	return err == nil
}

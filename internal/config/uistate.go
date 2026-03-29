package config

import (
	"encoding/json"
	"os"
	"time"
)

// UIState holds persistent UI state across CCMO restarts.
type UIState struct {
	SelectedSession string    `json:"selectedSession,omitempty"`
	Filter          string    `json:"filter,omitempty"`
	SortColumn      string    `json:"sortColumn,omitempty"`
	LastExitTime    time.Time `json:"lastExitTime,omitempty"`
}

// LoadUIState reads UI state from ~/.ccmo/ui-state.json.
// Returns zero-value UIState on any error (missing file, corruption, etc.).
func LoadUIState() UIState {
	data, err := os.ReadFile(UIStatePath())
	if err != nil {
		return UIState{}
	}
	var s UIState
	if json.Unmarshal(data, &s) != nil {
		return UIState{}
	}
	return s
}

// SaveUIState writes UI state to ~/.ccmo/ui-state.json with 0600 permissions.
func SaveUIState(s UIState) error {
	s.LastExitTime = time.Now()
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(UIStatePath(), data, 0o600)
}

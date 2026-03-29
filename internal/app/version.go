package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Version is set at build time via -ldflags.
var Version = "dev"

type githubRelease struct {
	TagName string `json:"tag_name"`
}

// CheckUpdate checks GitHub for a newer release (non-blocking).
// Returns (latest_version, true) if an update is available.
func CheckUpdate(repo string) (string, bool) {
	client := &http.Client{Timeout: 3 * time.Second}
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)

	resp, err := client.Get(url)
	if err != nil || resp.StatusCode != 200 {
		return "", false
	}
	defer resp.Body.Close()

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", false
	}

	if release.TagName != "" && release.TagName != Version && release.TagName != "v"+Version {
		return release.TagName, true
	}
	return "", false
}

package data

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jaeho/ccmo/internal/security"
)

// UsageFetcher fetches OAuth usage data in the background.
type UsageFetcher struct {
	mu          sync.Mutex
	Data        UsageData
	LastError   string
	cooldown    time.Duration
	lastAttempt time.Time
	pending     bool
}

func NewUsageFetcher() *UsageFetcher {
	f := &UsageFetcher{
		cooldown: 300 * time.Second, // idle default
	}
	// Load cached data from disk
	f.loadCache()
	// If cache exists, suppress immediate API call — wait for cooldown
	if f.Data.IsAvailable() {
		f.lastAttempt = time.Now()
	}
	return f
}

// SetActiveMode reduces cooldown when sessions are active.
func (f *UsageFetcher) SetActiveMode(active bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if active {
		f.cooldown = 60 * time.Second
	} else {
		f.cooldown = 300 * time.Second
	}
}

// HasPending returns true if a fetch is in-flight.
func (f *UsageFetcher) HasPending() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pending
}

// MaybeRefresh starts a background fetch if cooldown has elapsed.
// Returns true if a new fetch was started.
func (f *UsageFetcher) MaybeRefresh() bool {
	f.mu.Lock()
	if f.pending || time.Since(f.lastAttempt) < f.cooldown {
		f.mu.Unlock()
		return false
	}
	f.pending = true
	f.lastAttempt = time.Now()
	f.mu.Unlock()

	go func() {
		data, err := fetchAPIUsage()
		f.mu.Lock()
		defer f.mu.Unlock()
		f.pending = false
		if err != nil {
			f.LastError = err.Error()
			// On rate limit, use longer backoff (5 min)
			if isRateLimited(err) {
				f.cooldown = 300 * time.Second
			}
		} else {
			f.Data = *data
			f.LastError = ""
			f.saveCache()
		}
	}()

	return true
}

// ForceRefresh starts an immediate fetch regardless of cooldown.
func (f *UsageFetcher) ForceRefresh() {
	f.mu.Lock()
	f.lastAttempt = time.Time{} // reset cooldown
	f.mu.Unlock()
	f.MaybeRefresh()
}

// GetData returns a copy of the current usage data.
func (f *UsageFetcher) GetData() UsageData {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Data
}

// GetLastError returns the last fetch error, if any.
func (f *UsageFetcher) GetLastError() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.LastError
}

// API response structures
type apiUsageResponse struct {
	FiveHour     *apiUsageWindow `json:"five_hour"`
	SevenDay     *apiUsageWindow `json:"seven_day"`
	ExtraUsage   *apiExtraUsage  `json:"extra_usage"`
}

type apiUsageWindow struct {
	Utilization float64 `json:"utilization"`
	ResetsAt    *string `json:"resets_at"`
}

type apiExtraUsage struct {
	Utilization  float64  `json:"utilization"`
	UsedCredits  *float64 `json:"used_credits"`
	MonthlyLimit *float64 `json:"monthly_limit"`
}

func fetchAPIUsage() (*UsageData, error) {
	d, err := doFetchAPIUsage()
	if err != nil && isUnauthorized(err) && security.CredentialsChanged() {
		// Token may have been refreshed by Claude Code — retry once
		d, err = doFetchAPIUsage()
	}
	return d, err
}

func doFetchAPIUsage() (*UsageData, error) {
	token, err := security.ReadOAuthToken()
	if err != nil {
		return nil, fmt.Errorf("read token: %w", err)
	}
	defer func() {
		for i := range token {
			token[i] = 0
		}
	}()

	req, err := http.NewRequest("GET", "https://api.anthropic.com/api/oauth/usage", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+string(token))
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("api request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("api returned %d: %s", resp.StatusCode, string(body))
	}

	var apiResp apiUsageResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	d := &UsageData{
		UpdatedAt: time.Now(),
	}

	if apiResp.FiveHour != nil {
		d.SessionPct = apiResp.FiveHour.Utilization
		if apiResp.FiveHour.ResetsAt != nil {
			d.SessionReset = formatResetTime(*apiResp.FiveHour.ResetsAt)
		}
	}

	if apiResp.SevenDay != nil {
		d.WeeklyPct = apiResp.SevenDay.Utilization
		if apiResp.SevenDay.ResetsAt != nil {
			d.WeeklyReset = formatResetTime(*apiResp.SevenDay.ResetsAt)
		}
	}

	if apiResp.ExtraUsage != nil {
		d.ExtraPct = apiResp.ExtraUsage.Utilization
		if apiResp.ExtraUsage.UsedCredits != nil && apiResp.ExtraUsage.MonthlyLimit != nil {
			d.ExtraSpent = fmt.Sprintf("$%.2f / $%.2f",
				*apiResp.ExtraUsage.UsedCredits, *apiResp.ExtraUsage.MonthlyLimit)
		}
	}

	return d, nil
}

func formatResetTime(iso string) string {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return iso
	}
	local := t.Local()
	return local.Format("Jan 02, 15:04")
}

func usageCachePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ccmo", "usage.json")
}

func isRateLimited(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return len(s) >= 16 && s[:16] == "api returned 429"
}

func isUnauthorized(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return len(s) >= 16 && s[:16] == "api returned 401"
}

// usageCacheFile is the on-disk format for cached usage data.
type usageCacheFile struct {
	SessionPct   float64 `json:"session_pct"`
	SessionReset string  `json:"session_reset"`
	WeeklyPct    float64 `json:"weekly_pct"`
	WeeklyReset  string  `json:"weekly_reset"`
	ExtraPct     float64 `json:"extra_pct"`
	ExtraSpent   string  `json:"extra_spent"`
	ExtraReset   string  `json:"extra_reset"`
	UpdatedAt    string  `json:"updated_at"`
}

func (f *UsageFetcher) loadCache() {
	path := usageCachePath()
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var cache usageCacheFile
	if err := json.Unmarshal(raw, &cache); err != nil {
		return
	}
	t, _ := time.Parse(time.RFC3339, cache.UpdatedAt)
	f.Data = UsageData{
		SessionPct:   cache.SessionPct,
		SessionReset: cache.SessionReset,
		WeeklyPct:    cache.WeeklyPct,
		WeeklyReset:  cache.WeeklyReset,
		ExtraPct:     cache.ExtraPct,
		ExtraSpent:   cache.ExtraSpent,
		ExtraReset:   cache.ExtraReset,
		UpdatedAt:    t,
	}
}

func (f *UsageFetcher) saveCache() {
	// must hold f.mu
	cache := usageCacheFile{
		SessionPct:   f.Data.SessionPct,
		SessionReset: f.Data.SessionReset,
		WeeklyPct:    f.Data.WeeklyPct,
		WeeklyReset:  f.Data.WeeklyReset,
		ExtraPct:     f.Data.ExtraPct,
		ExtraSpent:   f.Data.ExtraSpent,
		ExtraReset:   f.Data.ExtraReset,
		UpdatedAt:    f.Data.UpdatedAt.Format(time.RFC3339),
	}
	raw, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(usageCachePath(), raw, 0o600)
}

package data

import "testing"

func TestDecodeProjectPathHeuristic(t *testing.T) {
	tests := []struct {
		encoded string
		want    string
	}{
		{"-home-user-Projects-app", "/home/user/Projects/app"},
		{"-tmp-my-project", "/tmp/my/project"},
		{"", ""},
	}
	for _, tc := range tests {
		got := DecodeProjectPathHeuristic(tc.encoded)
		if got != tc.want {
			t.Errorf("DecodeProjectPathHeuristic(%q) = %q, want %q", tc.encoded, got, tc.want)
		}
	}
}

func TestPathCache(t *testing.T) {
	pc := make(PathCache)
	pc["test-key"] = "/decoded/path"

	if v, ok := pc["test-key"]; !ok || v != "/decoded/path" {
		t.Error("expected cache hit")
	}
	if _, ok := pc["missing"]; ok {
		t.Error("expected cache miss")
	}
}

func TestEffortPriority(t *testing.T) {
	tests := []struct {
		effort string
		want   int
	}{
		{"max", 3},
		{"high", 2},
		{"auto", 1},
		{"low", 0},
		{"unknown", 1},
	}
	for _, tc := range tests {
		got := EffortPriority(tc.effort)
		if got != tc.want {
			t.Errorf("EffortPriority(%q) = %d, want %d", tc.effort, got, tc.want)
		}
	}
}

func TestModelTierFrom(t *testing.T) {
	tests := []struct {
		name string
		want ModelTier
	}{
		{"claude-opus-4-6", TierOpus},
		{"claude-sonnet-4-6", TierSonnet},
		{"claude-haiku-4-5", TierHaiku},
		{"unknown", TierSonnet},
	}
	for _, tc := range tests {
		got := ModelTierFrom(tc.name)
		if got != tc.want {
			t.Errorf("ModelTierFrom(%q) = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestContextMax(t *testing.T) {
	if ContextMax("claude-opus-4-6") != 1_000_000 {
		t.Error("opus should have 1M context")
	}
	if ContextMax("claude-sonnet-4-6") != 200_000 {
		t.Error("sonnet should have 200k context")
	}
}

func TestTokenUsageHitRate(t *testing.T) {
	u := TokenUsage{
		InputTokens:          600,
		CacheReadInputTokens: 400,
	}
	rate := u.HitRate()
	// 400 / (600+400) * 100 = 40
	if rate < 39.9 || rate > 40.1 {
		t.Errorf("expected ~40%% hit rate, got %.2f", rate)
	}
}

func TestTokenUsageHitRateZero(t *testing.T) {
	u := TokenUsage{}
	if u.HitRate() != 0 {
		t.Error("expected 0 hit rate for zero tokens")
	}
}

func TestTokenUsageAdd(t *testing.T) {
	a := TokenUsage{InputTokens: 100, OutputTokens: 50}
	b := TokenUsage{InputTokens: 200, OutputTokens: 75, CacheReadInputTokens: 10}
	a.Add(&b)
	if a.InputTokens != 300 || a.OutputTokens != 125 || a.CacheReadInputTokens != 10 {
		t.Errorf("Add failed: got %+v", a)
	}
}

func TestUsageDataAvailability(t *testing.T) {
	var u UsageData
	if u.IsAvailable() {
		t.Error("zero UsageData should not be available")
	}
	if !u.IsStale() {
		t.Error("zero UsageData should be stale")
	}
}

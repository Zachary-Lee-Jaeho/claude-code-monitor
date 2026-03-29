package data

import (
	"math"
	"testing"
)

func assertCostApprox(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.0001 {
		t.Errorf("%s: got %.6f, want %.6f", label, got, want)
	}
}

func TestCalculateCostOpus(t *testing.T) {
	u := &TokenUsage{
		InputTokens:              1_000_000,
		OutputTokens:             1_000_000,
		CacheCreationInputTokens: 1_000_000,
		CacheReadInputTokens:     1_000_000,
	}
	cost, _ := CalculateCost("claude-opus-4-6", u)
	// 15 + 75 + 18.75 + 1.50 = 110.25
	assertCostApprox(t, "opus total", cost, 110.25)
}

func TestCalculateCostSonnet(t *testing.T) {
	u := &TokenUsage{
		InputTokens:              1_000_000,
		OutputTokens:             1_000_000,
		CacheCreationInputTokens: 1_000_000,
		CacheReadInputTokens:     1_000_000,
	}
	cost, _ := CalculateCost("claude-sonnet-4-6", u)
	// 3 + 15 + 3.75 + 0.30 = 22.05
	assertCostApprox(t, "sonnet total", cost, 22.05)
}

func TestCalculateCostHaiku(t *testing.T) {
	u := &TokenUsage{
		InputTokens:              1_000_000,
		OutputTokens:             1_000_000,
		CacheCreationInputTokens: 1_000_000,
		CacheReadInputTokens:     1_000_000,
	}
	cost, _ := CalculateCost("claude-haiku-4-5", u)
	// 0.25 + 1.25 + 0.30 + 0.03 = 1.83
	assertCostApprox(t, "haiku total", cost, 1.83)
}

func TestCacheSavings(t *testing.T) {
	u := &TokenUsage{
		InputTokens:          0,
		OutputTokens:         100_000,
		CacheReadInputTokens: 500_000,
	}
	_, saved := CalculateCost("claude-opus-4-6", u)
	// Without cache: (0+500k)/1M*15 + 100k/1M*75 = 7.5 + 7.5 = 15.0
	// With cache: 0 + 100k/1M*75 + 500k/1M*1.5 = 7.5 + 0.75 = 8.25
	// Saved = 15.0 - 8.25 = 6.75
	assertCostApprox(t, "cache savings", saved, 6.75)
}

func TestUnknownModel(t *testing.T) {
	u := &TokenUsage{InputTokens: 1000, OutputTokens: 1000}
	cost, _ := CalculateCost("unknown-model-xyz", u)
	// Falls back to Sonnet tier
	if cost <= 0 {
		t.Error("expected non-zero cost for unknown model (sonnet fallback)")
	}
}

func TestZeroTokens(t *testing.T) {
	u := &TokenUsage{}
	cost, saved := CalculateCost("claude-opus-4-6", u)
	assertCostApprox(t, "zero cost", cost, 0)
	assertCostApprox(t, "zero saved", saved, 0)
}

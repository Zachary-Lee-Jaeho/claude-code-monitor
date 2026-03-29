package ui

import (
	"testing"
	"time"
)

func TestShortenModel(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"claude-opus-4-6-20250101", "opus-4-6"},
		{"claude-sonnet-4-6-20250101", "sonnet-4-6"},
		{"claude-haiku-4-5-20251001", "haiku-4-5"},
		{"claude-opus-4-6", "opus-4-6"},
		{"", "-"},
	}
	for _, tc := range tests {
		got := ShortenModel(tc.input)
		if got != tc.want {
			t.Errorf("ShortenModel(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		input uint64
		want  string
	}{
		{500, "500"},
		{1234, "1.2k"},
		{12345, "12.3k"},
		{1234567, "1.2M"},
		{0, "0"},
	}
	for _, tc := range tests {
		got := FormatTokens(tc.input)
		if got != tc.want {
			t.Errorf("FormatTokens(%d) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestFormatCost(t *testing.T) {
	tests := []struct {
		input float64
		want  string
	}{
		{0.5, "$0.50"},
		{123.456, "$123.46"},
		{0, "$0.00"},
	}
	for _, tc := range tests {
		got := FormatCost(tc.input)
		if got != tc.want {
			t.Errorf("FormatCost(%.3f) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		input time.Duration
		want  string
	}{
		{3661 * time.Second, "1:01h"},
		{300 * time.Second, "5m"},
		{7200 * time.Second, "2:00h"},
		{0, "0m"},
	}
	for _, tc := range tests {
		got := FormatDuration(tc.input)
		if got != tc.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestBuildProgressBar(t *testing.T) {
	bar := BuildProgressBar(50, 10)
	if len([]rune(bar)) != 10 {
		t.Errorf("expected 10 rune bar, got %d", len([]rune(bar)))
	}
	// 50% of 10 = 5 filled
	filled := 0
	for _, r := range bar {
		if r == '█' {
			filled++
		}
	}
	if filled != 5 {
		t.Errorf("expected 5 filled blocks at 50%%, got %d", filled)
	}
}

func TestBuildProgressBarEdges(t *testing.T) {
	bar0 := BuildProgressBar(0, 10)
	bar100 := BuildProgressBar(100, 10)
	barOver := BuildProgressBar(150, 10)

	filled0 := 0
	for _, r := range bar0 {
		if r == '█' {
			filled0++
		}
	}
	if filled0 != 0 {
		t.Errorf("0%% should have 0 filled, got %d", filled0)
	}

	filled100 := 0
	for _, r := range bar100 {
		if r == '█' {
			filled100++
		}
	}
	if filled100 != 10 {
		t.Errorf("100%% should have 10 filled, got %d", filled100)
	}

	filledOver := 0
	for _, r := range barOver {
		if r == '█' {
			filledOver++
		}
	}
	if filledOver != 10 {
		t.Errorf("150%% (clamped) should have 10 filled, got %d", filledOver)
	}
}

func TestShortenDir(t *testing.T) {
	short := ShortenDir("/a/b", 50)
	if short != "/a/b" {
		t.Errorf("short path should be unchanged, got %q", short)
	}
}

func TestEffortDisplay(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"max", "Max"},
		{"high", "High"},
		{"low", "Low"},
		{"auto", "Auto"},
		{"", "Auto"},
	}
	for _, tc := range tests {
		got := EffortDisplay(tc.input)
		if got != tc.want {
			t.Errorf("EffortDisplay(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

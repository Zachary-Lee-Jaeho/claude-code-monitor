package ui

import (
	"fmt"
	"strings"
	"time"
)

// ShortenModel removes "claude-" prefix and date suffixes.
// "claude-opus-4-6-20250101" → "opus-4-6"
func ShortenModel(name string) string {
	if name == "" {
		return "-"
	}
	s := strings.TrimPrefix(name, "claude-")
	// Remove trailing 8-digit date suffix (e.g., -20250101)
	if len(s) > 9 && s[len(s)-9] == '-' {
		tail := s[len(s)-8:]
		allDigits := true
		for _, c := range tail {
			if c < '0' || c > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			s = s[:len(s)-9]
		}
	}
	return s
}

// FormatTokens formats token count in compact form.
func FormatTokens(n uint64) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}

// FormatCost formats a dollar amount with 2 decimal places.
func FormatCost(cost float64) string {
	return fmt.Sprintf("$%.2f", cost)
}

// FormatDuration formats duration as HH:MMh.
func FormatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	if hours > 0 {
		return fmt.Sprintf("%d:%02dh", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

// EffortDisplay returns display label for effort level.
func EffortDisplay(effort string) string {
	switch effort {
	case "max":
		return "Max"
	case "high":
		return "High"
	case "low":
		return "Low"
	default:
		return "Auto"
	}
}

// ShortenDir formats a path as root/../parent/last.
func ShortenDir(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}
	// Replace home with ~
	if strings.HasPrefix(path, "/home/") {
		parts := strings.SplitN(path, "/", 4)
		if len(parts) >= 4 {
			path = "~/" + parts[3]
		}
	}
	if len(path) <= maxLen {
		return path
	}

	parts := strings.Split(path, "/")
	if len(parts) <= 2 {
		return path[:maxLen-2] + ".."
	}

	// root/../last
	first := parts[0]
	if first == "" && len(parts) > 1 {
		first = "/" + parts[1]
		parts = parts[2:]
	}
	last := parts[len(parts)-1]
	result := first + "/../" + last
	if len(result) > maxLen {
		return result[:maxLen-2] + ".."
	}
	return result
}

// BuildProgressBar creates a Unicode progress bar string.
// width is the bar character count (not including brackets).
func BuildProgressBar(pct float64, width int) string {
	if width < 1 {
		width = 1
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := int(pct / 100.0 * float64(width))
	if filled > width {
		filled = width
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

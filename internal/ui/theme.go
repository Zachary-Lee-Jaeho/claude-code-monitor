package ui

import "github.com/gdamore/tcell/v2"

// GitHub-dark inspired 24-bit color palette
var (
	ClaudeOrange = tcell.NewRGBColor(232, 143, 44)

	TextHighlight = tcell.NewRGBColor(230, 237, 243)
	TextDim       = tcell.NewRGBColor(139, 148, 158)
	TextIdle      = tcell.NewRGBColor(110, 118, 129)

	Blue   = tcell.NewRGBColor(88, 166, 255)
	Green  = tcell.NewRGBColor(63, 185, 80)
	Yellow = tcell.NewRGBColor(210, 153, 34)
	Orange = tcell.NewRGBColor(247, 129, 102)
	Red    = tcell.NewRGBColor(248, 81, 73)

	BgMain      = tcell.NewRGBColor(13, 17, 23)
	BgStatus    = tcell.NewRGBColor(31, 41, 55)
	BgActiveRow = tcell.NewRGBColor(28, 43, 58)
	BgBorder    = tcell.NewRGBColor(33, 38, 45)
)

// BarColor returns the appropriate color for a usage percentage.
func BarColor(pct float64) tcell.Color {
	if pct >= 85 {
		return Red
	}
	if pct >= 50 {
		return Orange
	}
	return Green
}

// CacheColor returns color based on cache hit rate.
func CacheColor(rate float64) tcell.Color {
	if rate >= 60 {
		return Green
	}
	if rate >= 30 {
		return Yellow
	}
	return Red
}

// EffortColor returns color for effort level display.
func EffortColor(effort string) tcell.Color {
	switch effort {
	case "max":
		return Orange
	case "high":
		return Yellow
	case "low":
		return TextIdle
	default:
		return TextHighlight
	}
}

// ModelTierColor returns color for model tier.
func ModelTierColor(model string) tcell.Color {
	switch {
	case len(model) > 0 && (model[0] == 'o' || model[0] == 'O'):
		return Orange
	case len(model) > 0 && (model[0] == 'h' || model[0] == 'H'):
		return Green
	default:
		return Blue
	}
}

package ui

import (
	"fmt"

	"github.com/jaeho/ccmo/internal/data"
	"github.com/rivo/tview"
)

// HeaderView renders the 3-line usage bars.
type HeaderView struct {
	*tview.TextView
}

func NewHeaderView() *HeaderView {
	tv := tview.NewTextView()
	tv.SetDynamicColors(true)
	tv.SetBorder(false)
	tv.SetBackgroundColor(BgMain)
	return &HeaderView{TextView: tv}
}

// Update refreshes the header with current usage data.
func (h *HeaderView) Update(usage data.UsageData, plan data.PlanConfig,
	window5hTokens uint64, window5hMsgs uint64, weeklyCost float64,
	totalBurnRate float64, termWidth int) {

	barWidth := termWidth / 4
	if barWidth < 5 {
		barWidth = 5
	}
	if barWidth > 40 {
		barWidth = 40
	}

	staleMarker := "" // staleness shown in status bar "usage: Xs ago"

	var text string

	// Line 1: Session quota (5-hour window)
	sessionPct := usage.SessionPct
	sessionInfo := ""
	if usage.IsAvailable() {
		sessionInfo = fmt.Sprintf("  %s tok  %d msgs", FormatTokens(window5hTokens), window5hMsgs)
		if usage.SessionReset != "" {
			sessionInfo += "  resets " + usage.SessionReset
		}
	} else {
		sessionInfo = fmt.Sprintf("  ~%s tok  %d msgs", FormatTokens(window5hTokens), window5hMsgs)
	}

	// Burn rate warning: estimate time to quota exhaustion
	if totalBurnRate > 0 && plan.TokenLimit() > 0 {
		remaining := float64(plan.TokenLimit()) - float64(window5hTokens)
		if remaining > 0 {
			minutesToExhaustion := remaining / totalBurnRate
			if minutesToExhaustion <= 60 {
				sessionInfo += burnRateWarning(minutesToExhaustion)
			}
		}
	}

	text += formatBarLine("Session", sessionPct, barWidth, sessionInfo, staleMarker) + "\n"

	// Line 2: Weekly spending
	weeklyPct := usage.WeeklyPct
	weeklyInfo := ""
	if usage.IsAvailable() {
		// Use OAuth pct to back-calculate estimated weekly cost (more accurate than JSONL total)
		estimatedWeekly := weeklyPct / 100.0 * plan.CostLimit()
		weeklyInfo = fmt.Sprintf("  %s / %s", FormatCost(estimatedWeekly), FormatCost(plan.CostLimit()))
		if usage.WeeklyReset != "" {
			weeklyInfo += "  resets " + usage.WeeklyReset
		}
	} else {
		if plan.CostLimit() > 0 {
			weeklyPct = weeklyCost / plan.CostLimit() * 100
		}
		weeklyInfo = fmt.Sprintf("  ~%s / %s", FormatCost(weeklyCost), FormatCost(plan.CostLimit()))
	}
	text += formatBarLine("Weekly ", weeklyPct, barWidth, weeklyInfo, staleMarker) + "\n"

	// Line 3: Extra usage
	extraPct := usage.ExtraPct
	extraInfo := ""
	if usage.IsAvailable() && usage.ExtraSpent != "" {
		extraInfo = "  " + usage.ExtraSpent
		if usage.ExtraReset != "" {
			extraInfo += "  resets " + usage.ExtraReset
		}
	} else {
		extraInfo = "  n/a"
	}
	text += formatBarLine("Extra  ", extraPct, barWidth, extraInfo, staleMarker)

	h.SetText(text)
}

// burnRateWarning returns a colored warning string based on minutes remaining.
func burnRateWarning(minutes float64) string {
	m := int(minutes)
	if m < 1 {
		m = 1
	}
	if minutes < 10 {
		return fmt.Sprintf("  [red::bl]⚠ ~%dm remaining![-::-]", m)
	}
	if minutes < 30 {
		return fmt.Sprintf("  [orange]⚠ ~%dm remaining[-]", m)
	}
	return fmt.Sprintf("  [yellow]~%dm remaining[-]", m)
}

func formatBarLine(label string, pct float64, barWidth int, info, staleMarker string) string {
	bar := BuildProgressBar(pct, barWidth)

	// Color the bar
	colorTag := "[green]"
	if pct >= 85 {
		colorTag = "[red]"
	} else if pct >= 50 {
		colorTag = "[orange]"
	}

	return fmt.Sprintf(" [::b]%s[-::-]  %s%s[-]  %3.0f%%%s%s",
		label, colorTag, bar, pct, staleMarker, info)
}

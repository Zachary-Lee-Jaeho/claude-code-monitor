package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/jaeho/ccmo/internal/data"
	"github.com/rivo/tview"
)

// DetailsView renders the split 50/50 detail panel.
type DetailsView struct {
	*tview.Flex
	left  *tview.TextView
	right *tview.TextView
}

func NewDetailsView() *DetailsView {
	left := tview.NewTextView()
	left.SetDynamicColors(true)
	left.SetBorder(true)
	left.SetBorderColor(BgBorder)
	left.SetTitle(" Thread Details ")
	left.SetTitleColor(ClaudeOrange)
	left.SetBackgroundColor(BgMain)

	right := tview.NewTextView()
	right.SetDynamicColors(true)
	right.SetBorder(true)
	right.SetBorderColor(BgBorder)
	right.SetTitle(" Usage ")
	right.SetTitleColor(ClaudeOrange)
	right.SetBackgroundColor(BgMain)

	flex := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(left, 0, 1, false).
		AddItem(right, 0, 1, false)

	return &DetailsView{Flex: flex, left: left, right: right}
}

// Update refreshes the detail panel for the selected thread.
func (d *DetailsView) Update(t *data.Thread, usage data.UsageData) {
	if t == nil {
		d.left.SetText("")
		d.right.SetText("")
		return
	}

	d.updateLeft(t, usage)
	d.updateRight(t)
}

func (d *DetailsView) updateLeft(t *data.Thread, usage data.UsageData) {
	var b strings.Builder
	now := time.Now()

	// Line 1: PID + project + status badge
	statusColor := "green"
	statusLabel := "ACTIVE"
	if t.Status != data.StatusRunning && t.Status != data.StatusWaiting {
		statusColor = "gray"
		statusLabel = "OFFLINE"
	}
	if t.PID > 0 {
		fmt.Fprintf(&b, " PID [::b]%d[-::-]  %s  [%s]%s[-]\n", t.PID, lastDirName(t.ProjectPath), statusColor, statusLabel)
	} else {
		fmt.Fprintf(&b, " PID -  %s  [%s]%s[-]\n", lastDirName(t.ProjectPath), statusColor, statusLabel)
	}
	b.WriteString("\n")

	// Line 2: Model + Effort
	model := ShortenModel(t.LastModel)
	effort := EffortDisplay(t.LastEffort)
	fmt.Fprintf(&b, " Model: [::b]%-12s[-::-]  Effort: [::b]%s[-::-]\n", model, effort)

	// Line 3: Duration + Burn rate
	duration := now.Sub(t.FirstActivity)
	fmt.Fprintf(&b, " Duration: %-10s  Burn: %.0f tok/m\n", FormatDuration(duration), t.BurnRate)
	b.WriteString("\n")

	// Line 4: Tokens
	fmt.Fprintf(&b, " Tokens:  in [blue]%s[-]  out [blue]%s[-]\n",
		FormatTokens(t.TotalUsage.TotalInputAll()), FormatTokens(t.TotalUsage.TotalOutput()))

	// Line 5: Cost + Cache
	hitRate := t.TotalUsage.HitRate()
	fmt.Fprintf(&b, " Cost:    [orange]%s[-]   hit %.1f%%  (saved %s)\n",
		FormatCost(t.TotalCost), hitRate, FormatCost(t.SavedCost))

	// Line 6: Messages remaining (if OAuth available)
	if usage.IsAvailable() {
		remainPct := 100.0 - usage.SessionPct
		fmt.Fprintf(&b, " Remain:  ~%.0f%% session quota\n", remainPct)
	}

	d.left.SetText(b.String())
}

func (d *DetailsView) updateRight(t *data.Thread) {
	var b strings.Builder

	// Full path
	path := t.ProjectPath
	if len(path) > 45 {
		path = "…" + path[len(path)-44:]
	}
	fmt.Fprintf(&b, " Path: %s\n", path)

	// Timestamps
	fmt.Fprintf(&b, " First: %s  Last: %s\n",
		t.FirstActivity.Local().Format("01-02 15:04"),
		t.LastActivity.Local().Format("01-02 15:04"))
	b.WriteString("\n")

	// Per-model distribution bar
	totalTokens := t.TotalUsage.TotalInputAll() + t.TotalUsage.TotalOutput()
	if totalTokens > 0 && len(t.PerModelUsage) > 0 {
		b.WriteString(" ──── Thread Model ────\n")

		// Calculate per-tier totals
		type tierInfo struct {
			name   string
			tokens uint64
			cost   float64
			pct    float64
		}
		tiers := make(map[data.ModelTier]*tierInfo)
		tiers[data.TierOpus] = &tierInfo{name: "opus"}
		tiers[data.TierSonnet] = &tierInfo{name: "sonnet"}
		tiers[data.TierHaiku] = &tierInfo{name: "haiku"}

		for model, usage := range t.PerModelUsage {
			tier := data.ModelTierFrom(model)
			info := tiers[tier]
			total := usage.TotalInputAll() + usage.TotalOutput()
			info.tokens += total
			cost, _ := data.CalculateCost(model, usage)
			info.cost += cost
		}

		for _, info := range tiers {
			if totalTokens > 0 {
				info.pct = float64(info.tokens) / float64(totalTokens) * 100
			}
		}

		// Stacked bar (20 chars wide)
		barW := 20
		opusCells := int(tiers[data.TierOpus].pct / 100 * float64(barW))
		sonnetCells := int(tiers[data.TierSonnet].pct / 100 * float64(barW))
		haikuCells := barW - opusCells - sonnetCells
		if haikuCells < 0 {
			haikuCells = 0
		}

		fmt.Fprintf(&b, " [orange]%s[-][blue]%s[-][green]%s[-]\n",
			strings.Repeat("█", opusCells),
			strings.Repeat("█", sonnetCells),
			strings.Repeat("█", haikuCells))

		fmt.Fprintf(&b, "  opus %.0f%%  sonnet %.0f%%  haiku %.0f%%\n",
			tiers[data.TierOpus].pct, tiers[data.TierSonnet].pct, tiers[data.TierHaiku].pct)
		b.WriteString("\n")

		// All-time stats
		b.WriteString(" ──── All-Time ────\n")
		fmt.Fprintf(&b, " Total %s  %s\n",
			FormatTokens(totalTokens), FormatCost(t.TotalCost))
		for _, tier := range []data.ModelTier{data.TierOpus, data.TierSonnet, data.TierHaiku} {
			info := tiers[tier]
			if info.tokens > 0 {
				fmt.Fprintf(&b, "  %s %s  %s\n", info.name, FormatTokens(info.tokens), FormatCost(info.cost))
			}
		}
	}

	d.right.SetText(b.String())
}

func lastDirName(path string) string {
	parts := strings.Split(path, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" {
			return parts[i]
		}
	}
	return path
}

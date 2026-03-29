package ui

import (
	"fmt"
	"time"

	"github.com/jaeho/ccmo/internal/data"
	"github.com/rivo/tview"
)

// Layout orchestrates the entire TUI layout.
type Layout struct {
	App       *tview.Application
	Pages     *tview.Pages
	Root      *tview.Flex
	Header    *HeaderView
	Label     *tview.TextView
	Threads   *ThreadsView
	Messages  *MessagesView
	Details   *DetailsView
	StatusBar *tview.TextView

	showRaw bool
}

// NewLayout creates the full TUI layout.
func NewLayout(showRaw bool) *Layout {
	app := tview.NewApplication()

	l := &Layout{
		App:      app,
		Header:   NewHeaderView(),
		Threads:  NewThreadsView(),
		Messages: NewMessagesView(showRaw),
		Details:  NewDetailsView(),
		showRaw:  showRaw,
	}

	// Label row: "Local Threads (N active)    Sort: COLUMN ▼"
	l.Label = tview.NewTextView()
	l.Label.SetDynamicColors(true)
	l.Label.SetBackgroundColor(BgMain)

	// Status bar
	l.StatusBar = tview.NewTextView()
	l.StatusBar.SetDynamicColors(true)
	l.StatusBar.SetBackgroundColor(BgStatus)

	// Build layout
	l.Root = tview.NewFlex().SetDirection(tview.FlexRow)
	l.Root.SetBorder(true)
	l.Root.SetBorderColor(ClaudeOrange)
	l.Root.SetBackgroundColor(BgMain)
	l.Root.SetTitle(" CCMO ")
	l.Root.SetTitleColor(ClaudeOrange)

	l.Pages = tview.NewPages()
	l.Pages.AddPage("main", l.Root, true, true)

	app.SetRoot(l.Pages, true)
	app.EnableMouse(false)

	return l
}

// Rebuild reconstructs the layout based on current terminal dimensions.
// Must be called on terminal resize and on each refresh.
func (l *Layout) Rebuild(threads []data.Thread, selected int, activeCount int,
	plan data.PlanConfig, usage data.UsageData,
	window5hTokens uint64, window5hMsgs uint64, weeklyCost float64,
	totalBurnRate float64,
	filter string, sortCol SortColumn, usageErr string, versionInfo string) {

	_, _, width, height := l.Root.GetInnerRect()
	if width == 0 {
		width = 120
	}
	if height == 0 {
		height = 40
	}

	// Update title with plan + clock
	l.Root.SetTitle(fmt.Sprintf(" CCMO (%s) ── %s ", plan.Label(), time.Now().Format("Mon 15:04")))
	l.Root.SetTitleAlign(tview.AlignLeft)

	// Sync sort column
	l.Threads.SetSortColumn(sortCol)

	// Update header
	l.Header.Update(usage, plan, window5hTokens, window5hMsgs, weeklyCost, totalBurnRate, width)

	// Update label
	filterText := ""
	if filter != "" && filter != "all" {
		filterText = fmt.Sprintf("  [gray]Filter: %s[-]", filter)
	}
	l.Label.SetText(fmt.Sprintf(" [::b]Threads[-::-] (%d active)%s%s",
		activeCount, filterText,
		fmt.Sprintf("    Sort: [orange]%s ▼[-]", sortCol.Label())))

	// Update threads table
	l.Threads.Update(threads, width)

	// Update selected thread details + messages
	var selectedThread *data.Thread
	if selected >= 0 && selected < len(threads) {
		selectedThread = &threads[selected]
	}
	l.Details.Update(selectedThread, usage)
	maxMsgLines := 5
	l.Messages.Update(selectedThread, maxMsgLines)

	// Update status bar
	l.updateStatusBar(usage, usageErr, versionInfo)

	// Rebuild flex layout based on available height
	l.Root.Clear()

	// Fixed heights: header=3, label=1, statusbar=1 = 5 lines
	fixedLines := 5
	available := height - fixedLines

	l.Root.AddItem(l.Header, 3, 0, false)
	l.Root.AddItem(l.Label, 1, 0, false)

	if available < 3 {
		// Minimal: just header + status
		l.Root.AddItem(l.StatusBar, 1, 0, false)
		return
	}

	// Determine what fits
	detailH := 10
	msgH := 0
	if selectedThread != nil && len(selectedThread.RecentCommands) > 0 {
		msgH = min(5, len(selectedThread.RecentCommands))
	}
	threadMinH := 3

	if available >= threadMinH+detailH+msgH {
		// Full layout
		threadH := available - detailH - msgH
		l.Root.AddItem(l.Threads, threadH, 0, true)
		if msgH > 0 {
			l.Root.AddItem(l.Messages, msgH, 0, false)
		}
		l.Root.AddItem(l.Details, detailH, 0, false)
	} else if available >= threadMinH+detailH {
		// No messages
		threadH := available - detailH
		l.Root.AddItem(l.Threads, threadH, 0, true)
		l.Root.AddItem(l.Details, detailH, 0, false)
	} else {
		// Threads only
		l.Root.AddItem(l.Threads, available, 0, true)
	}

	l.Root.AddItem(l.StatusBar, 1, 0, false)

	// Only grab focus if we're on the main page (don't steal from modals)
	front, _ := l.Pages.GetFrontPage()
	if front == "main" {
		l.App.SetFocus(l.Threads)
	}
}

func (l *Layout) updateStatusBar(usage data.UsageData, usageErr string, versionInfo string) {
	age := "never"
	if usage.IsAvailable() {
		d := time.Since(usage.UpdatedAt)
		if d < time.Minute {
			age = fmt.Sprintf("%ds ago", int(d.Seconds()))
		} else {
			age = fmt.Sprintf("%dm ago", int(d.Minutes()))
		}
	}

	live := "[green]● LIVE[-]"
	usageInfo := fmt.Sprintf("[gray]usage: %s[-]", age)
	if usageErr != "" {
		usageInfo = fmt.Sprintf("[red]usage: %s[-]", usageErr)
	}
	updateInfo := ""
	if versionInfo != "" {
		updateInfo = fmt.Sprintf("  [yellow]⬆ %s available[-]", versionInfo)
	}
	l.StatusBar.SetText(fmt.Sprintf(
		" [gray]↑↓[-]:select  [gray]←→[-]:sort  [gray]Tab[-]:filter  "+
			"[gray]d[-]:delete  [gray]a[-]:add server  [gray]?[-]:help  [gray]q[-]:quit"+
			"    %s  %s%s", live, usageInfo, updateInfo))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

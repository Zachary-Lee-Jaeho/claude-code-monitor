package app

import (
	"os"
	"sort"
	"sync"
	"time"

	"github.com/jaeho/ccmo/internal/config"
	"github.com/jaeho/ccmo/internal/data"
	"github.com/jaeho/ccmo/internal/remote"
	"github.com/jaeho/ccmo/internal/ui"
)

// App holds all application state.
type App struct {
	mu sync.Mutex

	Plan        data.PlanConfig
	allThreads  []data.Thread // unfiltered full list
	Threads     []data.Thread // filtered + sorted view for display
	Selected    int
	SelectedID  string // SessionFile of selected thread (stable across re-sorts)
	ActiveCount int
	SortColumn  ui.SortColumn
	Filter      string // "all", "local", or server name
	ShowRawMsgs bool

	// aggregates (cached to avoid recomputing)
	window5hTokens uint64
	window5hMsgs   uint64
	weeklyCost     float64
	totalBurnRate  float64 // aggregate tokens/minute across active threads

	cache     *data.JsonlCache
	pathCache data.PathCache
	fetcher   *data.UsageFetcher
	remoteMgr *remote.Manager

	rebuilding    bool // true during Rebuild — suppress selection callbacks
	lastRefresh   time.Time
	LatestVersion string // set by async update check
	Layout        *ui.Layout
}

// New creates a new App instance.
func New(plan data.PlanConfig, showRaw bool) *App {
	a := &App{
		Plan:        plan,
		Filter:      "all",
		cache:       data.NewJsonlCache(),
		pathCache:   make(data.PathCache),
		fetcher:     data.NewUsageFetcher(),
		SortColumn:  ui.SortCost,
		ShowRawMsgs: showRaw,
	}

	// Restore persisted UI state
	saved := config.LoadUIState()
	if saved.Filter != "" {
		a.Filter = saved.Filter
	}
	if col, ok := parseSortColumn(saved.SortColumn); ok {
		a.SortColumn = col
	}
	a.SelectedID = saved.SelectedSession

	a.Layout = ui.NewLayout(showRaw)

	// Start remote manager
	a.remoteMgr = remote.NewManager()
	a.remoteMgr.Start()

	return a
}

// Stop cleans up all resources.
func (a *App) Stop() {
	// Persist UI state for next launch
	a.mu.Lock()
	state := config.UIState{
		SelectedSession: a.SelectedID,
		Filter:          a.Filter,
		SortColumn:      a.SortColumn.Label(),
	}
	a.mu.Unlock()
	_ = config.SaveUIState(state)

	if a.remoteMgr != nil {
		a.remoteMgr.Stop()
	}
}

// RefreshData reloads all data from disk. Safe to call from any goroutine.
// Updates UI via QueueUpdateDraw.
func (a *App) RefreshData() {
	procs := data.ScanProcesses()
	threads := data.ScanThreads(a.cache, procs, a.pathCache)

	// Merge remote threads
	if a.remoteMgr != nil {
		threads = append(threads, a.remoteMgr.GetThreads()...)
	}

	a.mu.Lock()
	a.allThreads = threads
	a.lastRefresh = time.Now()
	a.recomputeAggregates()
	a.applyFilterAndSort()
	a.mu.Unlock()

	// Update usage fetcher mode
	a.fetcher.SetActiveMode(a.ActiveCount > 0)
	a.fetcher.MaybeRefresh()

	// Update UI (thread-safe)
	a.queueRender()
}

// RefreshDataAsync starts a background data refresh.
func (a *App) RefreshDataAsync() {
	go a.RefreshData()
}

// InitialRender does a synchronous data load + UI build (call BEFORE app.Run).
func (a *App) InitialRender() {
	procs := data.ScanProcesses()
	threads := data.ScanThreads(a.cache, procs, a.pathCache)

	if a.remoteMgr != nil {
		threads = append(threads, a.remoteMgr.GetThreads()...)
	}

	a.mu.Lock()
	a.allThreads = threads
	a.lastRefresh = time.Now()
	a.recomputeAggregates()
	a.applyFilterAndSort()
	a.mu.Unlock()

	a.fetcher.SetActiveMode(a.ActiveCount > 0)
	a.fetcher.MaybeRefresh()

	// Direct render — no QueueUpdateDraw needed before Run()
	a.renderDirect()
}

// Rerender re-sorts/re-filters existing data and renders. No disk I/O.
// Safe to call from the main goroutine (InputCapture).
func (a *App) Rerender() {
	a.mu.Lock()
	a.applyFilterAndSort()
	a.mu.Unlock()
	a.renderDirect()
}

// --- internal helpers ---

func (a *App) recomputeAggregates() {
	// must hold a.mu — compute from allThreads (unfiltered)
	a.ActiveCount = 0
	a.window5hTokens = 0
	a.window5hMsgs = 0
	a.weeklyCost = 0
	a.totalBurnRate = 0
	for _, t := range a.allThreads {
		if t.IsActive {
			a.ActiveCount++
		}
		a.window5hTokens += t.Window5hUsage.TotalInputAll() + t.Window5hUsage.TotalOutput()
		a.window5hMsgs += t.Window5hMsgCount
		a.weeklyCost += t.WeeklyCost
		if t.Status == data.StatusRunning || t.Status == data.StatusWaiting {
			a.totalBurnRate += t.BurnRate
		}
	}
}

func (a *App) applyFilterAndSort() {
	// must hold a.mu
	// Remember selected thread ID before re-sort
	if a.SelectedID == "" && a.Selected >= 0 && a.Selected < len(a.Threads) {
		a.SelectedID = a.Threads[a.Selected].SessionFile
	}

	// Build filtered view from allThreads (never mutate allThreads)
	if a.Filter != "all" && a.Filter != "" {
		a.Threads = make([]data.Thread, 0, len(a.allThreads))
		for _, t := range a.allThreads {
			switch a.Filter {
			case "local":
				if t.Host == "" {
					a.Threads = append(a.Threads, t)
				}
			default:
				if t.Host == a.Filter {
					a.Threads = append(a.Threads, t)
				}
			}
		}
	} else {
		// No filter — copy full list
		a.Threads = make([]data.Thread, len(a.allThreads))
		copy(a.Threads, a.allThreads)
	}

	a.applySorting()

	// Restore selection by session ID
	if a.SelectedID != "" {
		for i, t := range a.Threads {
			if t.SessionFile == a.SelectedID {
				a.Selected = i
				return
			}
		}
	}
	// Fallback: clamp index
	if a.Selected >= len(a.Threads) {
		a.Selected = len(a.Threads) - 1
	}
	if a.Selected < 0 {
		a.Selected = 0
	}
}

func (a *App) queueRender() {
	usage := a.fetcher.GetData()
	usageErr := a.fetcher.GetLastError()
	a.Layout.App.QueueUpdateDraw(func() {
		a.mu.Lock()
		a.rebuilding = true
		selected := a.Selected
		a.Layout.Rebuild(a.Threads, selected, a.ActiveCount,
			a.Plan, usage, a.window5hTokens, a.window5hMsgs, a.weeklyCost, a.totalBurnRate, a.Filter, a.SortColumn, usageErr, a.LatestVersion)
		a.rebuilding = false
		a.mu.Unlock()
		// Restore table selection outside lock (triggers SetSelectionChangedFunc)
		if selected >= 0 {
			a.Layout.Threads.Select(selected+1, 0)
		}
	})
}

func (a *App) renderDirect() {
	usage := a.fetcher.GetData()
	usageErr := a.fetcher.GetLastError()
	a.mu.Lock()
	a.rebuilding = true
	selected := a.Selected
	a.Layout.Rebuild(a.Threads, selected, a.ActiveCount,
		a.Plan, usage, a.window5hTokens, a.window5hMsgs, a.weeklyCost, a.totalBurnRate, a.Filter, a.SortColumn, usageErr, a.LatestVersion)
	a.rebuilding = false
	a.mu.Unlock()
	// Restore table selection outside lock
	if selected >= 0 {
		a.Layout.Threads.Select(selected+1, 0)
	}
}

func (a *App) applySorting() {
	sort.SliceStable(a.Threads, func(i, j int) bool {
		ti, tj := &a.Threads[i], &a.Threads[j]
		switch a.SortColumn {
		case ui.SortPID:
			return ti.PID > tj.PID
		case ui.SortHost:
			return ti.Host < tj.Host
		case ui.SortDir:
			return ti.ProjectPath < tj.ProjectPath
		case ui.SortProject:
			return lastCmd(ti) < lastCmd(tj)
		case ui.SortStatus:
			return ti.Status.Priority() > tj.Status.Priority()
		case ui.SortModel:
			return ti.LastModel < tj.LastModel
		case ui.SortEffort:
			return data.EffortPriority(ti.LastEffort) > data.EffortPriority(tj.LastEffort)
		case ui.SortCtx:
			return ti.LastCtxUsed > tj.LastCtxUsed
		case ui.SortCache:
			return ti.TotalUsage.HitRate() > tj.TotalUsage.HitRate()
		case ui.SortCost:
			return ti.TotalCost > tj.TotalCost
		case ui.SortDuration:
			di := ti.LastActivity.Sub(ti.FirstActivity)
			dj := tj.LastActivity.Sub(tj.FirstActivity)
			return di > dj
		}
		return false
	})
}

func lastCmd(t *data.Thread) string {
	if len(t.RecentCommands) > 0 {
		return t.RecentCommands[len(t.RecentCommands)-1]
	}
	return ""
}

// RefreshInterval returns how often to refresh.
func (a *App) RefreshInterval() time.Duration {
	if a.fetcher.HasPending() {
		return 500 * time.Millisecond
	}
	return 2 * time.Second
}

// SetSelected sets the selection index (called from tview table callback).
func (a *App) SetSelected(idx int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.rebuilding {
		return // suppress callbacks during Rebuild
	}
	if idx >= 0 && idx < len(a.Threads) {
		a.Selected = idx
		a.SelectedID = a.Threads[idx].SessionFile
	}
}

// SortNext advances to next sort column.
func (a *App) SortNext() {
	a.mu.Lock()
	a.SortColumn = a.SortColumn.Next()
	a.mu.Unlock()
}

// SortPrev goes to previous sort column.
func (a *App) SortPrev() {
	a.mu.Lock()
	a.SortColumn = a.SortColumn.Prev()
	a.mu.Unlock()
}

// ToggleFilter cycles through filters: all → local → server1 → server2 → ... → all
func (a *App) ToggleFilter() {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Build filter cycle: all, local, then each server name
	cycle := []string{"all", "local"}
	if a.remoteMgr != nil {
		for name := range a.remoteMgr.ServerStatus() {
			cycle = append(cycle, name)
		}
	}

	// Find current position and advance
	for i, f := range cycle {
		if f == a.Filter {
			a.Filter = cycle[(i+1)%len(cycle)]
			return
		}
	}
	a.Filter = "all"
}

// SetLatestVersion stores the latest available version for display.
func (a *App) SetLatestVersion(v string) {
	a.mu.Lock()
	a.LatestVersion = v
	a.mu.Unlock()
}

// ForceRefreshUsage triggers an immediate OAuth usage fetch.
func (a *App) ForceRefreshUsage() {
	a.fetcher.ForceRefresh()
}

// DeleteSelected removes JSONL files for the selected thread.
func (a *App) DeleteSelected() bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.Selected < 0 || a.Selected >= len(a.Threads) {
		return false
	}
	t := &a.Threads[a.Selected]
	for _, f := range t.JsonlFiles {
		os.Remove(f)
	}
	return true
}

// SelectedThread returns the currently selected thread, or nil.
func (a *App) SelectedThread() *data.Thread {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.Selected >= 0 && a.Selected < len(a.Threads) {
		return &a.Threads[a.Selected]
	}
	return nil
}

// parseSortColumn converts a label string back to a SortColumn.
func parseSortColumn(label string) (ui.SortColumn, bool) {
	for i := ui.SortPID; i <= ui.SortDuration; i++ {
		if i.Label() == label {
			return i, true
		}
	}
	return ui.SortCost, false
}

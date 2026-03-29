package ui

import (
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/jaeho/ccmo/internal/data"
	"github.com/rivo/tview"
)

// SortColumn identifies which column is being sorted.
type SortColumn int

const (
	SortPID SortColumn = iota
	SortHost
	SortDir
	SortProject
	SortStatus
	SortModel
	SortEffort
	SortCtx
	SortCache
	SortCost
	SortDuration
	sortColumnCount
)

func (s SortColumn) Label() string {
	labels := []string{"PID", "HOST", "DIR", "PROJECT", "STATUS", "MODEL", "EFFORT", "CTX", "CACHE", "COST", "DURATION"}
	if int(s) < len(labels) {
		return labels[s]
	}
	return ""
}

func (s SortColumn) Next() SortColumn {
	return SortColumn((int(s) + 1) % int(sortColumnCount))
}

func (s SortColumn) Prev() SortColumn {
	n := int(s) - 1
	if n < 0 {
		n = int(sortColumnCount) - 1
	}
	return SortColumn(n)
}

// ThreadsView renders the sortable thread table.
type ThreadsView struct {
	*tview.Table
	sortColumn SortColumn
}

func NewThreadsView() *ThreadsView {
	table := tview.NewTable()
	table.SetBorder(false)
	table.SetBackgroundColor(BgMain)
	table.SetSelectable(true, false)
	table.SetSelectedStyle(tcell.StyleDefault.Background(BgActiveRow).Foreground(TextHighlight))
	table.SetFixed(1, 0) // fixed header row

	return &ThreadsView{Table: table, sortColumn: SortCost}
}

func (tv *ThreadsView) SortColumn() SortColumn     { return tv.sortColumn }
func (tv *ThreadsView) SetSortColumn(c SortColumn) { tv.sortColumn = c }

// Update refreshes the table with thread data.
func (tv *ThreadsView) Update(threads []data.Thread, termWidth int) {
	tv.Clear()

	// Determine visible columns based on width
	cols := visibleColumns(termWidth)

	// Header row
	for i, col := range cols {
		label := col.label
		style := tcell.StyleDefault.Foreground(TextDim).Bold(false)
		if col.sort == tv.sortColumn {
			label += " ▼"
			style = tcell.StyleDefault.Foreground(ClaudeOrange).Bold(true)
		}
		cell := tview.NewTableCell(label).SetStyle(style).
			SetExpansion(col.expansion).SetAlign(col.align).
			SetSelectable(false)
		tv.SetCell(0, i, cell)
	}

	// Data rows
	now := time.Now()
	for row, t := range threads {
		isDimmed := t.Status == data.StatusIdle || t.Status == data.StatusError
		for i, col := range cols {
			text, fg := col.render(&t, now)
			if isDimmed {
				fg = TextIdle
			}
			cell := tview.NewTableCell(text).
				SetStyle(tcell.StyleDefault.Foreground(fg).Background(BgMain)).
				SetExpansion(col.expansion).SetAlign(col.align)
			tv.SetCell(row+1, i, cell)
		}
	}
}

type columnDef struct {
	label     string
	sort      SortColumn
	minWidth  int
	expansion int
	align     int
	render    func(t *data.Thread, now time.Time) (string, tcell.Color)
}

func visibleColumns(width int) []columnDef {
	all := allColumns()

	// Always show PID and PROJECT
	// Hide right-to-left: DURATION, COST, CACHE, CTX, EFFORT, MODEL, STATUS, HOST, DIR
	hideOrder := []SortColumn{SortDuration, SortCost, SortCache, SortCtx, SortEffort, SortModel, SortStatus, SortHost, SortDir}

	result := make([]columnDef, len(all))
	copy(result, all)

	usedWidth := 0
	for _, c := range result {
		usedWidth += c.minWidth
	}

	for _, hideCol := range hideOrder {
		if usedWidth <= width {
			break
		}
		// Remove this column
		var filtered []columnDef
		for _, c := range result {
			if c.sort == hideCol {
				usedWidth -= c.minWidth
			} else {
				filtered = append(filtered, c)
			}
		}
		result = filtered
	}

	return result
}

func allColumns() []columnDef {
	return []columnDef{
		{label: "PID", sort: SortPID, minWidth: 7, expansion: 0, align: tview.AlignRight,
			render: func(t *data.Thread, _ time.Time) (string, tcell.Color) {
				if t.PID > 0 {
					return fmt.Sprintf("%d", t.PID), TextHighlight
				}
				return "-", TextDim
			}},
		{label: "HOST", sort: SortHost, minWidth: 10, expansion: 0, align: tview.AlignLeft,
			render: func(t *data.Thread, _ time.Time) (string, tcell.Color) {
				return t.Host, TextDim
			}},
		{label: "DIR", sort: SortDir, minWidth: 15, expansion: 1, align: tview.AlignLeft,
			render: func(t *data.Thread, _ time.Time) (string, tcell.Color) {
				return ShortenDir(t.ProjectPath, 30), TextDim
			}},
		{label: "PROJECT", sort: SortProject, minWidth: 20, expansion: 2, align: tview.AlignLeft,
			render: func(t *data.Thread, _ time.Time) (string, tcell.Color) {
				name := ""
				if len(t.RecentCommands) > 0 {
					name = t.RecentCommands[len(t.RecentCommands)-1]
					if len(name) > 40 {
						name = name[:40]
					}
				}
				if name == "" {
					name = t.SessionFile
					if len(name) > 8 {
						name = name[:8]
					}
				}
				return name, Blue
			}},
		{label: "STATUS", sort: SortStatus, minWidth: 10, expansion: 0, align: tview.AlignCenter,
			render: func(t *data.Thread, _ time.Time) (string, tcell.Color) {
				symbol := t.Status.Symbol()
				var fg tcell.Color
				switch t.Status {
				case data.StatusRunning:
					fg = Green
				case data.StatusWaiting:
					fg = Yellow
				case data.StatusError:
					fg = Red
				default:
					fg = TextIdle
				}
				return symbol + " " + t.Status.String(), fg
			}},
		{label: "MODEL", sort: SortModel, minWidth: 12, expansion: 0, align: tview.AlignLeft,
			render: func(t *data.Thread, _ time.Time) (string, tcell.Color) {
				return ShortenModel(t.LastModel), TextHighlight
			}},
		{label: "EFFORT", sort: SortEffort, minWidth: 6, expansion: 0, align: tview.AlignCenter,
			render: func(t *data.Thread, _ time.Time) (string, tcell.Color) {
				return EffortDisplay(t.LastEffort), EffortColor(t.LastEffort)
			}},
		{label: "CTX", sort: SortCtx, minWidth: 14, expansion: 0, align: tview.AlignRight,
			render: func(t *data.Thread, _ time.Time) (string, tcell.Color) {
				used := FormatTokens(t.LastCtxUsed)
				max := FormatTokens(data.ContextMax(t.LastModel))
				return used + "/" + max, Blue
			}},
		{label: "CACHE", sort: SortCache, minWidth: 6, expansion: 0, align: tview.AlignRight,
			render: func(t *data.Thread, _ time.Time) (string, tcell.Color) {
				rate := t.TotalUsage.HitRate()
				return fmt.Sprintf("%.0f%%", rate), CacheColor(rate)
			}},
		{label: "COST", sort: SortCost, minWidth: 9, expansion: 0, align: tview.AlignRight,
			render: func(t *data.Thread, _ time.Time) (string, tcell.Color) {
				return FormatCost(t.TotalCost), ClaudeOrange
			}},
		{label: "DURATION", sort: SortDuration, minWidth: 8, expansion: 0, align: tview.AlignRight,
			render: func(t *data.Thread, now time.Time) (string, tcell.Color) {
				d := now.Sub(t.FirstActivity)
				return FormatDuration(d), TextDim
			}},
	}
}

package ui

import (
	"fmt"
	"strings"

	"github.com/jaeho/ccmo/internal/data"
	"github.com/jaeho/ccmo/internal/security"
	"github.com/rivo/tview"
)

// MessagesView renders recent user commands.
type MessagesView struct {
	*tview.TextView
	showRaw bool
}

func NewMessagesView(showRaw bool) *MessagesView {
	tv := tview.NewTextView()
	tv.SetDynamicColors(true)
	tv.SetBorder(false)
	tv.SetBackgroundColor(BgMain)
	return &MessagesView{TextView: tv, showRaw: showRaw}
}

// Update refreshes the messages panel for the selected thread.
func (m *MessagesView) Update(t *data.Thread, maxLines int) {
	if t == nil || len(t.RecentCommands) == 0 {
		m.SetText("")
		return
	}

	var b strings.Builder
	cmds := t.RecentCommands

	// Show last N messages (reverse chronological)
	start := len(cmds) - maxLines
	if start < 0 {
		start = 0
	}

	for i := len(cmds) - 1; i >= start; i-- {
		msg := cmds[i]
		if !m.showRaw {
			msg = security.RedactSecrets(msg)
		}

		// Truncate to reasonable width
		if len(msg) > 80 {
			msg = msg[:80] + "…"
		}

		if i == len(cmds)-1 {
			// Most recent: highlighted blue
			fmt.Fprintf(&b, " [blue]\"%s\"[-]\n", tview.Escape(msg))
		} else {
			// Older: dim gray
			fmt.Fprintf(&b, " [gray]\"%s\"[-]\n", tview.Escape(msg))
		}
	}

	m.SetText(b.String())
}

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/jaeho/ccmo/internal/app"
	"github.com/jaeho/ccmo/internal/config"
	"github.com/jaeho/ccmo/internal/data"
	"github.com/jaeho/ccmo/internal/remote"
	"github.com/jaeho/ccmo/internal/ui"
	"github.com/rivo/tview"
	"github.com/spf13/cobra"
)

var (
	flagPlan        string
	flagUpdateUsage bool
	flagShowRaw     bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "ccmo",
		Short: "Claude Code Monitor — htop-style TUI for Claude Code sessions",
		RunE:  runMonitor,
	}

	rootCmd.Version = app.Version
	rootCmd.Flags().StringVar(&flagPlan, "plan", "", "Set plan: pro, max5, max20")
	rootCmd.Flags().BoolVar(&flagUpdateUsage, "update-usage", false, "Refresh quota data and exit")
	rootCmd.Flags().BoolVar(&flagShowRaw, "show-raw-messages", false, "Show unredacted user messages")

	// Remote subcommand
	var remotePort int
	var remoteKey string

	remoteCmd := &cobra.Command{
		Use:   "remote",
		Short: "Manage remote servers",
	}

	addCmd := &cobra.Command{
		Use:   "add [name] [user@host]",
		Short: "Add a remote server",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.AddServer(args[0], args[1], remotePort, remoteKey); err != nil {
				return err
			}
			fmt.Printf("Added server %q (%s:%d)\n", args[0], args[1], remotePort)
			return nil
		},
	}
	addCmd.Flags().IntVar(&remotePort, "port", 22, "SSH port")
	addCmd.Flags().StringVar(&remoteKey, "key", "", "SSH identity file path")

	rmCmd := &cobra.Command{
		Use:   "rm [name]",
		Short: "Remove a remote server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.RemoveServer(args[0]); err != nil {
				return err
			}
			fmt.Printf("Removed server %q\n", args[0])
			return nil
		},
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List remote servers",
		RunE: func(cmd *cobra.Command, args []string) error {
			servers, err := config.LoadServers()
			if err != nil {
				return err
			}
			if len(servers) == 0 {
				fmt.Println("No remote servers configured.")
				return nil
			}
			fmt.Printf("%-15s %-25s %-6s %-8s\n", "NAME", "HOST", "PORT", "ENABLED")
			for _, s := range servers {
				enabled := "yes"
				if !s.Enabled {
					enabled = "no"
				}
				fmt.Printf("%-15s %-25s %-6d %-8s\n", s.Name, s.Host, s.Port, enabled)
			}
			return nil
		},
	}

	testCmd := &cobra.Command{
		Use:   "test [name]",
		Short: "Test SSH connection to a server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			srv, err := config.GetServer(args[0])
			if err != nil {
				return err
			}
			fmt.Printf("Testing connection to %s (%s:%d)...\n", srv.Name, srv.Host, srv.Port)
			if err := remote.TestConnection(*srv); err != nil {
				return fmt.Errorf("FAILED: %w", err)
			}
			fmt.Println("OK — SSH connected, claude projects directory found")
			return nil
		},
	}

	remoteCmd.AddCommand(addCmd, rmCmd, listCmd, testCmd)
	rootCmd.AddCommand(remoteCmd)

	// Hooks subcommand
	hooksCmd := &cobra.Command{
		Use:   "hooks",
		Short: "Manage CCMO hooks in Claude Code settings",
	}

	hooksInstallCmd := &cobra.Command{
		Use:   "install",
		Short: "Install CCMO hooks into ~/.claude/settings.json",
		RunE: func(cmd *cobra.Command, args []string) error {
			added, err := config.InstallHooks()
			if err != nil {
				return err
			}
			if added == 0 {
				fmt.Println("All CCMO hooks already installed.")
			} else {
				fmt.Printf("Installed %d hook(s) into ~/.claude/settings.json\n", added)
			}
			return nil
		},
	}

	hooksRemoveCmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove CCMO hooks from ~/.claude/settings.json",
		RunE: func(cmd *cobra.Command, args []string) error {
			removed, err := config.RemoveHooks()
			if err != nil {
				return err
			}
			if removed == 0 {
				fmt.Println("No CCMO hooks found.")
			} else {
				fmt.Printf("Removed %d hook(s) from ~/.claude/settings.json\n", removed)
			}
			return nil
		},
	}

	hooksStatusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show installed CCMO hooks",
		RunE: func(cmd *cobra.Command, args []string) error {
			installed, err := config.HookStatus()
			if err != nil {
				return err
			}
			if len(installed) == 0 {
				fmt.Println("No CCMO hooks installed.")
			} else {
				fmt.Printf("CCMO hooks installed for %d event(s):\n", len(installed))
				for _, e := range installed {
					fmt.Printf("  - %s\n", e)
				}
			}
			return nil
		},
	}

	hooksCmd.AddCommand(hooksInstallCmd, hooksRemoveCmd, hooksStatusCmd)
	rootCmd.AddCommand(hooksCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runMonitor(cmd *cobra.Command, args []string) error {
	// Resolve plan
	var plan data.PlanConfig
	var ok bool

	if flagPlan != "" {
		plan, ok = data.ParsePlanType(flagPlan)
		if !ok {
			return fmt.Errorf("invalid plan: %s (use pro, max5, max20)", flagPlan)
		}
		config.SavePlan(plan)
	} else {
		plan, ok = config.LoadPlan()
		if !ok {
			plan = config.PromptPlanSelection()
		}
	}

	// Update usage mode
	if flagUpdateUsage {
		fmt.Println("Refreshing usage data...")
		fetcher := data.NewUsageFetcher()
		fetcher.ForceRefresh()
		// Wait for result
		time.Sleep(3 * time.Second)
		d := fetcher.GetData()
		if d.IsAvailable() {
			fmt.Printf("Session: %.1f%% (resets %s)\n", d.SessionPct, d.SessionReset)
			fmt.Printf("Weekly:  %.1f%% (resets %s)\n", d.WeeklyPct, d.WeeklyReset)
			if d.ExtraSpent != "" {
				fmt.Printf("Extra:   %.1f%% (%s)\n", d.ExtraPct, d.ExtraSpent)
			}
		} else {
			fmt.Println("Could not fetch usage data.")
		}
		return nil
	}

	// Start TUI
	application := app.New(plan, flagShowRaw)
	defer application.Stop()

	// Start optional hook server (best effort — port may be in use)
	hookServer := app.NewHookServer()
	if err := hookServer.Start(); err == nil {
		defer hookServer.Stop()
	}

	// Async version check
	go func() {
		if latest, available := app.CheckUpdate("jaeho/ccmo"); available {
			application.SetLatestVersion(latest)
		}
	}()

	// Initial data load (synchronous — before app.Run)
	application.InitialRender()

	// Background refresh goroutine (uses QueueUpdateDraw — after app.Run)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(application.RefreshInterval()):
				application.RefreshData()
			}
		}
	}()

	// Table selection change → sync App.Selected
	application.Layout.Threads.SetSelectionChangedFunc(func(row, col int) {
		if row > 0 {
			application.SetSelected(row - 1)
		}
	})

	// ALL key handling in one place. Nothing delegated to Table.
	application.Layout.App.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Modal/dialog/help active → handle differently
		frontPage, _ := application.Layout.Pages.GetFrontPage()
		closeOverlay := func() {
			application.Layout.Pages.SwitchToPage("main")
			application.Layout.Pages.RemovePage("help")
			application.Layout.Pages.RemovePage("dialog")
			application.Layout.App.SetFocus(application.Layout.Threads)
		}

		if frontPage == "help" {
			// ANY key closes help
			closeOverlay()
			return nil
		}
		if frontPage != "main" {
			// Esc always closes any dialog
			if event.Key() == tcell.KeyEsc {
				closeOverlay()
				return nil
			}
			// Other keys → let tview Modal/Form handle (Enter, Tab, etc.)
			return event
		}

		// Table row navigation (move selection programmatically)
		moveTable := func(offset int) {
			row, _ := application.Layout.Threads.GetSelection()
			row += offset
			count := application.Layout.Threads.GetRowCount()
			if row < 1 { // row 0 = header
				row = 1
			}
			if row >= count {
				row = count - 1
			}
			application.Layout.Threads.Select(row, 0)
			// Explicitly sync selection (callback might not fire if same row)
			application.SetSelected(row - 1)
			application.Rerender()
		}

		switch event.Key() {
		case tcell.KeyUp:
			moveTable(-1)
			return nil
		case tcell.KeyDown:
			moveTable(1)
			return nil
		case tcell.KeyLeft:
			application.SortPrev()
			application.Rerender()
			return nil
		case tcell.KeyRight:
			application.SortNext()
			application.Rerender()
			return nil
		case tcell.KeyTab:
			application.ToggleFilter()
			application.Rerender()
			return nil
		case tcell.KeyF5:
			application.RefreshDataAsync()
			return nil
		case tcell.KeyF2:
			application.SortNext()
			application.Rerender()
			return nil
		case tcell.KeyDelete:
			showDeleteDialog(application)
			return nil
		case tcell.KeyEsc:
			return nil
		case tcell.KeyCtrlC:
			application.Layout.App.Stop()
			return nil
		case tcell.KeyRune:
			switch event.Rune() {
			case 'k':
				moveTable(-1)
				return nil
			case 'j':
				moveTable(1)
				return nil
			case 'q':
				application.Layout.App.Stop()
				return nil
			case 'h':
				application.SortPrev()
				application.Rerender()
				return nil
			case 'l':
				application.SortNext()
				application.Rerender()
				return nil
			case 's':
				application.SortNext()
				application.Rerender()
				return nil
			case 'r':
				application.RefreshDataAsync()
				return nil
			case 'u':
				application.ForceRefreshUsage()
				return nil
			case 'd':
				showDeleteDialog(application)
				return nil
			case 'a':
				showAddServerDialog(application)
				return nil
			case '?':
				showHelp(application)
				return nil
			}
		}
		return nil // consume ALL unhandled keys — prevent Table from eating them
	})

	// Run the TUI
	if err := application.Layout.App.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}
	return nil
}

func showDeleteDialog(a *app.App) {
	t := a.SelectedThread()
	if t == nil {
		return
	}

	// Capture file list NOW (before refresh can change selection)
	filesToDelete := make([]string, len(t.JsonlFiles))
	copy(filesToDelete, t.JsonlFiles)
	folderName := t.FolderName
	sessionFile := t.SessionFile

	name := sessionFile
	if len(t.RecentCommands) > 0 {
		name = t.RecentCommands[len(t.RecentCommands)-1]
		if len(name) > 30 {
			name = name[:30] + "…"
		}
	}

	closeDialog := func() {
		a.Layout.Pages.SwitchToPage("main")
		a.Layout.Pages.RemovePage("dialog")
		a.Layout.App.SetFocus(a.Layout.Threads)
	}

	modal := ui.ConfirmDialog(name,
		func() {
			// Delete captured files
			for _, f := range filesToDelete {
				os.Remove(f)
			}
			// Also try removing the session directory (UUID dir)
			if folderName != "" && sessionFile != "" {
				home, _ := os.UserHomeDir()
				sessionDir := filepath.Join(home, ".claude", "projects", folderName, sessionFile)
				os.RemoveAll(sessionDir) // best effort — only removes if empty or is a dir
			}
			closeDialog()
			// Background refresh — will update UI via QueueUpdateDraw
			go a.RefreshData()
		},
		func() {
			closeDialog()
		},
	)
	a.Layout.Pages.AddPage("dialog", modal, true, true)
	a.Layout.App.SetFocus(modal)
}

func showAddServerDialog(a *app.App) {
	form := ui.ServerDialog(
		func(name, host string, port int, keyFile string) {
			config.AddServer(name, host, port, keyFile)
			a.Layout.Pages.SwitchToPage("main")
			a.Layout.Pages.RemovePage("dialog")
			a.Layout.App.SetFocus(a.Layout.Threads)
			a.RefreshDataAsync()
		},
		func() {
			a.Layout.Pages.SwitchToPage("main")
			a.Layout.Pages.RemovePage("dialog")
			a.Layout.App.SetFocus(a.Layout.Threads)
		},
	)

	// Center the form
	flex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(form, 15, 0, true).
			AddItem(nil, 0, 1, false), 50, 0, true).
		AddItem(nil, 0, 1, false)

	a.Layout.Pages.AddPage("dialog", flex, true, true)
	a.Layout.App.SetFocus(form)
}

func showHelp(a *app.App) {
	help := tview.NewTextView()
	help.SetDynamicColors(true)
	help.SetTextAlign(tview.AlignLeft)
	help.SetBackgroundColor(ui.BgMain)
	help.SetBorder(true)
	help.SetBorderColor(ui.ClaudeOrange)
	help.SetTitle(" Help ")
	help.SetTitleColor(ui.ClaudeOrange)
	help.SetText(`
  [orange::b]CCMO — Claude Code Monitor[-::-]

  [::b]Navigation[-::-]
    ↑/k     Previous thread
    ↓/j     Next thread
    ←/h     Sort by previous column
    →/l     Sort by next column

  [::b]Actions[-::-]
    r/F5    Force refresh
    u       Refresh usage quota
    d/Del   Delete selected session
    Tab     Toggle filter (All/Local)
    s/F2    Cycle sort column
    ?       This help

  [::b]Quit[-::-]
    q       Quit
    Ctrl+C  Quit

  [gray]Press any key to close...[-]`)

	// Center the help view
	flex := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(help, 22, 0, true).
			AddItem(nil, 0, 1, false), 60, 0, true).
		AddItem(nil, 0, 1, false)

	a.Layout.Pages.AddPage("help", flex, true, true)
}

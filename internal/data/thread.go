package data

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// PathCache caches decoded project paths.
type PathCache map[string]string

// ScanThreads discovers all Claude Code sessions and builds Thread structs.
func ScanThreads(cache *JsonlCache, processes []ProcessInfo, pathCache PathCache) []Thread {
	claudeDir := claudeProjectsDir()
	if claudeDir == "" {
		return nil
	}

	entries, err := os.ReadDir(claudeDir)
	if err != nil {
		return nil
	}

	var threads []Thread
	now := time.Now()

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		folderName := entry.Name()
		projectDir := filepath.Join(claudeDir, folderName)

		// Decode project path
		projectPath, ok := pathCache[folderName]
		if !ok {
			projectPath = DecodeProjectPath(folderName)
			pathCache[folderName] = projectPath
		}

		// Find top-level JSONL files
		topJsonls := findTopLevelJsonls(projectDir)
		for _, jf := range topJsonls {
			thread := buildThread(cache, []string{jf}, folderName, filepath.Base(jf), projectPath, now)
			if thread != nil {
				threads = append(threads, *thread)
			}
		}

		// Find UUID session directories
		uuidSessions := findUUIDSessions(projectDir)
		for _, us := range uuidSessions {
			thread := buildThread(cache, us.files, folderName, us.name, projectPath, now)
			if thread != nil {
				threads = append(threads, *thread)
			}
		}
	}

	// Assign PIDs and determine status
	assignProcesses(threads, processes)

	// Sort: active first, then by last activity desc
	sort.Slice(threads, func(i, j int) bool {
		if threads[i].IsActive != threads[j].IsActive {
			return threads[i].IsActive
		}
		return threads[i].LastActivity.After(threads[j].LastActivity)
	})

	return threads
}

type uuidSession struct {
	name  string
	files []string
}

func findTopLevelJsonls(dir string) []string {
	matches, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	return matches
}

func findUUIDSessions(dir string) []uuidSession {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var sessions []uuidSession
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		// UUID dirs typically have dashes and are 36+ chars
		if len(name) < 8 {
			continue
		}
		subdir := filepath.Join(dir, name)

		var files []string
		// Direct JSONL files in UUID dir
		if matches, err := filepath.Glob(filepath.Join(subdir, "*.jsonl")); err == nil {
			files = append(files, matches...)
		}
		// Subagent JSONL files
		subagentsDir := filepath.Join(subdir, "subagents")
		if matches, err := filepath.Glob(filepath.Join(subagentsDir, "*", "*.jsonl")); err == nil {
			files = append(files, matches...)
		}

		if len(files) > 0 {
			sessions = append(sessions, uuidSession{name: name, files: files})
		}
	}
	return sessions
}

func buildThread(cache *JsonlCache, files []string, folderName, sessionFile, projectPath string, now time.Time) *Thread {
	if len(files) == 0 {
		return nil
	}

	t := &Thread{
		FolderName:    folderName,
		SessionFile:   sessionFile,
		ProjectPath:   projectPath,
		JsonlFiles:    files,
		PerModelUsage: make(map[string]*TokenUsage),
		LastEffort:    "auto",
	}

	window5h := now.Add(-5 * time.Hour)
	window7d := now.Add(-7 * 24 * time.Hour)
	var lastEntry *AssistantEntry
	var lastEffortCmd *EffortCommand

	for _, f := range files {
		parsed, err := cache.ParseFile(f)
		if err != nil {
			continue
		}

		for i := range parsed.Entries {
			e := &parsed.Entries[i]

			// Aggregate total usage per model
			if _, ok := t.PerModelUsage[e.Model]; !ok {
				t.PerModelUsage[e.Model] = &TokenUsage{}
			}
			t.PerModelUsage[e.Model].Add(&e.Usage)
			t.TotalUsage.Add(&e.Usage)

			cost, saved := CalculateCost(e.Model, &e.Usage)
			t.TotalCost += cost
			t.SavedCost += saved

			// Time windows
			if !e.Timestamp.IsZero() {
				if t.FirstActivity.IsZero() || e.Timestamp.Before(t.FirstActivity) {
					t.FirstActivity = e.Timestamp
				}
				if e.Timestamp.After(t.LastActivity) {
					t.LastActivity = e.Timestamp
				}

				// 5-hour window
				if e.Timestamp.After(window5h) {
					t.Window5hUsage.Add(&e.Usage)
					t.Window5hMsgCount++
					if t.Window5hStart == nil || e.Timestamp.Before(*t.Window5hStart) {
						ts := e.Timestamp
						t.Window5hStart = &ts
					}
				}

				// 7-day window
				if e.Timestamp.After(window7d) {
					wcost, _ := CalculateCost(e.Model, &e.Usage)
					t.WeeklyCost += wcost
				}
			}

			if lastEntry == nil || e.Timestamp.After(lastEntry.Timestamp) {
				lastEntry = e
			}
		}

		// Recent user messages (last 10 across all files)
		for _, msg := range parsed.UserMessages {
			t.RecentCommands = append(t.RecentCommands, msg)
		}

		// Model/effort commands
		if parsed.LastModelCmd != nil {
			if t.LastModel == "" || parsed.LastModelCmd.Timestamp.After(time.Time{}) {
				t.LastModel = parsed.LastModelCmd.ModelID
			}
		}
		if parsed.LastEffortCmd != nil {
			if lastEffortCmd == nil || parsed.LastEffortCmd.Timestamp.After(lastEffortCmd.Timestamp) {
				lastEffortCmd = parsed.LastEffortCmd
			}
		}
	}

	// Use file mtime as fallback for timestamps
	if t.LastActivity.IsZero() {
		for _, f := range files {
			if info, err := os.Stat(f); err == nil {
				if info.ModTime().After(t.LastActivity) {
					t.LastActivity = info.ModTime()
				}
			}
		}
	}
	if t.FirstActivity.IsZero() {
		t.FirstActivity = t.LastActivity
	}

	// Determine last model from entries if not set by /model command
	if t.LastModel == "" && lastEntry != nil {
		t.LastModel = lastEntry.Model
	}

	// Context used from last entry
	if lastEntry != nil {
		t.LastCtxUsed = lastEntry.Usage.TotalInputAll()
	}

	// Infer effort
	hasThinking := lastEntry != nil && lastEntry.HasThinking
	t.LastEffort = InferEffort(t.LastModel, hasThinking, lastEffortCmd)

	// Burn rate: output tokens per minute
	duration := t.LastActivity.Sub(t.FirstActivity)
	if duration > time.Minute {
		t.BurnRate = float64(t.TotalUsage.OutputTokens) / duration.Minutes()
	}

	// Active if updated within 30 minutes
	t.IsActive = now.Sub(t.LastActivity) < 30*time.Minute

	// Keep only last 10 recent commands
	if len(t.RecentCommands) > 10 {
		t.RecentCommands = t.RecentCommands[len(t.RecentCommands)-10:]
	}

	return t
}

func assignProcesses(threads []Thread, processes []ProcessInfo) {
	now := time.Now()
	staleThreshold := 5 * time.Minute

	// Build lookup structures from process list
	// sessionID → ProcessInfo (for --resume matches)
	bySession := make(map[string]*ProcessInfo)
	// CWD → highest PID ProcessInfo (fallback for processes without --resume)
	byCWD := make(map[string]*ProcessInfo)

	for i := range processes {
		p := &processes[i]
		if p.SessionID != "" {
			bySession[p.SessionID] = p
		} else {
			if existing, ok := byCWD[p.CWD]; !ok || p.PID > existing.PID {
				byCWD[p.CWD] = p
			}
		}
	}

	// Track which CWD fallback PIDs have been claimed
	cwdClaimed := make(map[string]bool)

	for i := range threads {
		t := &threads[i]
		if t.Host != "" {
			continue
		}

		// Try exact session match first (--resume UUID matches SessionFile or folder name)
		if p, ok := bySession[t.SessionFile]; ok {
			t.PID = p.PID
			if now.Sub(t.LastActivity) < staleThreshold || t.LastActivity.IsZero() {
				t.Status = StatusRunning
			} else {
				t.Status = StatusWaiting
			}
			continue
		}

		// Also check if SessionFile is a .jsonl filename — strip extension for matching
		sessionBase := strings.TrimSuffix(t.SessionFile, ".jsonl")
		if p, ok := bySession[sessionBase]; ok {
			t.PID = p.PID
			if now.Sub(t.LastActivity) < staleThreshold || t.LastActivity.IsZero() {
				t.Status = StatusRunning
			} else {
				t.Status = StatusWaiting
			}
			continue
		}

		// Fallback: process without --resume in same CWD (new session, no resume flag)
		// Only assign to most recently active unmatched thread
		if p, ok := byCWD[t.ProjectPath]; ok && !cwdClaimed[t.ProjectPath] {
			cwdClaimed[t.ProjectPath] = true
			t.PID = p.PID
			if now.Sub(t.LastActivity) < staleThreshold || t.LastActivity.IsZero() {
				t.Status = StatusRunning
			} else {
				t.Status = StatusWaiting
			}
			continue
		}

		t.Status = StatusIdle
	}
}

// DecodeProjectPath reverses Claude's folder encoding.
// Claude encodes: every non-alphanumeric char → '-'
// Greedy forward matching: try longest possible path segments, verify on disk.
func DecodeProjectPath(encoded string) string {
	if encoded == "" {
		return ""
	}

	// Remove leading dash (represents root /)
	s := encoded
	if s[0] == '-' {
		s = s[1:]
	}

	parts := strings.Split(s, "-")
	if len(parts) == 0 {
		return "/" + encoded
	}

	// Greedy forward matching: try longest path that exists on disk
	var result []string
	i := 0
	for i < len(parts) {
		bestEnd := i + 1
		bestPath := ""

		// Try joining consecutive segments with '-' (original char could be -, _, ., space, etc.)
		// and check if the resulting path exists
		for j := i + 1; j <= len(parts); j++ {
			// The segment joined with '-' (which was the original separator in encoding)
			segment := strings.Join(parts[i:j], "-")
			candidate := "/" + strings.Join(append(result, segment), "/")
			if _, err := os.Stat(candidate); err == nil {
				bestEnd = j
				bestPath = candidate
			}

			// Also try with underscore, dot, space as the separator between parts
			if j > i+1 {
				for _, sep := range []string{"_", ".", " "} {
					segment2 := strings.Join(parts[i:j], sep)
					candidate2 := "/" + strings.Join(append(result, segment2), "/")
					if _, err := os.Stat(candidate2); err == nil {
						bestEnd = j
						bestPath = candidate2
					}
				}
			}
		}

		if bestPath != "" {
			// Extract the last segment from the best path
			pathParts := strings.Split(bestPath, "/")
			segment := pathParts[len(pathParts)-1]
			result = append(result, segment)
		} else {
			// No path exists — just use the dash-joined segment
			segment := strings.Join(parts[i:bestEnd], "-")
			if segment != "" {
				result = append(result, segment)
			}
		}
		i = bestEnd
	}

	if len(result) == 0 {
		return "/" + encoded
	}
	return "/" + strings.Join(result, "/")
}

// DecodeProjectPathHeuristic is used for remote paths (can't verify on disk).
func DecodeProjectPathHeuristic(encoded string) string {
	if encoded == "" {
		return ""
	}
	s := encoded
	if s[0] == '-' {
		s = s[1:]
	}
	// Simple heuristic: replace leading segments that look like standard dirs
	return "/" + strings.ReplaceAll(s, "-", "/")
}

func claudeProjectsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "projects")
}

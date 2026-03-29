package data

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v3/process"
)

// ProcessInfo holds info about a running Claude process.
type ProcessInfo struct {
	PID       uint32
	CWD       string
	SessionID string // from --resume flag; empty if new session
}

// ScanProcesses finds running Claude Code instances.
// Returns a slice of ProcessInfo with PID, CWD, and session ID.
func ScanProcesses() []ProcessInfo {
	myPID := int32(os.Getpid())

	procs, err := process.Processes()
	if err != nil {
		return scanProcFS()
	}

	var result []ProcessInfo
	for _, p := range procs {
		if p.Pid == myPID {
			continue
		}

		name, err := p.Name()
		if err != nil {
			continue
		}

		if !isClaudeProcess(name) {
			continue
		}

		cwd, err := p.Cwd()
		if err != nil {
			continue
		}

		sessionID := ""
		cmdline, err := p.CmdlineSlice()
		if err == nil {
			sessionID = extractResumeID(cmdline)
		}

		result = append(result, ProcessInfo{
			PID:       uint32(p.Pid),
			CWD:       cwd,
			SessionID: sessionID,
		})
	}

	return result
}

// scanProcFS reads /proc directly as fallback on Linux.
func scanProcFS() []ProcessInfo {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}

	myPID := os.Getpid()
	var result []ProcessInfo

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid == myPID {
			continue
		}

		comm, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "comm"))
		if err != nil {
			continue
		}
		name := strings.TrimSpace(string(comm))
		if !isClaudeProcess(name) {
			continue
		}

		cwd, err := os.Readlink(filepath.Join("/proc", entry.Name(), "cwd"))
		if err != nil {
			continue
		}

		sessionID := ""
		cmdlineRaw, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err == nil {
			args := strings.Split(string(cmdlineRaw), "\x00")
			sessionID = extractResumeID(args)
		}

		result = append(result, ProcessInfo{
			PID:       uint32(pid),
			CWD:       cwd,
			SessionID: sessionID,
		})
	}

	return result
}

func isClaudeProcess(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "claude") || strings.Contains(lower, "ccd-cli")
}

// extractResumeID finds --resume <uuid> in command line args.
func extractResumeID(args []string) string {
	for i, arg := range args {
		if arg == "--resume" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

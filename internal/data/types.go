package data

import (
	"strings"
	"time"
)

// PlanType represents a Claude subscription plan.
type PlanType int

const (
	PlanPro PlanType = iota
	PlanMax5
	PlanMax20
	PlanCustom
)

type PlanConfig struct {
	Type       PlanType
	CustomLimit uint64 // only used when Type == PlanCustom
}

func (p PlanConfig) TokenLimit() uint64 {
	switch p.Type {
	case PlanPro:
		return 19_000
	case PlanMax5:
		return 88_000
	case PlanMax20:
		return 220_000
	case PlanCustom:
		return p.CustomLimit
	}
	return 88_000
}

func (p PlanConfig) CostLimit() float64 {
	switch p.Type {
	case PlanPro:
		return 18.0
	case PlanMax5:
		return 35.0
	case PlanMax20:
		return 140.0
	case PlanCustom:
		return float64(p.CustomLimit) / 1000.0
	}
	return 35.0
}

func (p PlanConfig) Label() string {
	switch p.Type {
	case PlanPro:
		return "Pro"
	case PlanMax5:
		return "Max5"
	case PlanMax20:
		return "Max20"
	case PlanCustom:
		return "Custom"
	}
	return "Max5"
}

func ParsePlanType(s string) (PlanConfig, bool) {
	switch strings.ToLower(s) {
	case "pro":
		return PlanConfig{Type: PlanPro}, true
	case "max5":
		return PlanConfig{Type: PlanMax5}, true
	case "max20":
		return PlanConfig{Type: PlanMax20}, true
	}
	return PlanConfig{}, false
}

// ModelTier determines pricing tier from model name.
type ModelTier int

const (
	TierSonnet ModelTier = iota
	TierOpus
	TierHaiku
)

func ModelTierFrom(name string) ModelTier {
	lower := strings.ToLower(name)
	if strings.Contains(lower, "opus") {
		return TierOpus
	}
	if strings.Contains(lower, "haiku") {
		return TierHaiku
	}
	return TierSonnet
}

// ThreadStatus represents the current state of a session thread.
type ThreadStatus int

const (
	StatusIdle ThreadStatus = iota
	StatusWaiting
	StatusRunning
	StatusError
)

func (s ThreadStatus) String() string {
	switch s {
	case StatusRunning:
		return "running"
	case StatusWaiting:
		return "waiting"
	case StatusIdle:
		return "idle"
	case StatusError:
		return "error"
	}
	return "idle"
}

func (s ThreadStatus) Symbol() string {
	switch s {
	case StatusRunning:
		return "●"
	case StatusWaiting:
		return "⏸"
	case StatusIdle:
		return "○"
	case StatusError:
		return "✕"
	}
	return "○"
}

// StatusPriority returns sort weight (higher = more important).
func (s ThreadStatus) Priority() int {
	switch s {
	case StatusRunning:
		return 3
	case StatusWaiting:
		return 2
	case StatusIdle:
		return 1
	case StatusError:
		return 0
	}
	return 0
}

// TokenUsage tracks token consumption across 4 types.
type TokenUsage struct {
	InputTokens              uint64
	OutputTokens             uint64
	CacheCreationInputTokens uint64
	CacheReadInputTokens     uint64
}

func (u *TokenUsage) TotalInputAll() uint64 {
	return u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
}

func (u *TokenUsage) TotalOutput() uint64 {
	return u.OutputTokens
}

func (u *TokenUsage) HitRate() float64 {
	total := u.TotalInputAll()
	if total == 0 {
		return 0
	}
	return float64(u.CacheReadInputTokens) / float64(total) * 100.0
}

func (u *TokenUsage) Add(other *TokenUsage) {
	u.InputTokens += other.InputTokens
	u.OutputTokens += other.OutputTokens
	u.CacheCreationInputTokens += other.CacheCreationInputTokens
	u.CacheReadInputTokens += other.CacheReadInputTokens
}

// AssistantEntry represents a single assistant response from JSONL.
type AssistantEntry struct {
	MessageID   string
	Model       string
	Usage       TokenUsage
	HasThinking bool
	Timestamp   time.Time
}

// Thread represents one Claude Code session (top-level JSONL or UUID subdir).
type Thread struct {
	PID           uint32
	Host          string // blank for local, server name for remote
	Status        ThreadStatus
	ProjectPath   string
	FolderName    string // encoded folder name in ~/.claude/projects/
	SessionFile   string // top-level JSONL name or UUID
	TotalUsage    TokenUsage
	TotalCost     float64
	SavedCost     float64
	LastModel     string
	FirstActivity time.Time
	LastActivity  time.Time
	IsActive      bool
	BurnRate      float64 // output_tokens / duration_minutes
	JsonlFiles    []string
	Window5hUsage    TokenUsage
	Window5hStart    *time.Time
	Window5hMsgCount uint64
	WeeklyCost       float64
	PerModelUsage    map[string]*TokenUsage
	RecentCommands   []string
	LastCtxUsed      uint64
	LastEffort       string // "max", "high", "auto", "low"
}

// ContextMax returns the maximum context window for a model.
func ContextMax(model string) uint64 {
	if strings.Contains(strings.ToLower(model), "opus") {
		return 1_000_000
	}
	return 200_000
}

// EffortPriority returns sort weight for effort level.
func EffortPriority(effort string) int {
	switch strings.ToLower(effort) {
	case "max":
		return 3
	case "high":
		return 2
	case "auto":
		return 1
	case "low":
		return 0
	}
	return 1
}

// UsageData holds server-side quota info from OAuth API.
type UsageData struct {
	SessionPct   float64
	SessionReset string
	WeeklyPct    float64
	WeeklyReset  string
	ExtraPct     float64
	ExtraSpent   string
	ExtraReset   string
	UpdatedAt    time.Time
}

func (u *UsageData) IsAvailable() bool {
	return !u.UpdatedAt.IsZero()
}

func (u *UsageData) IsStale() bool {
	return !u.IsAvailable() || time.Since(u.UpdatedAt) >= 5*time.Minute
}

// CachedFile holds the parsed result of a JSONL file.
type CachedFile struct {
	Entries       []AssistantEntry
	UserMessages  []string
	LastModelCmd  *ModelCommand
	LastEffortCmd *EffortCommand
}

type ModelCommand struct {
	Timestamp time.Time
	ModelID   string
}

type EffortCommand struct {
	Timestamp time.Time
	Level     string
}

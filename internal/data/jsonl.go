package data

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
)

// JsonlCache provides mtime-based caching for parsed JSONL files.
type JsonlCache struct {
	entries map[string]*jsonlCacheEntry
}

type jsonlCacheEntry struct {
	mtime  time.Time
	parsed *CachedFile
}

func NewJsonlCache() *JsonlCache {
	return &JsonlCache{entries: make(map[string]*jsonlCacheEntry)}
}

// ParseFile parses a JSONL file with mtime caching.
func (c *JsonlCache) ParseFile(path string) (*CachedFile, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	mtime := info.ModTime()

	if entry, ok := c.entries[path]; ok && entry.mtime.Equal(mtime) {
		return entry.parsed, nil
	}

	parsed, err := doParseJSONL(path)
	if err != nil {
		return nil, err
	}

	c.entries[path] = &jsonlCacheEntry{mtime: mtime, parsed: parsed}
	return parsed, nil
}

// raw JSON structures for JSONL parsing
type rawMessage struct {
	Type    string          `json:"type"`
	Role    string          `json:"role"`
	Message json.RawMessage `json:"message"`
	// top-level fields for assistant messages
	UUID    string          `json:"uuid"`
	Model   string          `json:"model"`
	Usage   *rawUsage       `json:"usage"`
	Content json.RawMessage `json:"content"`
	// timestamp (top-level in JSONL)
	Timestamp string `json:"timestamp"`
	// for user messages
	IsMeta bool   `json:"isMeta"`
	Text   string `json:"text"`
	// for tool results
	Stdout string `json:"stdout"`
}

type rawUsage struct {
	InputTokens              uint64 `json:"input_tokens"`
	OutputTokens             uint64 `json:"output_tokens"`
	CacheCreationInputTokens uint64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     uint64 `json:"cache_read_input_tokens"`
}

type rawAssistantMessage struct {
	ID      string          `json:"id"`
	Model   string          `json:"model"`
	Usage   *rawUsage       `json:"usage"`
	Content json.RawMessage `json:"content"`
}

type rawContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

var (
	ansiRegex       = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	effortArgsRegex = regexp.MustCompile(`<command-args>(.*?)</command-args>`)
	setModelRegex   = regexp.MustCompile(`Set model to\s+(.+)`)
)

func doParseJSONL(path string) (*CachedFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseJSONLReader(f)
}

// ParseJSONLContent parses JSONL content from a string (used for remote SSH data).
func ParseJSONLContent(content string) (*CachedFile, error) {
	return parseJSONLReader(strings.NewReader(content))
}

func parseJSONLReader(r io.Reader) (*CachedFile, error) {
	result := &CachedFile{
		Entries:      make([]AssistantEntry, 0),
		UserMessages: make([]string, 0),
	}
	seen := make(map[string]*AssistantEntry) // message_id → entry (dedup)

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 10MB max line

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var msg rawMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue // skip malformed lines
		}

		switch {
		case isAssistantMessage(&msg):
			if entry := parseAssistantEntry(&msg); entry != nil {
				if existing, ok := seen[entry.MessageID]; ok {
					// keep highest token count
					if entry.Usage.TotalInputAll()+entry.Usage.TotalOutput() >
						existing.Usage.TotalInputAll()+existing.Usage.TotalOutput() {
						*existing = *entry
					}
				} else {
					result.Entries = append(result.Entries, *entry)
					seen[entry.MessageID] = &result.Entries[len(result.Entries)-1]
				}
			}

		case isUserMessage(&msg):
			if text := extractUserMessage(&msg); text != "" {
				result.UserMessages = append(result.UserMessages, text)
			}

		case isToolResult(&msg):
			if cmd := parseModelCommand(&msg); cmd != nil {
				if result.LastModelCmd == nil || cmd.Timestamp.After(result.LastModelCmd.Timestamp) {
					result.LastModelCmd = cmd
				}
			}
			if cmd := parseEffortCommand(&msg); cmd != nil {
				if result.LastEffortCmd == nil || cmd.Timestamp.After(result.LastEffortCmd.Timestamp) {
					result.LastEffortCmd = cmd
				}
			}
		}
	}

	return result, nil
}

func isAssistantMessage(msg *rawMessage) bool {
	return msg.Type == "assistant" || msg.Role == "assistant"
}

func isUserMessage(msg *rawMessage) bool {
	return (msg.Type == "human" || msg.Type == "user" || msg.Role == "user") && !msg.IsMeta
}

func isToolResult(msg *rawMessage) bool {
	return msg.Type == "tool_result" || msg.Type == "local-command-stdout"
}

func parseAssistantEntry(msg *rawMessage) *AssistantEntry {
	// Parse timestamp from top-level field
	ts := parseTimestamp(msg.Timestamp)

	// Try nested message object first (primary path for real JSONL)
	var nested rawAssistantMessage
	if msg.Message != nil {
		if err := json.Unmarshal(msg.Message, &nested); err == nil && nested.ID != "" {
			model := nested.Model
			// Skip synthetic models
			if model == "" || strings.HasPrefix(model, "synthetic") || strings.HasPrefix(model, "<synthetic") {
				return nil
			}
			entry := &AssistantEntry{
				MessageID: nested.ID,
				Model:     model,
				Timestamp: ts,
			}
			if nested.Usage != nil {
				entry.Usage = TokenUsage{
					InputTokens:              nested.Usage.InputTokens,
					OutputTokens:             nested.Usage.OutputTokens,
					CacheCreationInputTokens: nested.Usage.CacheCreationInputTokens,
					CacheReadInputTokens:     nested.Usage.CacheReadInputTokens,
				}
			}
			entry.HasThinking = hasThinkingContent(nested.Content)
			return entry
		}
	}

	// Try top-level fields (fallback)
	if msg.UUID == "" && msg.Model == "" {
		return nil
	}
	id := msg.UUID
	if id == "" {
		return nil
	}

	// Skip synthetic models
	model := msg.Model
	if model == "" || strings.HasPrefix(model, "synthetic") || strings.HasPrefix(model, "<synthetic") {
		return nil
	}

	entry := &AssistantEntry{
		MessageID: id,
		Model:     model,
		Timestamp: ts,
	}
	if msg.Usage != nil {
		entry.Usage = TokenUsage{
			InputTokens:              msg.Usage.InputTokens,
			OutputTokens:             msg.Usage.OutputTokens,
			CacheCreationInputTokens: msg.Usage.CacheCreationInputTokens,
			CacheReadInputTokens:     msg.Usage.CacheReadInputTokens,
		}
	}
	entry.HasThinking = hasThinkingContent(msg.Content)
	return entry
}

func parseTimestamp(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	// Try RFC3339 (ISO8601)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	// Try RFC3339 with milliseconds
	if t, err := time.Parse("2006-01-02T15:04:05.000Z", s); err == nil {
		return t
	}
	// Try flexible parse
	if t, err := time.Parse("2006-01-02T15:04:05.999Z", s); err == nil {
		return t
	}
	return time.Time{}
}

func hasThinkingContent(content json.RawMessage) bool {
	if content == nil {
		return false
	}
	var blocks []rawContentBlock
	if err := json.Unmarshal(content, &blocks); err != nil {
		return false
	}
	for _, b := range blocks {
		if b.Type == "thinking" {
			return true
		}
	}
	return false
}

func extractUserMessage(msg *rawMessage) string {
	// Try direct text field
	text := msg.Text
	if text == "" {
		// Try content as string
		if msg.Content != nil {
			var s string
			if err := json.Unmarshal(msg.Content, &s); err == nil {
				text = s
			} else {
				// Try content as array of blocks
				var blocks []rawContentBlock
				if err := json.Unmarshal(msg.Content, &blocks); err == nil {
					for _, b := range blocks {
						if b.Type == "text" && b.Text != "" {
							text = b.Text
							break
						}
					}
				}
			}
		}
	}

	if text == "" {
		return ""
	}

	// Take first line, truncate to 200 chars
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		text = text[:idx]
	}
	if len(text) > 200 {
		text = text[:200]
	}
	return strings.TrimSpace(text)
}

func parseModelCommand(msg *rawMessage) *ModelCommand {
	stdout := msg.Stdout
	if stdout == "" {
		return nil
	}
	cleaned := stripAnsi(stdout)
	matches := setModelRegex.FindStringSubmatch(cleaned)
	if matches == nil {
		return nil
	}
	displayName := strings.TrimSpace(matches[1])
	modelID := displayNameToModelID(displayName)
	return &ModelCommand{
		Timestamp: time.Now(),
		ModelID:   modelID,
	}
}

func parseEffortCommand(msg *rawMessage) *EffortCommand {
	stdout := msg.Stdout
	if stdout == "" {
		return nil
	}
	matches := effortArgsRegex.FindStringSubmatch(stdout)
	if matches == nil {
		return nil
	}
	level := strings.ToLower(strings.TrimSpace(matches[1]))
	switch level {
	case "max", "high", "auto", "low":
		return &EffortCommand{Timestamp: time.Now(), Level: level}
	}
	return nil
}

func stripAnsi(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

func displayNameToModelID(display string) string {
	lower := strings.ToLower(display)
	switch {
	case strings.Contains(lower, "opus"):
		if strings.Contains(lower, "4.6") {
			return "claude-opus-4-6"
		}
		return "claude-opus-4"
	case strings.Contains(lower, "sonnet"):
		if strings.Contains(lower, "4.6") {
			return "claude-sonnet-4-6"
		}
		return "claude-sonnet-4"
	case strings.Contains(lower, "haiku"):
		if strings.Contains(lower, "4.5") {
			return "claude-haiku-4-5"
		}
		return "claude-haiku-4"
	}
	return lower
}

// InferEffort determines effort level from model, thinking flag, and explicit command.
func InferEffort(model string, hasThinking bool, effortCmd *EffortCommand) string {
	// Explicit command always wins
	if effortCmd != nil {
		return effortCmd.Level
	}
	if hasThinking {
		if strings.Contains(strings.ToLower(model), "opus") {
			return "max"
		}
		return "high"
	}
	if strings.Contains(strings.ToLower(model), "haiku") {
		return "low"
	}
	return "auto"
}

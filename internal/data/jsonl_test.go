package data

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTempJSONL(t *testing.T, lines ...string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	var content string
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseAssistantEntry(t *testing.T) {
	path := writeTempJSONL(t,
		`{"type":"assistant","message":{"id":"msg_01","model":"claude-opus-4-6","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":10,"cache_read_input_tokens":5},"content":[{"type":"text","text":"hello"}]},"timestamp":"2026-01-01T00:00:00Z"}`,
	)
	cache := NewJsonlCache()
	parsed, err := cache.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(parsed.Entries))
	}
	e := parsed.Entries[0]
	if e.MessageID != "msg_01" {
		t.Errorf("expected msg_01, got %s", e.MessageID)
	}
	if e.Model != "claude-opus-4-6" {
		t.Errorf("expected claude-opus-4-6, got %s", e.Model)
	}
	if e.Usage.InputTokens != 100 {
		t.Errorf("expected 100 input tokens, got %d", e.Usage.InputTokens)
	}
	if e.Usage.OutputTokens != 50 {
		t.Errorf("expected 50 output tokens, got %d", e.Usage.OutputTokens)
	}
	if e.Usage.CacheCreationInputTokens != 10 {
		t.Errorf("expected 10 cache creation, got %d", e.Usage.CacheCreationInputTokens)
	}
	if e.Usage.CacheReadInputTokens != 5 {
		t.Errorf("expected 5 cache read, got %d", e.Usage.CacheReadInputTokens)
	}
}

func TestParseUserMessage(t *testing.T) {
	path := writeTempJSONL(t,
		`{"type":"human","text":"build the app"}`,
		`{"type":"tool_result","stdout":"ok"}`,
		`{"type":"human","isMeta":true,"text":"system prompt"}`,
	)
	cache := NewJsonlCache()
	parsed, err := cache.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.UserMessages) != 1 {
		t.Fatalf("expected 1 user message, got %d", len(parsed.UserMessages))
	}
	if parsed.UserMessages[0] != "build the app" {
		t.Errorf("expected 'build the app', got %q", parsed.UserMessages[0])
	}
}

func TestDeduplicateByMessageID(t *testing.T) {
	path := writeTempJSONL(t,
		`{"type":"assistant","message":{"id":"msg_dup","model":"claude-sonnet-4-6","usage":{"input_tokens":10,"output_tokens":5},"content":[]},"timestamp":"2026-01-01T00:00:00Z"}`,
		`{"type":"assistant","message":{"id":"msg_dup","model":"claude-sonnet-4-6","usage":{"input_tokens":100,"output_tokens":50},"content":[]},"timestamp":"2026-01-01T00:00:01Z"}`,
	)
	cache := NewJsonlCache()
	parsed, err := cache.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Entries) != 1 {
		t.Fatalf("expected 1 deduped entry, got %d", len(parsed.Entries))
	}
	if parsed.Entries[0].Usage.InputTokens != 100 {
		t.Errorf("expected higher token count (100), got %d", parsed.Entries[0].Usage.InputTokens)
	}
}

func TestDetectModelCommand(t *testing.T) {
	path := writeTempJSONL(t,
		`{"type":"tool_result","stdout":"Set model to Claude Opus 4.6"}`,
	)
	cache := NewJsonlCache()
	parsed, err := cache.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.LastModelCmd == nil {
		t.Fatal("expected model command to be detected")
	}
	if parsed.LastModelCmd.ModelID != "claude-opus-4-6" {
		t.Errorf("expected claude-opus-4-6, got %s", parsed.LastModelCmd.ModelID)
	}
}

func TestDetectEffortCommand(t *testing.T) {
	path := writeTempJSONL(t,
		`{"type":"tool_result","stdout":"<command-args>low</command-args>"}`,
	)
	cache := NewJsonlCache()
	parsed, err := cache.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.LastEffortCmd == nil {
		t.Fatal("expected effort command to be detected")
	}
	if parsed.LastEffortCmd.Level != "low" {
		t.Errorf("expected low, got %s", parsed.LastEffortCmd.Level)
	}
}

func TestStripANSI(t *testing.T) {
	input := "\x1b[32mSet model to\x1b[0m Claude Opus 4.6"
	got := stripAnsi(input)
	want := "Set model to Claude Opus 4.6"
	if got != want {
		t.Errorf("stripAnsi(%q) = %q, want %q", input, got, want)
	}
}

func TestMtimeCache(t *testing.T) {
	path := writeTempJSONL(t,
		`{"type":"assistant","message":{"id":"msg_c1","model":"claude-sonnet-4-6","usage":{"input_tokens":10,"output_tokens":5},"content":[]},"timestamp":"2026-01-01T00:00:00Z"}`,
	)
	cache := NewJsonlCache()

	p1, err := cache.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Same mtime → should return same pointer
	p2, err := cache.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if p1 != p2 {
		t.Error("expected cached result (same pointer)")
	}

	// Touch file to change mtime
	time.Sleep(10 * time.Millisecond)
	now := time.Now().Add(time.Second)
	os.Chtimes(path, now, now)

	p3, err := cache.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if p3 == p1 {
		t.Error("expected re-parsed result after mtime change")
	}
}

func TestEmptyFile(t *testing.T) {
	path := writeTempJSONL(t)
	cache := NewJsonlCache()
	parsed, err := cache.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Entries) != 0 {
		t.Errorf("expected 0 entries from empty file, got %d", len(parsed.Entries))
	}
	if len(parsed.UserMessages) != 0 {
		t.Errorf("expected 0 user messages from empty file, got %d", len(parsed.UserMessages))
	}
}

func TestMalformedJSON(t *testing.T) {
	path := writeTempJSONL(t,
		`{not valid json`,
		`{"type":"assistant","message":{"id":"msg_ok","model":"claude-sonnet-4-6","usage":{"input_tokens":1,"output_tokens":1},"content":[]},"timestamp":"2026-01-01T00:00:00Z"}`,
		``,
		`also broken {{{`,
	)
	cache := NewJsonlCache()
	parsed, err := cache.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Entries) != 1 {
		t.Errorf("expected 1 valid entry, got %d", len(parsed.Entries))
	}
}

func TestInferEffortOpusThinking(t *testing.T) {
	got := InferEffort("claude-opus-4-6", true, nil)
	if got != "max" {
		t.Errorf("expected max, got %s", got)
	}
}

func TestInferEffortSonnetThinking(t *testing.T) {
	got := InferEffort("claude-sonnet-4-6", true, nil)
	if got != "high" {
		t.Errorf("expected high, got %s", got)
	}
}

func TestInferEffortHaikuNoThinking(t *testing.T) {
	got := InferEffort("claude-haiku-4-5", false, nil)
	if got != "low" {
		t.Errorf("expected low, got %s", got)
	}
}

func TestInferEffortDefault(t *testing.T) {
	got := InferEffort("claude-sonnet-4-6", false, nil)
	if got != "auto" {
		t.Errorf("expected auto, got %s", got)
	}
}

func TestEffortCommandOverride(t *testing.T) {
	cmd := &EffortCommand{Timestamp: time.Now(), Level: "low"}
	got := InferEffort("claude-opus-4-6", true, cmd)
	if got != "low" {
		t.Errorf("expected low (command override), got %s", got)
	}
}

func TestAssistantEntryWithThinking(t *testing.T) {
	path := writeTempJSONL(t,
		`{"type":"assistant","message":{"id":"msg_think","model":"claude-opus-4-6","usage":{"input_tokens":100,"output_tokens":50},"content":[{"type":"thinking","text":"let me think..."},{"type":"text","text":"hello"}]},"timestamp":"2026-01-01T00:00:00Z"}`,
	)
	cache := NewJsonlCache()
	parsed, err := cache.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(parsed.Entries))
	}
	if !parsed.Entries[0].HasThinking {
		t.Error("expected HasThinking to be true")
	}
}

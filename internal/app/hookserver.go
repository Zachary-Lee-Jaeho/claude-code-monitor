package app

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jaeho/ccmo/internal/config"
	"github.com/jaeho/ccmo/internal/security"
)

// HookEvent represents a Claude Code hook event received via HTTP.
type HookEvent struct {
	SessionID     string `json:"session_id"`
	CWD           string `json:"cwd"`
	HookEventName string `json:"hook_event_name"`
	ToolName      string `json:"tool_name,omitempty"`
	ToolInput     json.RawMessage `json:"tool_input,omitempty"`
	Timestamp     time.Time
}

// HookServer receives Claude Code hook events on 127.0.0.1:7777.
type HookServer struct {
	mu       sync.Mutex
	server   *http.Server
	secret   string
	events   []HookEvent
	maxEvents int
	listener net.Listener
}

// NewHookServer creates a hook server. Pass empty secret to auto-generate.
func NewHookServer() *HookServer {
	secret := loadOrCreateSecret()
	return &HookServer{
		secret:    secret,
		maxEvents: 1000,
	}
}

// Start begins listening on 127.0.0.1:7777.
func (h *HookServer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/hooks/event", h.handleEvent)
	mux.HandleFunc("/health", h.handleHealth)

	h.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	// Bind to 127.0.0.1 ONLY — never 0.0.0.0
	var err error
	h.listener, err = net.Listen("tcp", "127.0.0.1:7777")
	if err != nil {
		return fmt.Errorf("hook server listen: %w", err)
	}

	go h.server.Serve(h.listener)
	return nil
}

// Stop gracefully shuts down the hook server.
func (h *HookServer) Stop() {
	if h.server != nil {
		h.server.Close()
	}
}

// RecentEvents returns the last N events.
func (h *HookServer) RecentEvents(n int) []HookEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	if n > len(h.events) {
		n = len(h.events)
	}
	result := make([]HookEvent, n)
	copy(result, h.events[len(h.events)-n:])
	return result
}

// Secret returns the shared secret for hook URL configuration.
func (h *HookServer) Secret() string {
	return h.secret
}

func (h *HookServer) handleEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Verify shared secret
	token := r.URL.Query().Get("token")
	if h.secret != "" && token != h.secret {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1024*1024)) // 1MB max
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	var event HookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	event.Timestamp = time.Now()

	h.mu.Lock()
	h.events = append(h.events, event)
	if len(h.events) > h.maxEvents {
		h.events = h.events[len(h.events)-h.maxEvents:]
	}
	h.mu.Unlock()

	w.WriteHeader(http.StatusOK)
}

func (h *HookServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"ok","events":%d}`, len(h.events))
}

// Secret management

func secretPath() string {
	return filepath.Join(config.AppDir(), "hook-secret")
}

func loadOrCreateSecret() string {
	path := secretPath()

	// Try to load existing secret
	data, err := os.ReadFile(path)
	if err == nil && len(data) > 0 {
		return string(data)
	}

	// Generate new secret
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "" // no secret — still works but without auth
	}
	secret := hex.EncodeToString(bytes)

	// Save with secure permissions
	security.EnsureDir(config.AppDir())
	os.WriteFile(path, []byte(secret), 0o600)
	security.EnsureFilePermissions(path, 0o600)

	return secret
}

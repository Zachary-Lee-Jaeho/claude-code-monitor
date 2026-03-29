package remote

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jaeho/ccmo/internal/config"
	"github.com/jaeho/ccmo/internal/data"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// SSHCollector manages an SSH connection to a single remote server
// and periodically collects Claude Code session data.
// remoteFileCache tracks byte offset, cached content, and parsed result for incremental reads.
type remoteFileCache struct {
	mtime   time.Time
	size    int64
	content string           // accumulated raw content
	parsed  *data.CachedFile // cached parse result — only re-parse when content changes
	dirty   bool             // true when content changed since last parse
}

type SSHCollector struct {
	cfg       config.ServerConfig
	mu        sync.Mutex
	client    *ssh.Client
	agentConn net.Conn // SSH agent socket — closed on Stop
	threads   []data.Thread
	cache     *data.JsonlCache
	pathCache data.PathCache
	fileCache map[string]*remoteFileCache // path → cached content+offset
	lastErr   string
	connected bool

	ctx    context.Context
	cancel context.CancelFunc
	refresh chan struct{}
}

func NewSSHCollector(cfg config.ServerConfig) *SSHCollector {
	ctx, cancel := context.WithCancel(context.Background())
	return &SSHCollector{
		cfg:       cfg,
		cache:     data.NewJsonlCache(),
		pathCache: make(data.PathCache),
		fileCache: make(map[string]*remoteFileCache),
		ctx:       ctx,
		cancel:    cancel,
		refresh:   make(chan struct{}, 1),
	}
}

// Run starts the collection loop. Call in a goroutine.
func (c *SSHCollector) Run() {
	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		if err := c.connect(); err != nil {
			c.mu.Lock()
			c.lastErr = err.Error()
			c.connected = false
			c.mu.Unlock()

			select {
			case <-c.ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		// Connected — reset backoff
		backoff = time.Second
		c.mu.Lock()
		c.connected = true
		c.lastErr = ""
		c.mu.Unlock()

		// Collection loop
		c.collectLoop()
	}
}

func (c *SSHCollector) collectLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Initial collection
	c.collect()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.collect()
		case <-c.refresh:
			c.collect()
		}
	}
}

func (c *SSHCollector) collect() {
	// Scan processes
	processes := c.scanProcesses()

	// Scan projects
	claudeDir := c.cfg.ClaudeDir
	if claudeDir == "" {
		claudeDir = "~/.claude"
	}
	projectsDir := claudeDir + "/projects"

	folders, err := c.execCommand(fmt.Sprintf("ls -1 %s 2>/dev/null", projectsDir))
	if err != nil {
		c.mu.Lock()
		c.lastErr = "ls projects: " + err.Error()
		c.mu.Unlock()
		return
	}

	var threads []data.Thread
	now := time.Now()
	folderList := splitLines(folders)

	// Single find command for ALL folders at once — resolve ~ via shell
	// Use `find` with shell-expanded path; capture resolved prefix from output
	allFiles := c.listJsonlFiles(projectsDir)

	// Group files by folder — extract folder from path structure
	// Paths from find are like: /home/user/.claude/projects/FOLDER/file.jsonl
	// We extract the folder by looking at the parent directory name of each file
	filesByFolder := make(map[string][]remoteFileInfo)
	for _, fi := range allFiles {
		dir := filepath.Dir(fi.path)       // .../projects/FOLDER
		folder := filepath.Base(dir)        // FOLDER
		if folder == "" || folder == "." {
			continue
		}
		filesByFolder[folder] = append(filesByFolder[folder], fi)
	}

	for _, folder := range folderList {
		if folder == "" || folder == "memory" {
			continue
		}

		projectPath := data.DecodeProjectPathHeuristic(folder)

		files := filesByFolder[folder]

		for _, fi := range files {
			// Incremental read: skip if mtime unchanged
			cached := c.fileCache[fi.path]
			if cached != nil && cached.mtime.Equal(fi.mtime) {
				// Unchanged — reuse cached parsed result directly
				cached.dirty = false
			} else if cached != nil && fi.size > cached.size {
				// File grew — read only new bytes (offset is 1-based in tail)
				offset := cached.size + 1
				newContent, err := c.execCommand(fmt.Sprintf("tail -c +%d '%s'", offset, fi.path))
				if err != nil {
					continue
				}
				cached.content += newContent
				cached.mtime = fi.mtime
				cached.size = fi.size
				cached.dirty = true
			} else {
				// New file or file was rewritten (shrunk) — full read
				content, err := c.execCommand(fmt.Sprintf("cat '%s'", fi.path))
				if err != nil {
					continue
				}
				c.fileCache[fi.path] = &remoteFileCache{
					mtime:   fi.mtime,
					size:    fi.size,
					content: content,
					dirty:   true,
				}
				cached = c.fileCache[fi.path]
			}

			if len(cached.content) == 0 {
				continue
			}

			// Only re-parse when content actually changed
			if cached.dirty || cached.parsed == nil {
				parsed, err := data.ParseJSONLContent(cached.content)
				if err != nil {
					continue
				}
				cached.parsed = parsed
				cached.dirty = false
			}

			thread := buildRemoteThread(cached.parsed, folder, filepath.Base(fi.path), projectPath, now)
			if thread != nil {
				assignRemotePID(thread, processes)
				assignRemoteStatus(thread, now)
				threads = append(threads, *thread)
			}
		}
	}

	c.mu.Lock()
	c.threads = threads
	c.mu.Unlock()
}

type remoteFileInfo struct {
	path  string
	mtime time.Time
	size  int64
}

func (c *SSHCollector) listJsonlFiles(dir string) []remoteFileInfo {
	// Get all jsonl files with mtimes and sizes
	output, err := c.execCommand(fmt.Sprintf(
		"find %s -name '*.jsonl' -exec stat -c '%%Y %%s %%n' {} + 2>/dev/null", dir))
	if err != nil {
		return nil
	}

	var files []remoteFileInfo
	for _, line := range splitLines(output) {
		parts := strings.SplitN(line, " ", 3)
		if len(parts) != 3 {
			continue
		}
		epoch, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			continue
		}
		size, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}
		files = append(files, remoteFileInfo{
			path:  parts[2],
			mtime: time.Unix(epoch, 0),
			size:  size,
		})
	}
	return files
}

type remoteProcess struct {
	pid       uint32
	cwd       string
	sessionID string
}

func (c *SSHCollector) scanProcesses() []remoteProcess {
	// Single command: get PIDs, args, and CWDs in one shot
	// Output format: PID|CWD|ARGS (one line per process)
	output, err := c.execCommand(
		`ps -eo pid,args 2>/dev/null | grep -E 'claude|ccd-cli' | grep -v grep | ` +
			`awk '{pid=$1; $1=""; args=$0; cwd=""; cmd="readlink /proc/"pid"/cwd 2>/dev/null"; cmd | getline cwd; close(cmd); if(cwd!="") print pid"|"cwd"|"args}'`)
	if err != nil {
		return nil
	}
	var result []remoteProcess
	for _, line := range splitLines(output) {
		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			continue
		}
		pid, err := strconv.ParseUint(strings.TrimSpace(parts[0]), 10, 32)
		if err != nil {
			continue
		}
		cwd := strings.TrimSpace(parts[1])
		if cwd == "" {
			continue
		}
		args := strings.TrimSpace(parts[2])

		// Extract --resume session ID from args
		sessionID := ""
		fields := strings.Fields(args)
		for i, f := range fields {
			if f == "--resume" && i+1 < len(fields) {
				sessionID = fields[i+1]
				break
			}
		}

		result = append(result, remoteProcess{
			pid:       uint32(pid),
			cwd:       cwd,
			sessionID: sessionID,
		})
	}
	return result
}

func (c *SSHCollector) connect() error {
	sshConfig := &ssh.ClientConfig{
		Timeout: 10 * time.Second,
	}

	// Parse host (user@hostname)
	user := "root"
	hostname := c.cfg.Host
	if at := strings.Index(hostname, "@"); at >= 0 {
		user = hostname[:at]
		hostname = hostname[at+1:]
	}
	sshConfig.User = user

	// Host key verification
	// Go's knownhosts library has issues with hashed entries + non-standard ports.
	// We wrap the callback: on "key mismatch", verify via ssh-keygen -F as fallback.
	knownHostsPath := filepath.Join(os.Getenv("HOME"), ".ssh", "known_hosts")
	if _, err := os.Stat(knownHostsPath); err == nil {
		hostKeyCallback, err := knownhosts.New(knownHostsPath)
		if err == nil {
			sshConfig.HostKeyCallback = func(hostname string, remote net.Addr, key ssh.PublicKey) error {
				err := hostKeyCallback(hostname, remote, key)
				if err == nil {
					return nil
				}
				// On mismatch with non-standard port, try ssh-keygen -F verification
				if c.cfg.Port != 0 && c.cfg.Port != 22 {
					if verifyHostKeySSHKeygen(hostname, key) {
						return nil
					}
				}
				return err
			}
		} else {
			sshConfig.HostKeyCallback = ssh.InsecureIgnoreHostKey()
		}
	} else {
		sshConfig.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	}

	// Auth methods
	var authMethods []ssh.AuthMethod

	// 1. Try ssh-agent first
	if agentAuth, agentConn := sshAgentConn(); agentAuth != nil {
		authMethods = append(authMethods, agentAuth)
		c.mu.Lock()
		if c.agentConn != nil {
			c.agentConn.Close()
		}
		c.agentConn = agentConn
		c.mu.Unlock()
	}

	// 2. Try identity file
	if c.cfg.IdentityFile != "" {
		keyPath := expandHome(c.cfg.IdentityFile)
		if key, err := os.ReadFile(keyPath); err == nil {
			if signer, err := ssh.ParsePrivateKey(key); err == nil {
				authMethods = append(authMethods, ssh.PublicKeys(signer))
			}
			// Zero key material
			for i := range key {
				key[i] = 0
			}
		}
	}

	// 3. Default key paths
	for _, keyName := range []string{"id_ed25519", "id_rsa"} {
		keyPath := filepath.Join(os.Getenv("HOME"), ".ssh", keyName)
		if key, err := os.ReadFile(keyPath); err == nil {
			if signer, err := ssh.ParsePrivateKey(key); err == nil {
				authMethods = append(authMethods, ssh.PublicKeys(signer))
			}
			for i := range key {
				key[i] = 0
			}
		}
	}

	if len(authMethods) == 0 {
		return fmt.Errorf("no SSH auth methods available for %s", c.cfg.Name)
	}
	sshConfig.Auth = authMethods

	addr := fmt.Sprintf("%s:%d", hostname, c.cfg.Port)
	client, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return fmt.Errorf("ssh dial %s: %w", addr, err)
	}

	c.mu.Lock()
	if c.client != nil {
		c.client.Close()
	}
	c.client = client
	c.mu.Unlock()

	// Start keepalive
	go c.keepalive()

	return nil
}

func (c *SSHCollector) keepalive() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.mu.Lock()
			client := c.client
			c.mu.Unlock()
			if client == nil {
				return
			}
			_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
			if err != nil {
				c.mu.Lock()
				c.connected = false
				c.lastErr = "keepalive failed"
				c.mu.Unlock()
				return
			}
		}
	}
}

func (c *SSHCollector) execCommand(cmd string) (string, error) {
	c.mu.Lock()
	client := c.client
	c.mu.Unlock()

	if client == nil {
		return "", fmt.Errorf("not connected")
	}

	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	var stdout bytes.Buffer
	session.Stdout = &stdout

	// Timeout via context
	done := make(chan error, 1)
	go func() { done <- session.Run(cmd) }()

	select {
	case err := <-done:
		if err != nil {
			return stdout.String(), err
		}
		return strings.TrimSpace(stdout.String()), nil
	case <-time.After(5 * time.Second):
		return "", fmt.Errorf("command timeout: %s", cmd)
	}
}

// Stop closes the SSH connection and cancels the collection loop.
func (c *SSHCollector) Stop() {
	c.cancel()
	c.mu.Lock()
	if c.client != nil {
		c.client.Close()
		c.client = nil
	}
	if c.agentConn != nil {
		c.agentConn.Close()
		c.agentConn = nil
	}
	c.mu.Unlock()
}

// GetThreads returns a copy of the latest collected threads.
func (c *SSHCollector) GetThreads() []data.Thread {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]data.Thread, len(c.threads))
	copy(result, c.threads)
	return result
}

// TriggerRefresh signals the collector to refresh immediately.
func (c *SSHCollector) TriggerRefresh() {
	select {
	case c.refresh <- struct{}{}:
	default:
	}
}

func (c *SSHCollector) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

func (c *SSHCollector) LastError() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastErr
}

// Helper functions

func sshAgentConn() (ssh.AuthMethod, net.Conn) {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return nil, nil
	}
	conn, err := net.Dial("unix", sock)
	if err != nil {
		return nil, nil
	}
	agentClient := agent.NewClient(conn)
	return ssh.PublicKeysCallback(agentClient.Signers), conn
}

// verifyHostKeySSHKeygen uses ssh-keygen -F to verify a host key.
// Workaround for Go knownhosts library issues with hashed entries + non-standard ports.
func verifyHostKeySSHKeygen(hostname string, key ssh.PublicKey) bool {
	// Go passes "host:port" but ssh-keygen expects "[host]:port" for non-standard ports
	host, port, err := net.SplitHostPort(hostname)
	if err == nil && port != "22" {
		hostname = fmt.Sprintf("[%s]:%s", host, port)
	} else if err == nil {
		hostname = host
	}
	cmd := exec.Command("ssh-keygen", "-F", hostname)
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return false
	}
	// Parse output, find key type + base64 key, compare
	keyType := key.Type()
	keyBase64 := base64.StdEncoding.EncodeToString(key.Marshal())
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		fields := strings.Fields(line)
		// Format: host keytype base64key
		for i, f := range fields {
			if f == keyType && i+1 < len(fields) {
				if fields[i+1] == keyBase64 {
					return true
				}
			}
		}
	}
	return false
}

// TestConnection tests SSH connectivity to a server and returns diagnostics.
func TestConnection(cfg config.ServerConfig) error {
	c := NewSSHCollector(cfg)
	defer c.Stop()

	if err := c.connect(); err != nil {
		return fmt.Errorf("SSH connect failed: %w", err)
	}

	// Verify we can run a command
	output, err := c.execCommand("echo ok")
	if err != nil {
		return fmt.Errorf("command execution failed: %w", err)
	}
	if strings.TrimSpace(output) != "ok" {
		return fmt.Errorf("unexpected response: %q", output)
	}

	// Check claude dir exists
	claudeDir := cfg.ClaudeDir
	if claudeDir == "" {
		claudeDir = "~/.claude"
	}
	_, err = c.execCommand(fmt.Sprintf("test -d %s/projects && echo found", claudeDir))
	if err != nil {
		return fmt.Errorf("claude projects directory not found at %s/projects", claudeDir)
	}

	return nil
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func buildRemoteThread(parsed *data.CachedFile, folderName, sessionFile, projectPath string, now time.Time) *data.Thread {
	if parsed == nil || (len(parsed.Entries) == 0 && len(parsed.UserMessages) == 0) {
		return nil
	}

	t := &data.Thread{
		FolderName:    folderName,
		SessionFile:   sessionFile,
		ProjectPath:   projectPath,
		PerModelUsage: make(map[string]*data.TokenUsage),
		LastEffort:    "auto",
	}

	window5h := now.Add(-5 * time.Hour)
	window7d := now.Add(-7 * 24 * time.Hour)
	var lastEntry *data.AssistantEntry

	for i := range parsed.Entries {
		e := &parsed.Entries[i]

		if _, ok := t.PerModelUsage[e.Model]; !ok {
			t.PerModelUsage[e.Model] = &data.TokenUsage{}
		}
		t.PerModelUsage[e.Model].Add(&e.Usage)
		t.TotalUsage.Add(&e.Usage)

		cost, saved := data.CalculateCost(e.Model, &e.Usage)
		t.TotalCost += cost
		t.SavedCost += saved

		if !e.Timestamp.IsZero() {
			if t.FirstActivity.IsZero() || e.Timestamp.Before(t.FirstActivity) {
				t.FirstActivity = e.Timestamp
			}
			if e.Timestamp.After(t.LastActivity) {
				t.LastActivity = e.Timestamp
			}
			if e.Timestamp.After(window5h) {
				t.Window5hUsage.Add(&e.Usage)
				t.Window5hMsgCount++
			}
			if e.Timestamp.After(window7d) {
				wcost, _ := data.CalculateCost(e.Model, &e.Usage)
				t.WeeklyCost += wcost
			}
		}

		if lastEntry == nil || e.Timestamp.After(lastEntry.Timestamp) {
			lastEntry = e
		}
	}

	if lastEntry != nil {
		t.LastModel = lastEntry.Model
		t.LastCtxUsed = lastEntry.Usage.TotalInputAll()
	}

	// Effort inference
	hasThinking := lastEntry != nil && lastEntry.HasThinking
	t.LastEffort = data.InferEffort(t.LastModel, hasThinking, parsed.LastEffortCmd)

	// Burn rate
	duration := t.LastActivity.Sub(t.FirstActivity)
	if duration > time.Minute {
		t.BurnRate = float64(t.TotalUsage.OutputTokens) / duration.Minutes()
	}

	t.IsActive = now.Sub(t.LastActivity) < 30*time.Minute

	// Recent commands (last 10)
	t.RecentCommands = parsed.UserMessages
	if len(t.RecentCommands) > 10 {
		t.RecentCommands = t.RecentCommands[len(t.RecentCommands)-10:]
	}

	return t
}

func assignRemotePID(t *data.Thread, processes []remoteProcess) {
	sessionBase := strings.TrimSuffix(t.SessionFile, ".jsonl")
	// Exact session match first
	for _, p := range processes {
		if p.sessionID != "" && (p.sessionID == t.SessionFile || p.sessionID == sessionBase) {
			t.PID = p.pid
			return
		}
	}
	// Fallback: CWD match (highest PID)
	for _, p := range processes {
		if p.cwd == t.ProjectPath && p.sessionID == "" {
			if p.pid > t.PID {
				t.PID = p.pid
			}
		}
	}
}

func assignRemoteStatus(t *data.Thread, now time.Time) {
	hasPID := t.PID > 0
	stale := now.Sub(t.LastActivity) >= 5*time.Minute

	switch {
	case hasPID && !stale:
		t.Status = data.StatusRunning
	case hasPID:
		t.Status = data.StatusWaiting
	case now.Sub(t.LastActivity) < 30*time.Minute:
		t.Status = data.StatusIdle
	default:
		t.Status = data.StatusIdle
	}
}

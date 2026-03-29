package remote

import (
	"fmt"
	"sync"

	"github.com/jaeho/ccmo/internal/config"
	"github.com/jaeho/ccmo/internal/data"
)

// Manager coordinates remote server connections and data collection.
type Manager struct {
	mu         sync.Mutex
	collectors map[string]*SSHCollector // name → collector
	threads    map[string][]data.Thread // name → threads from that server
}

func NewManager() *Manager {
	return &Manager{
		collectors: make(map[string]*SSHCollector),
		threads:    make(map[string][]data.Thread),
	}
}

// Start initializes connections to all enabled servers.
func (m *Manager) Start() error {
	servers, err := config.LoadServers()
	if err != nil {
		return fmt.Errorf("load servers: %w", err)
	}

	for _, srv := range servers {
		if !srv.Enabled {
			continue
		}
		collector := NewSSHCollector(srv)
		m.mu.Lock()
		m.collectors[srv.Name] = collector
		m.mu.Unlock()

		go collector.Run()
	}
	return nil
}

// Stop closes all SSH connections.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.collectors {
		c.Stop()
	}
	m.collectors = make(map[string]*SSHCollector)
}

// GetThreads returns all remote threads, tagged with host name.
// If a server connection is failing, returns a synthetic error thread.
func (m *Manager) GetThreads() []data.Thread {
	m.mu.Lock()
	defer m.mu.Unlock()

	var all []data.Thread
	for name, c := range m.collectors {
		threads := c.GetThreads()
		for i := range threads {
			threads[i].Host = name
		}
		if len(threads) == 0 && !c.IsConnected() && c.LastError() != "" {
			// Show a placeholder error thread so user knows the server is failing
			errThread := data.Thread{
				Host:        name,
				Status:      data.StatusError,
				SessionFile: c.LastError(),
			}
			threads = append(threads, errThread)
		}
		all = append(all, threads...)
	}
	return all
}

// Refresh triggers a data refresh on all collectors.
func (m *Manager) Refresh() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.collectors {
		c.TriggerRefresh()
	}
}

// ServerStatus returns connection status for each server.
func (m *Manager) ServerStatus() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	status := make(map[string]string)
	for name, c := range m.collectors {
		if c.IsConnected() {
			status[name] = "connected"
		} else if c.LastError() != "" {
			status[name] = "error: " + c.LastError()
		} else {
			status[name] = "connecting"
		}
	}
	return status
}

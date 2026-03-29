# CCMO — Claude Code Monitor

> htop-style TUI for monitoring Claude Code sessions

## Features

- Real-time session monitoring (tokens, cost, model, status)
- 3-line usage bars (Session / Weekly / Extra quota)
- Sortable 10-column thread table
- Remote server session monitoring via SSH
- Keyboard-driven navigation (vim-style)
- Burn rate warning with quota exhaustion prediction
- Dark theme (24-bit RGB)
- Single static binary, minimal external dependencies

## Install

### Binary

Download from [Releases](https://github.com/jaeho/ccmo/releases):

```bash
# Linux (amd64)
curl -L https://github.com/jaeho/ccmo/releases/latest/download/ccmo-linux-amd64 -o ccmo
chmod +x ccmo && sudo mv ccmo /usr/local/bin/

# macOS (Apple Silicon)
curl -L https://github.com/jaeho/ccmo/releases/latest/download/ccmo-darwin-arm64 -o ccmo
chmod +x ccmo && sudo mv ccmo /usr/local/bin/
```

### go install

```bash
go install github.com/jaeho/ccmo@latest
```

### Build from source

```bash
git clone https://github.com/jaeho/ccmo.git
cd ccmo
go build -o ccmo ./
```

## Quick Start

```bash
# First run — select your plan
ccmo

# Set plan via CLI
ccmo --plan max5    # pro | max5 | max20

# Add a remote server
ccmo remote add dev user@10.0.1.50

# Test SSH connection
ccmo remote test dev

# Launch monitor
ccmo
```

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Up` / `k` | Previous thread |
| `Down` / `j` | Next thread |
| `Left` / `h` | Sort previous column |
| `Right` / `l` | Sort next column |
| `s` / `F2` | Cycle sort forward |
| `r` / `F5` | Force refresh |
| `u` | Refresh server-side quota |
| `d` / `Del` | Delete selected thread |
| `a` | Add remote server dialog |
| `Tab` | Filter: All / Local / per-server |
| `?` | Help overlay |
| `q` / `Ctrl+C` | Quit |

## Configuration

### Plan

Plans determine token and cost limits displayed in the usage bars.

| Plan | Price | Output Tokens/Week |
|------|-------|--------------------|
| Pro | $18/mo | 19k |
| Max 5 | $35/mo | 88k |
| Max 20 | $140/mo | 220k |

Config stored in `~/.ccmo/config.json`.

### Remote Servers

```bash
ccmo remote add <name> <user@host>   # Add server
ccmo remote rm <name>                # Remove server
ccmo remote list                     # List all servers
ccmo remote test <name>              # Test SSH connection
```

Config stored in `~/.ccmo/servers.json`. Uses SSH agent or key file authentication.

### Hooks (Optional)

CCMO works without any hooks. For optional real-time event streaming:

```bash
ccmo hooks install    # Add hooks to ~/.claude/settings.json
ccmo hooks remove     # Remove hooks
ccmo hooks status     # Check installed hooks
```

## Architecture

```
Local JSONL ──── mtime poll (2s) ────────────┐
                                              │
Remote JSONL ─── SSH exec + mtime cache (5s) ─┤
                                              ├──> App State ──> tview Render (2s)
Local process ── gopsutil / /proc ────────────┤
                                              │
Remote process ─ SSH exec (5s) ───────────────┤
                                              │
OAuth API ────── background goroutine ────────┤
                                              │
HTTP hooks ───── 127.0.0.1:7777 (OPTIONAL) ──┘
```

**Data sources:** Session JSONL files (primary), process detection, OAuth API, and optional HTTP hooks.

## Cross-compile

```bash
GOOS=darwin GOARCH=arm64 go build -o ccmo-darwin-arm64 ./
GOOS=linux GOARCH=amd64 go build -o ccmo-linux-amd64 ./
```

## License

MIT

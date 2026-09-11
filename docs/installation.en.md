# Installation & Operations

[English](installation.en.md) | [中文](installation.md)

## Contents

- [Installation](#installation)
  - [Binary Download](#binary-download)
  - [Docker](#docker)
  - [Build from Source](#build-from-source)
- [Register MCP Client](#register-mcp-client)
  - [Claude Code](#claude-code)
  - [Qwen Code](#qwen-code)
  - [Cursor / Other Clients](#cursor--other-clients)
- [stdio CLI](#stdio-cli)
- [Agent Quick Deploy](#agent-quick-deploy)
- [Operations](#operations)
  - [Subcommands](#subcommands)
  - [Windows Auto-Start](#windows-auto-start)
  - [MCP Hooks Auto Start/Stop](#mcp-hooks-auto-startstop)
  - [Background Service](#background-service)
  - [Health Check & Admin Endpoints](#health-check--admin-endpoints)
- [Troubleshooting](#troubleshooting)

---

## Installation

### Binary Download

Download from the [Release page](https://github.com/daidaiJ/websearch-mcpserver/releases); each binary ships with a matching `.sha256` checksum.

**HTTP daemon** (`start`, then listen on `/mcp`):

| Platform | File |
|----------|------|
| Linux x86_64 | `websearch-mcpserver-linux-amd64` |
| Windows x86_64 | `websearch-mcpserver-windows-amd64.exe` |
| macOS Intel | `websearch-mcpserver-darwin-amd64` |
| macOS Apple Silicon | `websearch-mcpserver-darwin-arm64` |

> **Split release matrices**: GitHub Release only ships the four platforms above (linux/windows amd64 + darwin amd64/arm64). There is no linux-arm64 binary — use the GHCR image below. Windows ARM64 is not published. MCP Registry `.mcpb` bundles match these four platforms.
>
> **Tagging** (two steps, do not push together): first push a plain version tag (e.g. `v3.4.0`) to publish the GitHub Release **and** the GHCR image; **after that Release is ready**, separately push a `-registry` suffix tag (e.g. `v3.4.0-registry`) to publish to the MCP Registry only (packs `.mcpb` from the existing `v3.4.0` Release). The follow-up registry tag stays on the same commit.

**stdio CLI** (spawned by the MCP client, no HTTP port):

| Platform | File |
|----------|------|
| Linux x86_64 | `websearch-mcp-cli-linux-amd64` |
| Windows x86_64 | `websearch-mcp-cli-windows-amd64.exe` |
| macOS Intel | `websearch-mcp-cli-darwin-amd64` |
| macOS Apple Silicon | `websearch-mcp-cli-darwin-arm64` |

### Docker

Official images are on GHCR as a **linux/amd64 + linux/arm64** manifest (Apple Silicon / ARM servers can pull directly):

```bash
docker pull ghcr.io/daidaij/websearch-mcpserver:latest
# or pin: ghcr.io/daidaij/websearch-mcpserver:3.4.0
```

```yaml
# docker-compose.yml
services:
  websearch:
    image: ghcr.io/daidaij/websearch-mcpserver:latest
    restart: always
    volumes:
      - ./config.yaml:/app/config.yaml
    ports:
      - "8338:8338"
```

Build locally:

```bash
git clone --depth 1 https://github.com/daidaiJ/websearch-mcpserver.git
cd websearch-mcpserver && docker build -t websearch:v1 .
```

### Build from Source

```bash
go build -o websearch ./cmd/
go build -o websearch-mcp-cli ./cmd/cli
# With version injection
go build -ldflags="-X main.version=v1.0.0" -o websearch ./cmd/
go build -ldflags="-X main.version=v1.0.0" -o websearch-mcp-cli ./cmd/cli
```

---

## Register MCP Client

After the service starts (HTTP daemon listens on `http://localhost:8338/mcp`), register it with your client. Both **Claude Code** and **Qwen Code** configurations are shown below.

> **Auth**: if `auth_token` is set (or env `WEBSEARCH_TOKEN`), clients must send `Authorization: Bearer <token>` to `/mcp` and `/searxng/search`, otherwise 401. With no token, no header is needed.

### Claude Code

**CLI**:

```bash
claude mcp add --transport http websearch http://localhost:8338/mcp
# when auth_token is set:
claude mcp add --transport http websearch http://localhost:8338/mcp --header "Authorization: Bearer <token>"
```

**JSON** (`.claude.json` / `mcp.json`):

```json
{
  "mcpServers": {
    "websearch": {
      "type": "http",
      "url": "http://localhost:8338/mcp",
      "headers": {
        "Authorization": "Bearer <token>"
      }
    }
  }
}
```

### Qwen Code

> Qwen Code uses `httpUrl` for HTTP transport (not Claude Code's `type`+`url`), and `command`+`args` for stdio.

**CLI** (writes to user-level `~/.qwen/settings.json`; use `-s project` for project scope):

```bash
qwen mcp add --transport http websearch http://localhost:8338/mcp
```

**JSON** (`~/.qwen/settings.json` or project `.qwen/settings.json`):

```jsonc
{
  "mcpServers": {
    "websearch": {
      "httpUrl": "http://localhost:8338/mcp",
      "headers": {
        "Authorization": "Bearer <token>"
      }
    }
  }
}
```

> Omit `headers` when `auth_token` is not configured.

Qwen Code common flags:

| Flag | Description |
|------|-------------|
| `-s, --scope` | `user` (default, global) / `project` (current project only) |
| `--timeout` | Tool-call timeout in ms, default 600000 (10 min) |
| `--trust` | Trust the server, skip tool-call confirmations |
| `-e KEY=value` | Inject environment variables (e.g. `-e BAIDU_SK=xxx`) |

Restart Qwen Code (`/exit` then relaunch) for changes to take effect; check connection status with `/mcp`.

### Cursor / Other Clients

```json
{
  "mcpServers": {
    "websearch": {
      "type": "http",
      "url": "http://localhost:8338/mcp"
    }
  }
}
```

---

## stdio CLI

To skip the HTTP daemon, download `websearch-mcp-cli-*` and let the client spawn the process (stdin/stdout by default). With no config file it uses in-memory defaults (`mode: engine`).

```bash
# Optional: write an example config
./websearch-mcp-cli init
# Or a specific path
./websearch-mcp-cli -c ~/.config/websearch/config.yaml init
```

**Claude Code**:

```json
{
  "mcpServers": {
    "websearch": {
      "command": "/path/to/websearch-mcp-cli",
      "args": ["-c", "/path/to/config.yaml"]
    }
  }
}
```

**Qwen Code**:

```bash
qwen mcp add websearch /path/to/websearch-mcp-cli -c /path/to/config.yaml
```

```jsonc
{
  "mcpServers": {
    "websearch": {
      "command": "/path/to/websearch-mcp-cli",
      "args": ["-c", "/path/to/config.yaml"]
    }
  }
}
```

Omit `args` when running with defaults. Logs go to stderr and `websearch.log` under the config directory so they do not corrupt JSON-RPC on stdout.

CLI commands: no args = stdio; `init` writes an example config; `version` prints the version.

Config shares the same YAML as HTTP; see [configuration.en.md#stdio-cli-configuration-notes](configuration.en.md#stdio-cli-configuration-notes) for differences such as `port`.

---

## Agent Quick Deploy

Agents can use these steps for automated installation:

```bash
# 1. Download latest binary
#    Linux:
curl -sL https://api.github.com/repos/daidaiJ/websearch-mcpserver/releases/latest \
  | grep "browser_download_url.*linux-amd64\"" \
  | cut -d '"' -f 4 \
  | xargs curl -sL -o /usr/local/bin/websearch-mcpserver
chmod +x /usr/local/bin/websearch-mcpserver

#    macOS (Intel):
# curl -sL https://api.github.com/repos/daidaiJ/websearch-mcpserver/releases/latest \
#   | grep "browser_download_url.*darwin-amd64\"" \
#   | cut -d '"' -f 4 \
#   | xargs curl -sL -o /usr/local/bin/websearch-mcpserver
# chmod +x /usr/local/bin/websearch-mcpserver

#    macOS (Apple Silicon):
# curl -sL https://api.github.com/repos/daidaiJ/websearch-mcpserver/releases/latest \
#   | grep "browser_download_url.*darwin-arm64\"" \
#   | cut -d '"' -f 4 \
#   | xargs curl -sL -o /usr/local/bin/websearch-mcpserver
# chmod +x /usr/local/bin/websearch-mcpserver

#    Windows (PowerShell):
# $release = Invoke-RestMethod https://api.github.com/repos/daidaiJ/websearch-mcpserver/releases/latest
# $asset = $release.assets | Where-Object { $_.name -match 'windows-amd64' }
# Invoke-WebRequest -Uri $asset.browser_download_url -OutFile "C:\tools\websearch-mcpserver.exe"

# 2. Write minimal config (zero keys needed)
# You may skip this step: the first `start` auto-generates a preset config.yaml
# (identical to config.example.yaml) next to the executable. With an explicit
# `-c` path, no file is auto-created and a missing file is an error.
mkdir -p ~/.config/websearch
cat > ~/.config/websearch/config.yaml << 'EOF'
port: 8338
mode: engine
EOF

# 3. Start and register (see "Register MCP Client" above)
./websearch-mcpserver start
```

> **What "zero config" means**: `install` / `websearch-mcp-cli init` / first `start` (without `-c`) writes a preset `config.yaml` identical to `config.example.yaml`; edit that file for port/keys/mode. The daemon **never** runs on invisible in-memory defaults without a config file.

Windows auto-start (optional): run `websearch-mcpserver.exe install` after download.

---

## Operations

### Subcommands

HTTP daemon binary `websearch-mcpserver`:

| Command | Description |
|---------|-------------|
| `start` | Start service (ref=1 or ref+1) |
| `stop` | Decrease reference (ref-1, graceful exit at zero) |
| `kill` | Force terminate (ignores reference count) |
| `status` | Show status, port, reference count |
| `version` | Show version (injected at build time, default `dev`) |
| `install` | Install Windows auto-start |
| `uninstall` | Uninstall Windows auto-start |

CLI flags: `-c, --config` to specify config file path.

### Windows Auto-Start

```bash
./websearch-mcpserver.exe install   # Creates VBS script + Startup folder shortcut
./websearch-mcpserver.exe uninstall # Removes shortcut
```

Uses COM API (ole32.dll) to create shortcuts, no PowerShell dependency.

### MCP Hooks Auto Start/Stop

Recommended: use Hooks for automatic session lifecycle (Qwen Code example):

```json
{
  "hooks": {
    "SessionStart": [{ "matcher": "*", "hooks": [{ "type": "command", "command": "/path/to/websearch-mcpserver start", "timeout": 10000 }] }],
    "SessionEnd":   [{ "matcher": "*", "hooks": [{ "type": "command", "command": "/path/to/websearch-mcpserver stop",  "timeout": 10000 }] }]
  }
}
```

Reference counting ensures multi-session sharing; auto-exits when all sessions close.

### Background Service

| Platform | Solution |
|----------|----------|
| Windows | `nssm` register as Windows Service |
| Linux | systemd `Restart=always` |
| macOS | launchd plist |

With a background service, `start` once only — no hooks needed.

#### Linux Auto-Start (systemd)

```bash
# 1. Create systemd service file
sudo tee /etc/systemd/system/websearch.service << 'EOF'
[Unit]
Description=WebSearch MCP Server
After=network.target

[Service]
Type=simple
User=YOUR_USERNAME
ExecStart=/usr/local/bin/websearch-mcpserver start
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# 2. Enable and start
sudo systemctl daemon-reload
sudo systemctl enable websearch
sudo systemctl start websearch

# 3. Check status
sudo systemctl status websearch
```

#### macOS Auto-Start (launchd)

```bash
# 1. Create plist file
tee ~/Library/LaunchAgents/com.websearch.server.plist << 'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.websearch.server</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/websearch-mcpserver</string>
        <string>start</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>WorkingDirectory</key>
    <string>/tmp</string>
</dict>
</plist>
EOF

# 2. Load and start
launchctl load ~/Library/LaunchAgents/com.websearch.server.plist
launchctl start com.websearch.server

# 3. Check status
launchctl list | grep websearch
```

### Health Check & Admin Endpoints

| Item | Description |
|------|-------------|
| **Health Check** | `GET /__admin/health` — returns `{"ref_count": N, "message": "running"}`, remotely accessible |
| **Admin Endpoints** | `GET /__admin/status` · `POST /__admin/refcount` · `POST /__admin/shutdown` — local access only |
| **PID File** | `.websearch.pid` (JSON), in config file directory or executable directory |
| **Log File** | `websearch.log`, same directory, size-rotated (default 1MB, 1 day retention) |
| **Cache** | SQLite WAL mode, 6h expiry (based on last hit), 30min scheduled cleanup |

---

## Troubleshooting

| Problem | Solution |
|---------|----------|
| Tools unavailable after start | Check `mode` and corresponding keys; `cleanfetch` needs `cleanfetch.enabled: true` |
| Academic search timeout | Check `network` setting; overseas engines need proxy (auto-detected by default) |
| Port in use | `status` to check if already running, or `kill` then restart |
| Stale cache results | Cache auto-expires after 6h, or delete `cache.storage_path` file and restart |
| Docker container exits immediately | Confirm `config.yaml` is mounted, check log output |
| Process still running after stop | Wait up to 10s; if still running use `kill` to force terminate |
| No results or rate-limited | Check `rate_limit` config (default 3/s, 60/min); Google etc. auto-skipped when proxy unavailable |

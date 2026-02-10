# Magabot

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

A lightweight, security-first multi-platform chatbot with LLM integration. Single binary, zero runtime dependencies.

---

## Table of Contents

- [Installation](#installation)
- [Quick Start](#quick-start)
- [Setup](#setup)
- [Configuration](#configuration)
- [Secrets Management](#secrets-management)
- [CLI Commands](#cli-commands)
- [Chat Commands](#chat-commands)
- [Platforms](#platforms)
- [LLM Providers](#llm-providers)
- [Cron Jobs](#cron-jobs)
- [Update](#update)
- [Uninstall](#uninstall)
- [Security](#security)
- [File Structure](#file-structure)
- [Docker](#docker)
- [Building from Source](#building-from-source)

---

## Installation

### One-liner (Linux/macOS)

```bash
curl -sL https://raw.githubusercontent.com/kusandriadi/magabot/master/install.sh | bash
```

This auto-detects your OS and architecture (`linux/darwin`, `amd64/arm64/arm`), downloads the latest release from GitHub, and installs to `/usr/local/bin/`.

To install a specific version:

```bash
MAGABOT_VERSION=v0.1.8 curl -sL https://raw.githubusercontent.com/kusandriadi/magabot/master/install.sh | bash
```

### Manual Download

```bash
# Linux amd64
wget https://github.com/kusandriadi/magabot/releases/latest/download/magabot_linux_amd64.tar.gz
tar -xzf magabot_linux_amd64.tar.gz
sudo mv magabot /usr/local/bin/

# macOS arm64 (Apple Silicon)
wget https://github.com/kusandriadi/magabot/releases/latest/download/magabot_darwin_arm64.tar.gz
tar -xzf magabot_darwin_arm64.tar.gz
sudo mv magabot /usr/local/bin/
```

### Windows

Download `magabot_windows_amd64.zip` from [Releases](https://github.com/kusandriadi/magabot/releases), extract it, and add to your PATH.

```powershell
.\magabot.exe setup
```

### User-local Install (no sudo)

```bash
mkdir -p ~/bin
mv magabot ~/bin/
# Ensure ~/bin is in your PATH
```

---

## Quick Start

```bash
# 1. Install
curl -sL https://raw.githubusercontent.com/kusandriadi/magabot/master/install.sh | bash

# 2. Interactive setup
magabot setup

# 3. Start
magabot start

# 4. Check status
magabot status
```

---

## Setup

The interactive wizard guides you through the full configuration:

```bash
magabot setup                # Full wizard (all platforms + LLM)
```

You can also set up individual components:

```bash
# Platforms
magabot setup telegram       # Telegram (needs bot token from @BotFather)
magabot setup discord        # Discord (needs bot token from Developer Portal)
magabot setup slack          # Slack (needs bot token + app token for Socket Mode)
magabot setup whatsapp       # WhatsApp (QR code scan, beta)
magabot setup webhook        # HTTP webhook endpoint

# LLM Providers
magabot setup llm            # All LLM providers
magabot setup anthropic      # Anthropic Claude (API key or Claude CLI OAuth)
magabot setup openai         # OpenAI GPT (API key)
magabot setup gemini         # Google Gemini (API key)
magabot setup deepseek       # DeepSeek (API key)
magabot setup glm            # Zhipu GLM (API key)

# Other
magabot setup admin <id>     # Add a global admin by user ID
magabot setup paths          # Configure data/logs/memory directories
magabot setup skills         # Configure skills directory and auto-reload
```

### Anthropic OAuth

If you have Claude CLI installed, magabot can use its OAuth credentials:

```bash
magabot setup anthropic
# Select "Claude CLI OAuth" when prompted
# Tokens are loaded from ~/.claude/.credentials.json
```

---

## Configuration

Config file location: `~/.magabot/config.yaml`

### View and Edit

```bash
magabot config show          # Show config summary
magabot config edit          # Open in $EDITOR (nano/vim/vi)
magabot config path          # Print config file path
```

### Manage Global Admins

```bash
magabot config admin list          # List global admins
magabot config admin add <id>      # Add global admin
magabot config admin remove <id>   # Remove global admin
```

### Environment Variables

Config values support `$VAR` and `${VAR}` expansion:

```yaml
llm:
  anthropic:
    api_key: "${ANTHROPIC_API_KEY}"
  openai:
    api_key: "$OPENAI_API_KEY"

platforms:
  telegram:
    token: "${TELEGRAM_BOT_TOKEN}"
```

### Full Config Structure

```yaml
bot:
  name: "Magabot"
  prefix: "/"

access:
  mode: "allowlist"          # allowlist | denylist | open
  global_admins: ["YOUR_USER_ID"]

platforms:
  telegram:
    enabled: true
    token: ""
    admins: []
    allowed_users: []
    allowed_chats: []
    allow_groups: true
    allow_dms: true

  discord:
    enabled: false
    token: ""
    prefix: "!"
    admins: []
    allowed_users: []
    allowed_chats: []

  slack:
    enabled: false
    bot_token: ""            # xoxb-...
    app_token: ""            # xapp-... (Socket Mode)
    admins: []
    allowed_users: []
    allowed_chats: []

  whatsapp:
    enabled: false
    admins: []
    allowed_users: []
    allowed_chats: []

  webhook:
    enabled: false
    port: 8080
    path: "/webhook"
    bind: "127.0.0.1"
    auth_method: "bearer"    # none | bearer | hmac
    bearer_token: ""
    hmac_secret: ""

llm:
  default: "anthropic"
  fallback_chain: ["anthropic", "deepseek", "openai"]
  system_prompt: "You are a helpful AI assistant."
  max_input_length: 10000
  timeout: 60

  anthropic:
    enabled: true
    api_key: ""
    model: "claude-sonnet-4-20250514"
    max_tokens: 4096
    temperature: 0.7

  openai:
    enabled: false
    api_key: ""
    model: "gpt-4o"
    max_tokens: 4096

  gemini:
    enabled: false
    api_key: ""
    model: "gemini-1.5-pro"

  deepseek:
    enabled: false
    api_key: ""
    model: "deepseek-chat"

  glm:
    enabled: false
    api_key: ""
    model: "glm-4"

security:
  encryption_key: ""         # Generate with: magabot genkey

secrets:
  backend: ""               # local | vault (see Secrets Management)

paths:
  data_dir: "~/data/magabot"
  logs_dir: "~/data/magabot/logs"
  memory_dir: "~/data/magabot/memory"
  cache_dir: "~/data/magabot/cache"
  exports_dir: "~/data/magabot/exports"
  downloads_dir: "~/data/magabot/downloads"

skills:
  dir: "~/code/magabot-skills"
  auto_reload: true

storage:
  database: "~/.magabot/data/magabot.db"
  history_retention: 90      # days
  backup:
    enabled: true
    path: "~/.magabot/data/backups"
    keep_count: 10

logging:
  level: "info"              # debug | info | warn | error
  format: "json"
  redact_messages: true

cron:
  enabled: true
  jobs: []

heartbeat:
  enabled: false
  interval: "30m"

memory:
  enabled: true
  max_entries: 1000
  context_limit: 2000

session:
  max_history: 50
  task_timeout: "5m"
```

---

## Secrets Management

Magabot supports two secrets backends to keep sensitive values (API keys, tokens) out of `config.yaml`.

When a secrets backend is configured, the daemon loads secrets at startup and uses them as **fallbacks** -- values set in `config.yaml` or environment variables always take precedence.

### Local File Backend

Stores secrets in a JSON file with `0600` permissions.

**Setup:**

```yaml
# config.yaml
secrets:
  backend: "local"
  local:
    path: "~/.magabot/secrets.json"   # default
```

**File format** (`~/.magabot/secrets.json`):

```json
{
  "magabot/encryption_key": "your-encryption-key",
  "magabot/telegram/bot_token": "123456:ABC-DEF...",
  "magabot/llm/anthropic_api_key": "sk-ant-...",
  "magabot/llm/openai_api_key": "sk-...",
  "magabot/llm/gemini_api_key": "AIza...",
  "magabot/llm/deepseek_api_key": "sk-...",
  "magabot/llm/glm_api_key": "...",
  "magabot/slack/bot_token": "xoxb-...",
  "magabot/slack/app_token": "xapp-...",
  "magabot/tools/brave_api_key": "BSA..."
}
```

Set permissions:

```bash
chmod 600 ~/.magabot/secrets.json
```

### HashiCorp Vault Backend

Uses Vault KV v2 secrets engine.

**Setup:**

```yaml
# config.yaml
secrets:
  backend: "vault"
  vault:
    address: "http://127.0.0.1:8200"  # or VAULT_ADDR env
    mount_path: "secret"               # KV v2 mount point
    secret_path: "magabot"             # base path for secrets
```

The Vault token is read from `VAULT_TOKEN` environment variable or can be set in the vault config:

```bash
export VAULT_TOKEN="hvs.your-vault-token"
```

**Storing secrets in Vault:**

```bash
# Each secret is stored as a KV v2 entry with a "value" key
vault kv put secret/magabot/magabot/llm/anthropic_api_key value="sk-ant-..."
vault kv put secret/magabot/magabot/telegram/bot_token value="123456:ABC..."
vault kv put secret/magabot/magabot/encryption_key value="your-key"
```

### Secret Keys Reference

| Key | Config Field |
|-----|-------------|
| `magabot/encryption_key` | `security.encryption_key` |
| `magabot/telegram/bot_token` | `platforms.telegram.token` |
| `magabot/slack/bot_token` | `platforms.slack.bot_token` |
| `magabot/slack/app_token` | `platforms.slack.app_token` |
| `magabot/llm/anthropic_api_key` | `llm.anthropic.api_key` |
| `magabot/llm/openai_api_key` | `llm.openai.api_key` |
| `magabot/llm/gemini_api_key` | `llm.gemini.api_key` |
| `magabot/llm/glm_api_key` | `llm.glm.api_key` |
| `magabot/llm/deepseek_api_key` | `llm.deepseek.api_key` |
| `magabot/tools/brave_api_key` | tools brave search key |

### Priority Order

1. Config file value (`config.yaml`)
2. Environment variable (`$VAR` expansion in config)
3. Secrets backend (local file or Vault)

If a config field already has a value, the secrets backend is not used for that field.

---

## CLI Commands

### Daemon

```bash
magabot start                # Start daemon (foreground)
magabot stop                 # Stop daemon
magabot restart              # Restart daemon
magabot status               # Show PID, uptime, config, DB size
magabot log                  # Tail log file
```

### Config

```bash
magabot config show          # Show config summary
magabot config edit          # Edit in $EDITOR
magabot config path          # Print config file path
magabot config admin list    # List global admins
magabot config admin add <id>      # Add global admin
magabot config admin remove <id>   # Remove global admin
```

### Setup

```bash
magabot setup                # Full interactive wizard
magabot setup telegram       # Setup Telegram
magabot setup discord        # Setup Discord
magabot setup slack          # Setup Slack
magabot setup whatsapp       # Setup WhatsApp
magabot setup webhook        # Setup webhook endpoint
magabot setup llm            # Setup all LLM providers
magabot setup anthropic      # Setup Anthropic
magabot setup openai         # Setup OpenAI
magabot setup gemini         # Setup Gemini
magabot setup deepseek       # Setup DeepSeek
magabot setup glm            # Setup GLM
magabot setup admin <id>     # Add global admin
magabot setup paths          # Configure directories
magabot setup skills         # Configure skills
```

### Cron

```bash
magabot cron list            # List jobs (-a to include disabled)
magabot cron add             # Add new job (interactive)
magabot cron edit <id>       # Edit a job
magabot cron delete <id>     # Delete a job (-f to skip confirmation)
magabot cron enable <id>     # Enable a job
magabot cron disable <id>    # Disable a job
magabot cron run <id>        # Run a job now
magabot cron show <id>       # Show job details (-j for JSON)
```

### Skills

```bash
magabot skill list           # List installed skills
magabot skill info <name>    # Show skill details
magabot skill create <name>  # Create skill template
magabot skill enable <name>  # Enable a skill
magabot skill disable <name> # Disable a skill
magabot skill reload         # Reload all skills
magabot skill builtin        # List built-in skills
```

### Update

```bash
magabot update check         # Check for new version
magabot update apply         # Download and install update
magabot update rollback      # Restore previous version
```

### Utilities

```bash
magabot genkey               # Generate AES-256 encryption key
magabot reset                # Reset config (keeps platform connections)
magabot uninstall            # Remove magabot and all data
magabot version              # Show version
magabot help                 # Show help
```

---

## Chat Commands

Commands available when messaging the bot on any platform:

| Command | Description |
|---------|-------------|
| `/start` | Welcome message |
| `/help` | Show available commands |
| `/status` | Bot status (provider, message count) |
| `/models` | List available AI models |
| `/providers` | Show LLM providers |

Any other message is sent to the active LLM provider for a response.

### Admin Commands (in chat)

Platform and global admins can manage the bot directly from chat:

```
/config status                    # Show access mode, role, platform info
/config allow user <user_id>      # Add user to allowlist
/config allow chat <chat_id>      # Add chat to allowlist (or "this" for current)
/config remove user <user_id>     # Remove user from allowlist
/config remove chat <chat_id>     # Remove chat from allowlist
/config admin add <user_id>       # Add platform admin
/config admin remove <user_id>    # Remove platform admin
/config admin global add <id>     # Add global admin (global admins only)
/config admin global rm <id>      # Remove global admin (global admins only)
/config mode <allowlist|open>     # Set access mode (global admins only)
/config help                      # Show help
```

### Memory Commands

```
/memory add <text>                # Remember something
/memory <text>                    # Shortcut to remember
/memory search <query>            # Search memories
/memory list [type]               # List memories (types: fact, preference, event, note)
/memory delete <id>               # Delete a memory
/memory clear                     # Clear all memories
/memory stats                     # Show statistics
```

### Task Commands

```
/task spawn <description>         # Run a background task
/task list                        # List active sessions
/task status <id>                 # Show session status
/task cancel <id>                 # Cancel a running task
/task clear                       # Clear completed tasks
```

### Heartbeat Commands

```
/heartbeat status                 # Show check status
/heartbeat run                    # Run all checks now
/heartbeat enable <name>          # Enable a check
/heartbeat disable <name>         # Disable a check
/heartbeat list                   # List configured checks
```

---

## Platforms

| Platform | Method | Group Chat | DMs | Status |
|----------|--------|:----------:|:---:|:------:|
| Telegram | Long Polling | Yes | Yes | Stable |
| Discord | Gateway | Yes | Yes | Stable |
| Slack | Socket Mode | Yes | Yes | Stable |
| WhatsApp | WebSocket (whatsmeow) | Yes | Yes | Beta |
| Webhook | HTTP POST | N/A | N/A | Stable |

---

## LLM Providers

| Provider | Default Model | Auth |
|----------|--------------|------|
| Anthropic | `claude-sonnet-4-20250514` | API key or Claude CLI OAuth |
| OpenAI | `gpt-4o` | API key |
| Gemini | `gemini-1.5-pro` | API key |
| DeepSeek | `deepseek-chat` | API key |
| GLM (Zhipu) | `glm-4` | API key |

### Fallback Chain

If the default provider fails, magabot tries the next provider in the chain:

```yaml
llm:
  default: anthropic
  fallback_chain:
    - anthropic
    - deepseek
    - openai
```

---

## Cron Jobs

Schedule messages to any platform channel.

### Channel Format

```
telegram:<chat_id>           # Telegram chat
whatsapp:<phone>             # WhatsApp (+62...)
slack:#<channel>             # Slack channel
discord:<channel_id>         # Discord channel
webhook:<url>                # Custom webhook URL
```

### Schedule Format

Standard cron syntax or shortcuts:

```
0 9 * * 1-5                 # 9am weekdays
0 */2 * * *                 # Every 2 hours
30 8,12,17 * * *            # 8:30am, 12:30pm, 5:30pm
@hourly                     # Every hour
@daily                      # Daily at midnight
@weekly                     # Every week
```

---

## Update

```bash
# Check for updates
magabot update check

# Download and install (stops bot, verifies SHA-256 checksum, installs, backs up old binary)
magabot update apply

# Rollback to previous version if something goes wrong
magabot update rollback
```

Updates are downloaded from [GitHub Releases](https://github.com/kusandriadi/magabot/releases). The previous binary is saved as a `.backup` file for rollback.

---

## Uninstall

```bash
# Interactive uninstall (stops daemon, removes ~/.magabot config directory)
magabot uninstall
```

To also remove the binary:

```bash
sudo rm /usr/local/bin/magabot
# or if installed to ~/bin:
rm ~/bin/magabot
```

To remove data directory (if using non-default paths):

```bash
rm -rf ~/data/magabot
```

---

## Security

### Encryption

| Layer | Method |
|-------|--------|
| Secrets at rest | AES-256-GCM |
| Chat history | AES-256-GCM (SQLite) |
| Platform sessions | AES-256-GCM |
| Transport | TLS 1.3 (all API calls) |

### Access Control

```
Global Admin    -> Can manage all platforms, add/remove global admins, change access mode
Platform Admin  -> Can manage allowlist for their platform, add/remove platform admins
Allowed User    -> Can use the bot
```

### Security Checklist

```bash
# Generate encryption key
magabot genkey

# Set restrictive file permissions
chmod 600 ~/.magabot/config.yaml
chmod 600 ~/.magabot/secrets.json
chmod 700 ~/.magabot

# Add yourself as admin first
magabot config admin add YOUR_USER_ID

# Start
magabot start
```

### Other Security Features

- No root required
- Config files: `0600`, directories: `0700`
- SQLite `secure_delete = ON`
- Per-user rate limiting
- Input sanitization (control chars stripped)
- Parameterized SQL queries
- Path traversal protection
- Decompression bomb protection (size limits on extraction)

---

## File Structure

```
~/.magabot/
  config.yaml               # Configuration
  magabot.pid               # PID file (when running)
  secrets.json              # Local secrets (if using local backend)

~/data/magabot/              # Default data directory (configurable)
  db/
    magabot.db              # SQLite database (encrypted)
  memory/                   # Per-user memory stores
  logs/
    magabot.log             # Application log
  cache/
  exports/
  downloads/

~/code/magabot-skills/       # Default skills directory (configurable)
  my-skill/
    skill.yaml              # Skill definition (auto-reloaded on change)
```

---

## Docker

```bash
docker run -d \
  --name magabot \
  -v ~/.magabot:/home/magabot/.magabot \
  -v ~/data/magabot:/app/data \
  -p 8080:8080 \
  ghcr.io/kusandriadi/magabot:latest
```

Or build locally:

```bash
docker build -t magabot .
docker run -d --name magabot -v ~/.magabot:/home/magabot/.magabot magabot
```

The container runs as a non-root user (`uid 1000`).

---

## Building from Source

Requires Go 1.22+ and a C compiler (for SQLite CGO).

```bash
git clone https://github.com/kusandriadi/magabot.git
cd magabot

# Build
make build
./bin/magabot setup

# Or install system-wide
make install           # /usr/local/bin (requires sudo)
make install-user      # ~/bin (no sudo)

# Run tests
make test

# Build releases for all platforms
make release
```

### Make Targets

| Target | Description |
|--------|-------------|
| `make build` | Build for current platform |
| `make build-prod` | Production build (stripped) |
| `make install` | Install to `/usr/local/bin` |
| `make install-user` | Install to `~/bin` |
| `make uninstall` | Remove binary |
| `make test` | Run tests |
| `make release` | Cross-compile for all platforms |
| `make clean` | Remove build artifacts |
| `make deps` | Download and tidy dependencies |
| `make genkey` | Generate encryption key |

---

## License

MIT License - see [LICENSE](LICENSE)

# Magabot

[![CI](https://github.com/kusandriadi/magabot/actions/workflows/ci.yml/badge.svg)](https://github.com/kusandriadi/magabot/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

A **security-first**, multi-platform AI chatbot. Single binary, zero runtime dependencies.

---

## Table of Contents

- [Features](#features)
- [Security](#security)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Uninstall](#uninstall)
- [LLM Providers](#llm-providers)
- [Platforms](#platforms)
- [Webhooks](#webhooks)
- [Commands](#commands)
- [Skills](#skills)
- [Configuration](#configuration)
- [Building from Source](#building-from-source)
- [License](#license)

---

## Features

| Feature | Description |
|---------|-------------|
| **Multi-LLM** | Anthropic, OpenAI, Gemini, DeepSeek, GLM, Local (Ollama/vLLM) |
| **Multi-Platform** | Telegram, Discord, Slack, WhatsApp, Webhooks |
| **Fallback Chain** | Automatic failover between LLM providers |
| **Semantic Memory** | Vector-based memory with OpenAI/Voyage/Cohere embeddings |
| **Multi-Agent** | Spawn sub-agents for parallel tasks |
| **Cron Jobs** | Schedule messages with cron, interval, or one-shot |
| **Skills** | Extend with custom YAML-defined skills |
| **Hooks** | Pre/post message hooks for filtering and logging |
| **Zero Dependencies** | Single static binary, no Python/Node/containers |

---

## Security

Security is built into every layer:

| Feature | Description |
|---------|-------------|
| **AES-256-GCM Encryption** | Secrets, chat history, and sessions encrypted at rest |
| **Allowlist Access Control** | Global admins, platform admins, per-user/chat restrictions |
| **Rate Limiting** | Per-IP and per-user rate limiting on webhooks |
| **Brute Force Protection** | IP lockout after failed auth attempts |
| **Replay Attack Prevention** | Timestamp and nonce validation |
| **Input Sanitization** | Control character stripping on all messages |
| **Path Traversal Protection** | All file operations validated |
| **TLS 1.3** | Enforced on all outbound API calls |
| **Secure Delete** | SQLite `secure_delete = ON` |
| **SHA-256 Verification** | Binary updates verified before install |
| **Restrictive Permissions** | Config `0600`, directories `0700` |

---

## Installation

### Linux

```bash
# One-liner (recommended)
curl -sL https://raw.githubusercontent.com/kusandriadi/magabot/master/install.sh | bash

# Manual
wget https://github.com/kusandriadi/magabot/releases/latest/download/magabot_linux_amd64.tar.gz
tar -xzf magabot_linux_amd64.tar.gz
sudo mv magabot /usr/local/bin/
```

### macOS

```bash
# One-liner
curl -sL https://raw.githubusercontent.com/kusandriadi/magabot/master/install.sh | bash

# Manual (Apple Silicon)
wget https://github.com/kusandriadi/magabot/releases/latest/download/magabot_darwin_arm64.tar.gz
tar -xzf magabot_darwin_arm64.tar.gz
sudo mv magabot /usr/local/bin/

# Manual (Intel)
wget https://github.com/kusandriadi/magabot/releases/latest/download/magabot_darwin_amd64.tar.gz
tar -xzf magabot_darwin_amd64.tar.gz
sudo mv magabot /usr/local/bin/
```

### Windows

```powershell
# Download from releases
Invoke-WebRequest -Uri "https://github.com/kusandriadi/magabot/releases/latest/download/magabot_windows_amd64.zip" -OutFile "magabot.zip"
Expand-Archive -Path "magabot.zip" -DestinationPath "."

# Add to PATH or move to a directory in PATH
Move-Item magabot.exe C:\Windows\System32\
```

### User-local Install (no sudo)

```bash
mkdir -p ~/bin
mv magabot ~/bin/
echo 'export PATH="$HOME/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc
```

---

## Quick Start

```bash
# Interactive setup wizard
magabot setup

# Or use quick init (auto-detects env vars)
magabot init

# Start the bot
magabot start

# Check status
magabot status
```

---

## Uninstall

### Linux / macOS

```bash
magabot uninstall              # Stop daemon, remove config
sudo rm /usr/local/bin/magabot # Remove binary
rm -rf ~/data/magabot          # Remove data (optional)
```

### Windows

```powershell
magabot.exe uninstall
Remove-Item C:\Windows\System32\magabot.exe
Remove-Item -Recurse -Force "$env:USERPROFILE\.magabot"
```

---

## LLM Providers

| Provider | Model | Auth |
|----------|-------|------|
| **Anthropic** | claude-sonnet-4-20250514 | API key |
| **OpenAI** | gpt-4o | API key |
| **Gemini** | gemini-2.0-flash | API key |
| **DeepSeek** | deepseek-chat | API key |
| **GLM** | glm-4 | API key |
| **Local** | llama3 (Ollama/vLLM) | Optional |

### Setup

```bash
magabot setup llm
```

### Fallback Chain

If the primary provider fails, Magabot automatically tries the next:

```yaml
llm:
  main: anthropic
  fallback_chain: [anthropic, deepseek, openai]
```

---

## Platforms

| Platform | Method | Groups | DMs | Status |
|----------|--------|:------:|:---:|:------:|
| **Telegram** | Long Polling / Webhook | ✅ | ✅ | Stable |
| **Discord** | Gateway | ✅ | ✅ | Stable |
| **Slack** | Socket Mode / Events API | ✅ | ✅ | Stable |
| **WhatsApp** | WebSocket (whatsmeow) | ✅ | ✅ | Stable |
| **Webhook** | HTTP POST | - | - | Stable |

> **Note on WhatsApp:** Uses [whatsmeow](https://github.com/tulir/whatsmeow) (unofficial multi-device API). Requires QR code scan on first setup. WhatsApp may rate-limit or block automated usage - use responsibly.

### Setup

```bash
magabot setup platform
```

---

## Webhooks

Receive messages from external services (GitHub, Grafana, CI/CD).

### Setup

```bash
magabot setup webhook
```

### Security Features

| Feature | Description |
|---------|-------------|
| **Auth Methods** | Bearer token, Basic auth, HMAC signature |
| **IP Allowlist** | Restrict by IP or CIDR |
| **User Allowlist** | Restrict by user ID |
| **Rate Limiting** | Per-IP and per-user limits |
| **Brute Force Protection** | Lockout after N failures |
| **Timestamp Validation** | Reject old/future requests (±5 min) |
| **Nonce Validation** | Prevent replay attacks |
| **Request ID Tracking** | Audit trail for every request |

### Example Config

```yaml
webhook:
  enabled: true
  port: 8080
  auth_method: hmac
  hmac_users:
    "secret-github": "github:myrepo"
    "secret-grafana": "grafana:prod"
  rate_limit_per_ip: 60
  rate_limit_per_user: 30
  max_auth_failures: 5
  require_timestamp: true
  require_nonce: true
```

---

## Commands

### CLI Commands

| Command | Description |
|---------|-------------|
| `magabot setup` | Full interactive wizard |
| `magabot setup llm` | Configure LLM providers |
| `magabot setup platform` | Configure chat platforms |
| `magabot setup webhook` | Configure webhooks |
| `magabot start` | Start daemon |
| `magabot stop` | Stop daemon |
| `magabot restart` | Restart daemon |
| `magabot status` | Show status |
| `magabot log` | Tail log file |
| `magabot config show` | Show config summary |
| `magabot config edit` | Edit config in $EDITOR |
| `magabot cron list` | List cron jobs |
| `magabot cron add` | Add cron job |
| `magabot skill list` | List installed skills |
| `magabot update check` | Check for updates |
| `magabot update apply` | Apply update |
| `magabot uninstall` | Uninstall magabot |
| `magabot version` | Show version |
| `magabot help` | Show help |

### Chat Commands

| Command | Description |
|---------|-------------|
| `/help` | Show commands |
| `/status` | Bot status |
| `/models` | List AI models |
| `/memory add <text>` | Remember something |
| `/memory search <query>` | Search memories |
| `/config allow user <id>` | Add to allowlist (admin) |
| `/task spawn <desc>` | Run background task |

---

## Skills

Skills extend Magabot with custom functionality.

### List Skills

```bash
magabot skill list
magabot skill builtin
```

### Install Skill

```bash
# Clone to skills directory
git clone https://github.com/example/my-skill.git ~/code/magabot-skills/my-skill

# Enable
magabot skill enable my-skill

# Reload
magabot skill reload
```

### Uninstall Skill

```bash
# Disable
magabot skill disable my-skill

# Remove
rm -rf ~/code/magabot-skills/my-skill
```

### Create Skill

```bash
magabot skill create my-skill
```

Creates a template in `~/code/magabot-skills/my-skill/skill.yaml`:

```yaml
name: my-skill
version: 1.0.0
description: My custom skill
commands:
  - name: hello
    description: Say hello
    action: echo "Hello, world!"
```

---

## Configuration

Config file: `~/.magabot/config.yaml`

```bash
magabot config show    # Show summary
magabot config edit    # Edit in $EDITOR
magabot config path    # Print path
```

### Minimal Config

```yaml
llm:
  main: anthropic
  anthropic:
    enabled: true
    api_key: "${ANTHROPIC_API_KEY}"

platforms:
  telegram:
    enabled: true
    token: "${TELEGRAM_BOT_TOKEN}"

access:
  global_admins: ["YOUR_USER_ID"]
```

### Environment Variables

Config supports `$VAR` and `${VAR}` expansion:

```yaml
llm:
  anthropic:
    api_key: "${ANTHROPIC_API_KEY}"
```

---

## Building from Source

Requires Go 1.22+ and a C compiler (for SQLite CGO).

```bash
git clone https://github.com/kusandriadi/magabot.git
cd magabot

# Build
make build
./bin/magabot setup

# Install system-wide
make install           # /usr/local/bin (requires sudo)
make install-user      # ~/bin (no sudo)

# Run tests
make test

# Build for all platforms
make release

# Clean
make clean
```

### Cross-Platform Build

```bash
# Linux
GOOS=linux GOARCH=amd64 make build

# macOS
GOOS=darwin GOARCH=arm64 make build

# Windows
GOOS=windows GOARCH=amd64 make build
```

---

## License

MIT License - see [LICENSE](LICENSE)

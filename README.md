# Magabot

[![CI](https://github.com/kusandriadi/magabot/actions/workflows/ci.yml/badge.svg)](https://github.com/kusandriadi/magabot/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Magabot is a **privacy-first, self-hosted AI chatbot** that connects multiple LLM providers to your favorite messaging platforms. It ships as a single static binary with zero runtime dependencies — no Python, Node.js, Docker, or cloud accounts required. All data stays on your machine, encrypted at rest with AES-256-GCM.

---

## Table of Contents

- [Features](#features)
- [Security](#security)
- [LLM Providers](#llm-providers)
- [Platforms](#platforms)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Uninstall](#uninstall)
- [CLI Commands](#cli-commands)
- [Chat Commands](#chat-commands)
- [Skills](#skills)
- [Configuration](#configuration)
- [Building from Source](#building-from-source)
- [License](#license)

---

## Features

| Feature | Description |
|---------|-------------|
| **Multi-LLM** | Anthropic, OpenAI, Gemini, DeepSeek, GLM, Local (Ollama/vLLM) |
| **Multi-Platform** | Telegram, Slack, WhatsApp, Webhooks |
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
| **Response Size Limits** | All HTTP response reads capped to prevent OOM |

---

## LLM Providers

| Provider | Default Model | Auth |
|----------|---------------|------|
| **Anthropic** | claude-sonnet-4-20250514 | `ANTHROPIC_API_KEY` |
| **OpenAI** | gpt-4o | `OPENAI_API_KEY` |
| **Gemini** | gemini-2.0-flash | `GEMINI_API_KEY` or `GOOGLE_API_KEY` |
| **DeepSeek** | deepseek-chat | `DEEPSEEK_API_KEY` |
| **GLM** | glm-4.7 | `ZAI_API_KEY` or `GLM_API_KEY` |
| **Local** | llama3 (Ollama/vLLM) | Optional (`LOCAL_LLM_API_KEY`) |

### Fallback Chain

If the primary provider fails, Magabot automatically tries the next available provider:

```yaml
llm:
  main: anthropic
  fallback_chain: [anthropic, deepseek, openai]
```

---

## Platforms

| Platform | Method | Groups | DMs | Status |
|----------|--------|:------:|:---:|:------:|
| **Telegram** | Long Polling / Webhook | yes | yes | Stable |
| **Slack** | Socket Mode / Events API | yes | yes | Stable |
| **WhatsApp** | WebSocket (whatsmeow) | yes | yes | Stable |
| **Discord** | Bot API / Webhook | - | - | Notifications Only |
| **Webhook** | HTTP POST | - | - | Stable |

> **Note on WhatsApp:** Uses [whatsmeow](https://github.com/tulir/whatsmeow) (unofficial multi-device API). Requires QR code scan on first setup. WhatsApp may rate-limit or block automated usage — use responsibly.

> **Note on Discord:** Discord integration is currently limited to sending notifications via cron jobs (bot token or webhook URL). It is not a full interactive chat platform.

### Webhook Security

| Feature | Description |
|---------|-------------|
| **Auth Methods** | Bearer token, Basic auth, HMAC signature |
| **IP Allowlist** | Restrict by IP or CIDR |
| **User Allowlist** | Restrict by user ID |
| **Rate Limiting** | Per-IP and per-user limits |
| **Brute Force Protection** | Lockout after N failures |
| **Timestamp Validation** | Reject old/future requests |
| **Nonce Validation** | Prevent replay attacks |
| **Request ID Tracking** | Audit trail for every request |

---

## Installation

### Linux

```bash
# One-liner (recommended)
curl -sL https://raw.githubusercontent.com/kusandriadi/magabot/master/install.sh | bash

# Manual
curl -sLO https://github.com/kusandriadi/magabot/releases/latest/download/magabot_linux_amd64.tar.gz
tar -xzf magabot_linux_amd64.tar.gz
sudo mv magabot /usr/local/bin/
```

### macOS

```bash
# One-liner
curl -sL https://raw.githubusercontent.com/kusandriadi/magabot/master/install.sh | bash

# Manual (Apple Silicon)
curl -sLO https://github.com/kusandriadi/magabot/releases/latest/download/magabot_darwin_arm64.tar.gz
tar -xzf magabot_darwin_arm64.tar.gz
sudo mv magabot /usr/local/bin/

# Manual (Intel)
curl -sLO https://github.com/kusandriadi/magabot/releases/latest/download/magabot_darwin_amd64.tar.gz
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

### Linux

```bash
magabot uninstall              # Stop daemon, remove config
sudo rm /usr/local/bin/magabot # Remove binary
rm -rf ~/data/magabot          # Remove data (optional)
```

### macOS

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

## CLI Commands

| Command | Description |
|---------|-------------|
| `magabot setup` | Full interactive setup wizard |
| `magabot setup llm` | Configure LLM providers |
| `magabot setup platform` | Configure chat platforms |
| `magabot setup webhook` | Configure webhooks |
| `magabot init` | Quick init (auto-detects env vars) |
| `magabot start` | Start daemon |
| `magabot stop` | Stop daemon |
| `magabot restart` | Restart daemon |
| `magabot status` | Show status |
| `magabot log` | Tail log file |
| `magabot genkey` | Generate encryption key |
| `magabot reset` | Reset configuration |
| `magabot config show` | Show config summary |
| `magabot config edit` | Edit config in `$EDITOR` |
| `magabot config path` | Print config file path |
| `magabot config admin` | Manage admin users |
| `magabot cron list` | List cron jobs |
| `magabot cron add` | Add cron job |
| `magabot cron edit` | Edit cron job |
| `magabot cron delete` | Delete cron job |
| `magabot cron enable` | Enable cron job |
| `magabot cron disable` | Disable cron job |
| `magabot cron run` | Run cron job manually |
| `magabot cron show` | Show cron job details |
| `magabot skill list` | List installed skills |
| `magabot skill info` | Show skill details |
| `magabot skill create` | Create new skill template |
| `magabot skill enable` | Enable a skill |
| `magabot skill disable` | Disable a skill |
| `magabot skill reload` | Reload all skills |
| `magabot skill builtin` | List built-in skills |
| `magabot update check` | Check for updates |
| `magabot update apply` | Apply update |
| `magabot update rollback` | Roll back to previous version |
| `magabot uninstall` | Uninstall magabot |
| `magabot version` | Show version |
| `magabot help` | Show help |

---

## Chat Commands

Send these in any connected platform:

| Command | Description |
|---------|-------------|
| `/help` | Show available commands |
| `/status` | Bot status and uptime |
| `/models` | List available AI models |
| `/providers` | Show LLM provider status |
| `/config` | Show or manage configuration (admin) |
| `/memory add <text>` | Remember something |
| `/memory search <query>` | Search memories |
| `/task spawn <desc>` | Run a background task |

---

## Skills

Skills extend Magabot with custom YAML-defined functionality.

### Install a Skill

```bash
# Clone into skills directory
git clone https://github.com/example/my-skill.git ~/code/magabot-skills/my-skill

# Enable and reload
magabot skill enable my-skill
magabot skill reload
```

### Uninstall a Skill

```bash
magabot skill disable my-skill
rm -rf ~/code/magabot-skills/my-skill
```

### Create a Skill

```bash
magabot skill create my-skill
```

This generates a template at `~/code/magabot-skills/my-skill/skill.yaml`:

```yaml
name: my-skill
description: Description of your skill
version: 1.0.0
triggers:
  commands: ["/my-skill"]
  keywords: ["my-skill"]
actions:
  type: prompt
  prompt: "You are now in my-skill mode. Help the user with..."
system_prompt: "Additional context for the LLM when this skill is active."
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

Config supports `$VAR` and `${VAR}` expansion for environment variables.

---

## Building from Source

Requires Go 1.24+ and a C compiler (for SQLite CGO).

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

> **Note:** `make release` uses `CGO_ENABLED=0` for static binaries. This excludes SQLite support. For full functionality, build with CGO enabled and a C compiler.

---

## License

MIT License - see [LICENSE](LICENSE)

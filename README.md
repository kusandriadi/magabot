# Magabot

[![CI](https://github.com/kusandriadi/magabot/actions/workflows/ci.yml/badge.svg)](https://github.com/kusandriadi/magabot/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**Magabot** is a privacy-first, self-hosted AI chatbot that connects multiple LLM providers to messaging platforms. Single static binary, zero runtime dependencies, encrypted at rest with AES-256-GCM.

---

## Key Features

- **Multi-LLM** — Anthropic, OpenAI, GLM, Kimi, MiniMax, Local (Ollama/vLLM)
- **Multi-Platform** — Telegram, Slack, WhatsApp, Webhooks
- **Multi-Modal** — Vision (image analysis), voice messages, document processing, image generation
- **Privacy-First** — All data encrypted at rest, runs on your hardware
- **Zero Dependencies** — Single binary, no Docker/Python/Node.js required
- **Agent Sessions** — Spawn coding agents (Claude, Codex) directly from chat
- **Semantic Memory** — Vector-based memory with OpenAI/Voyage/Cohere embeddings
- **Cron Jobs** — Schedule messages with cron, interval, or one-shot timing
- **Skills System** — Extend with custom YAML-defined skills
- **Live Management** — Update, restart, and configure the bot from chat
- **Secure** — Input sanitization, rate limiting, path traversal protection, audit logging

---

## LLM Providers

| Provider | Default Model | Environment Variable |
|----------|---------------|---------------------|
| Anthropic | claude-sonnet-4-6 | `ANTHROPIC_API_KEY` |
| OpenAI | gpt-5 | `OPENAI_API_KEY` |
| GLM | glm-4.7 | `GLM_API_KEY` or `ZAI_API_KEY` |
| Kimi | moonshot-v1 | `KIMI_API_KEY` |
| MiniMax | minimax-pro | `MINIMAX_API_KEY` |
| Local | llama3 | `LOCAL_LLM_BASE_URL` |

Supports automatic failover between providers and custom base URLs. Anthropic also supports Claude CLI mode for Pro/Max subscriptions.

---

## Platforms

- **Telegram** — Long polling or webhook mode (groups & DMs)
- **Slack** — Socket mode or Events API (groups & DMs)
- **WhatsApp** — Multi-device WebSocket API via [whatsmeow](https://github.com/tulir/whatsmeow) (requires QR scan)
- **Webhook** — HTTP POST endpoint with Bearer/HMAC/Basic auth
- **Discord** — *(planned)*

---

## Installation

**Linux (recommended):**
```bash
curl -sL https://raw.githubusercontent.com/kusandriadi/magabot/master/install.sh | bash
```

**Manual install:**
```bash
curl -sLO https://github.com/kusandriadi/magabot/releases/latest/download/magabot_linux_amd64.tar.gz
tar -xzf magabot_linux_amd64.tar.gz
sudo mv magabot /usr/local/bin/
```

**User-local (no sudo):**
```bash
mkdir -p ~/bin && mv magabot ~/bin/
echo 'export PATH="$HOME/bin:$PATH"' >> ~/.bashrc && source ~/.bashrc
```

See [releases](https://github.com/kusandriadi/magabot/releases) for macOS and Windows binaries.

---

## Quick Start

```bash
# Interactive setup
magabot setup

# Start the bot
magabot start

# Check status
magabot status

# View logs
magabot log

# Restart / stop
magabot restart
magabot stop
```

---

## Configuration

Config file: `~/.magabot/config.yaml`

**Minimal example:**
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

security:
  allowed_users:
    telegram: ["YOUR_USER_ID"]
```

Environment variables are expanded with `$VAR` or `${VAR}` syntax.

**CLI commands:**
```bash
magabot config show     # View config summary
magabot config edit     # Edit in $EDITOR
magabot config path     # Print config file path
magabot genkey          # Generate encryption key
magabot update check    # Check for updates
magabot update apply    # Apply available update
magabot cron list       # List scheduled jobs
magabot skill list      # List installed skills
```

---

## Chat Commands

Send these in any connected platform:

| Command | Description |
|---------|-------------|
| `/start` | Welcome message and feature overview |
| `/help` | Show available commands |
| `/status` | Bot status, provider info, and user stats |
| `/model [name]` | Show current model or switch to another |
| `/effort [level]` | Set effort level (low/medium/high/max) |
| `/prompt [text]` | Set custom system prompt |
| `/fallback [model]` | Set fallback model |
| `/budget [amount]` | Set budget limit per request |
| `/providers` | List active LLM providers |
| `/clear` | Clear conversation history |
| `/memory` | Memory management (add/search/list) |
| `/task` | Background task management |

**Admin-only:**

| Command | Description |
|---------|-------------|
| `/config` | Manage bot configuration and access control |
| `/restart` | Restart the bot (with confirmation) |
| `/update` | Check and apply updates (with confirmation) |

**Agent Sessions (admin-only):**

| Command | Description |
|---------|-------------|
| `:new [agent] <dir>` | Start a coding agent (claude/codex) |
| `:status` | Show agent session info |
| `:quit` | Close agent session |

---

## Building from Source

Requires Go 1.26+ and a C compiler (for SQLite).

```bash
git clone https://github.com/kusandriadi/magabot.git
cd magabot
make build
./bin/magabot setup
```

Install:
```bash
make install        # System-wide (/usr/local/bin, requires sudo)
make install-user   # User-local (~/bin, no sudo)
```

Run tests:
```bash
make test
```

---

## Security

Magabot implements defense-in-depth security:

- **Encryption** — AES-256-GCM for secrets, chat history, and sessions
- **Access Control** — Allowlist-based auth with platform-specific admin controls
- **Input Validation** — All user input sanitized, control characters stripped
- **Rate Limiting** — Per-user and per-IP rate limiting with DoS protection
- **Path Traversal Protection** — All file operations validated against allowed directories
- **SSRF Protection** — Base URL validation for LLM providers
- **Audit Logging** — Security events logged to `~/.magabot/logs/security.log`
- **Safe Defaults** — Config files `0600`, directories `0700`

For vulnerability reports, see [SECURITY.md](SECURITY.md).

---

## Skills

Skills extend Magabot with custom functionality via YAML definitions.

**Built-in skills:** weather, translate, summarize, code, math, search

**Create a custom skill:**
```bash
magabot skill create my-skill    # Creates template in ~/.magabot/skills/
magabot skill enable my-skill
magabot skill disable my-skill
magabot skill builtin            # List built-in skills
```

---

## Cron Jobs

Schedule recurring messages or tasks:

```bash
magabot cron add "daily-report" --schedule "0 9 * * *" --message "Good morning!"
magabot cron list
magabot cron enable daily-report
magabot cron disable daily-report
```

Supports cron expressions, intervals, and one-shot scheduling.

---

## License

MIT License - see [LICENSE](LICENSE)

---

## Links

- **Documentation** — [docs/](docs/)
- **Issues** — [GitHub Issues](https://github.com/kusandriadi/magabot/issues)
- **Releases** — [GitHub Releases](https://github.com/kusandriadi/magabot/releases)

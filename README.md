# Magabot

[![CI](https://github.com/kusandriadi/magabot/actions/workflows/ci.yml/badge.svg)](https://github.com/kusandriadi/magabot/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**Magabot** is a privacy-first, self-hosted AI chatbot that connects multiple LLM providers to messaging platforms. Single static binary, zero runtime dependencies, encrypted at rest with AES-256-GCM.

---

## Key Features

- **Multi-LLM** — Anthropic, OpenAI, Gemini, DeepSeek, GLM, Local (Ollama/vLLM)
- **Multi-Platform** — Telegram, Slack, WhatsApp, Webhooks
- **Privacy-First** — All data encrypted at rest, runs on your hardware
- **Zero Dependencies** — Single binary, no Docker/Python/Node.js required
- **Semantic Memory** — Vector-based memory with OpenAI/Voyage/Cohere embeddings
- **Multi-Agent** — Spawn sub-agents for parallel tasks
- **Cron Jobs** — Schedule messages with cron, interval, or one-shot timing
- **Skills System** — Extend with custom YAML-defined skills
- **Secure** — Input sanitization, rate limiting, path traversal protection, audit logging

---

## LLM Providers

| Provider | Default Model | Environment Variable |
|----------|---------------|---------------------|
| Anthropic | claude-sonnet-4-20250514 | `ANTHROPIC_API_KEY` |
| OpenAI | gpt-4o | `OPENAI_API_KEY` |
| Gemini | gemini-2.0-flash | `GEMINI_API_KEY` |
| DeepSeek | deepseek-chat | `DEEPSEEK_API_KEY` |
| GLM | glm-4.7 | `GLM_API_KEY` or `ZAI_API_KEY` |
| Local | llama3 | `LOCAL_LLM_BASE_URL` |

Supports automatic failover between providers and custom base URLs.

---

## Platforms

- **Telegram** — Long polling or webhook mode (groups & DMs)
- **Slack** — Socket mode or Events API (groups & DMs)
- **WhatsApp** — Multi-device WebSocket API via [whatsmeow](https://github.com/tulir/whatsmeow) (requires QR scan)
- **Webhook** — HTTP POST endpoint with Bearer/HMAC/Basic auth
- **Discord** — Notifications only (via bot token or webhook URL)

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

**Common commands:**
```bash
magabot config show     # View config summary
magabot config edit     # Edit in $EDITOR
magabot config path     # Print config file path
magabot genkey          # Generate encryption key
```

---

## Chat Commands

Send these in any connected platform:

- `/help` — Show available commands
- `/status` — Bot status and uptime
- `/models` — List available AI models
- `/memory add <text>` — Remember something
- `/memory search <query>` — Search memories
- `/task spawn <desc>` — Run a background task

Admin-only:
- `/config` — Manage configuration

---

## Building from Source

Requires Go 1.24+ and a C compiler (for SQLite).

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

**Create a skill:**
```bash
magabot skill create my-skill
```

**Enable/disable:**
```bash
magabot skill enable my-skill
magabot skill disable my-skill
```

See `magabot skill help` for full documentation.

---

## License

MIT License - see [LICENSE](LICENSE)

---

## Links

- **Documentation** — [docs/](docs/)
- **Issues** — [GitHub Issues](https://github.com/kusandriadi/magabot/issues)
- **Releases** — [GitHub Releases](https://github.com/kusandriadi/magabot/releases)
- **Discord** — *(coming soon)*

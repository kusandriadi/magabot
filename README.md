# 🤖 Magabot

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Security](https://img.shields.io/badge/Security-AES--256--GCM-green.svg)](#-security)
[![Binary Size](https://img.shields.io/badge/Binary-14MB-orange.svg)](#-lightweight)

**Magabot** is a **lightweight**, **security-first** multi-platform chatbot with LLM integration.

Built in pure Go. Single 14MB binary. Zero runtime dependencies. No root required.

---

## ✨ What is Magabot?

Magabot is a self-hosted AI chatbot that:

- 🔌 **Connects to 6+ platforms** (Telegram, Discord, Slack, WhatsApp, Lark, Webhook)
- 🤖 **Supports 5 LLM providers** (Anthropic, OpenAI, Gemini, DeepSeek, GLM)
- 🔐 **Prioritizes security** with encryption, allowlists, and audit logging
- ⚡ **Runs anywhere** - VPS, Raspberry Pi, laptop, Docker
- 🧠 **Remembers context** with built-in memory/RAG
- ⏰ **Works proactively** with heartbeat and cron jobs
- 📦 **Updates itself** with one command

### Who is it for?

- **Personal assistant** - Your own AI that knows your preferences
- **Team bot** - Shared AI for your Discord/Slack workspace
- **Trading alerts** - Scheduled notifications with auto-trading support
- **Home automation** - Control your smart home via chat
- **Self-hosters** - Full control, no cloud dependencies

---

## 🪶 Lightweight

| Metric | Magabot | Typical Python Bot | Node.js Bot |
|--------|---------|-------------------|-------------|
| Binary Size | **14 MB** | ~200 MB + Python | ~150 MB + Node |
| Memory Usage | **~20 MB** | ~100 MB | ~80 MB |
| Startup Time | **<1 sec** | 3-5 sec | 2-3 sec |
| Dependencies | **0 runtime** | pip packages | npm packages |
| Installation | Single binary | Python + pip | Node + npm |

### Why Go?

```
┌─────────────────────────────────────────────────────────┐
│                    MAGABOT BINARY                       │
│                      (14 MB)                            │
│                                                         │
│  ┌───────────┐ ┌───────────┐ ┌───────────┐            │
│  │ Platforms │ │ LLM       │ │ Tools     │            │
│  │ Telegram  │ │ Anthropic │ │ Browser   │            │
│  │ Discord   │ │ OpenAI    │ │ Search    │            │
│  │ Slack     │ │ Gemini    │ │ Maps      │            │
│  │ WhatsApp  │ │ DeepSeek  │ │ Weather   │            │
│  │ Lark      │ │ GLM       │ │ Scraper   │            │
│  └───────────┘ └───────────┘ └───────────┘            │
│                                                         │
│  ┌───────────┐ ┌───────────┐ ┌───────────┐            │
│  │ Security  │ │ Storage   │ │ Features  │            │
│  │ AES-256   │ │ SQLite    │ │ Memory    │            │
│  │ Allowlist │ │ Encrypted │ │ Heartbeat │            │
│  │ Audit     │ │ Backup    │ │ Sessions  │            │
│  └───────────┘ └───────────┘ └───────────┘            │
│                                                         │
│           Everything in ONE static binary               │
└─────────────────────────────────────────────────────────┘
```

---

## 🔐 Security

Security is not an afterthought — it's the foundation.

### Encryption

| Layer | Method | Description |
|-------|--------|-------------|
| **Secrets** | AES-256-GCM | API keys, tokens encrypted at rest |
| **Messages** | AES-256-GCM | Chat history encrypted in SQLite |
| **Sessions** | AES-256-GCM | Platform sessions encrypted |
| **Transport** | TLS 1.3 | All API calls over HTTPS |

### Access Control

```yaml
# config.yaml
access:
  mode: allowlist           # allowlist | denylist | open
  global_admins:            # Can manage ALL platforms
    - "287676843"

platforms:
  telegram:
    admins: ["287676843"]   # Can manage THIS platform
    allowed_users: ["287676843", "123456789"]
    allowed_chats: ["-100123456"]
    allow_groups: true
    allow_dms: true
```

**Hierarchy:**
```
🌍 Global Admin
    └── Can manage all platforms
    └── Can add/remove global admins
    └── Can change access mode

👤 Platform Admin  
    └── Can manage allowlist for their platform
    └── Can add/remove platform admins
    └── Must be in allowlist first

✅ Allowed User
    └── Can use the bot
    └── Cannot change config
```

### Security Features

| Feature | Description |
|---------|-------------|
| **No root required** | Runs as normal user |
| **File permissions** | Config 0600, dirs 0700 |
| **Secure delete** | SQLite `secure_delete = ON` |
| **Rate limiting** | Per-user request limits |
| **Audit logging** | All actions logged with hashed IDs |
| **Input sanitization** | Control chars stripped |
| **SQL injection safe** | Parameterized queries only |
| **Path traversal safe** | Filenames sanitized |
| **No hardcoded secrets** | All secrets from config/env |

### Security Checklist

```bash
# Generate encryption key
magabot genkey

# Set restrictive permissions
chmod 600 ~/.magabot/config.yaml
chmod 700 ~/.magabot/data

# Add yourself as admin FIRST
magabot config admin add YOUR_USER_ID

# Then start
magabot start
```

---

## 🔌 Platforms

| Platform | Method | Group Chat | DMs | Status |
|----------|--------|:----------:|:---:|:------:|
| Telegram | Long Polling | ✅ | ✅ | ✅ Ready |
| Discord | Gateway | ✅ | ✅ | ✅ Ready |
| Slack | Socket Mode | ✅ | ✅ | ✅ Ready |
| Lark/Feishu | Webhook + API | ✅ | ✅ | ✅ Ready |
| WhatsApp | WebSocket | ✅ | ✅ | 🚧 Beta |
| Webhook | HTTP POST | N/A | N/A | ✅ Ready |

---

## 🤖 LLM Providers

| Provider | Models | Streaming | Status |
|----------|--------|:---------:|:------:|
| **Anthropic** | Claude 3.5, Claude 4 | ✅ | ✅ |
| **OpenAI** | GPT-4o, GPT-4 | ✅ | ✅ |
| **Google** | Gemini 1.5 Pro/Flash | ✅ | ✅ |
| **DeepSeek** | deepseek-chat, deepseek-coder, R1 | ✅ | ✅ |
| **Zhipu** | GLM-4, GLM-4V | ✅ | ✅ |

### Fallback Chain

```yaml
llm:
  default: anthropic
  fallback_chain:
    - anthropic
    - deepseek
    - openai
```

If Anthropic fails → try DeepSeek → try OpenAI

---

## 🛠️ Built-in Tools

| Tool | Provider | API Key? | Description |
|------|----------|:--------:|-------------|
| **Search** | DuckDuckGo | ❌ | Web search via scraping |
| **Search** | Brave (optional) | ✅ | Better results |
| **Maps** | Nominatim/OSM | ❌ | Geocoding, POI search |
| **Weather** | wttr.in | ❌ | Current + 3-day forecast |
| **Scraper** | Colly | ❌ | Static page scraping |
| **Browser** | Rod/Chromium | ❌ | Full JS rendering |

All tools are **100% free** (no API keys required for basics).

---

## 📦 Installation

### One-liner (Linux/macOS)

```bash
curl -sL https://raw.githubusercontent.com/kusa/magabot/main/install.sh | bash
```

### Manual

```bash
# Download latest release
wget https://github.com/kusa/magabot/releases/latest/download/magabot_linux_amd64.tar.gz

# Extract and install
tar -xzf magabot_linux_amd64.tar.gz
sudo mv magabot /usr/local/bin/

# Setup
magabot setup
```

### From Source

```bash
git clone https://github.com/kusa/magabot.git
cd magabot
make build
./bin/magabot setup
```

### Docker

```bash
docker run -d \
  --name magabot \
  -v ~/.magabot:/root/.magabot \
  ghcr.io/kusa/magabot:latest
```

---

## 🚀 Quick Start

```bash
# 1. Setup (interactive wizard)
magabot setup

# 2. Add yourself as admin
magabot config admin add YOUR_TELEGRAM_ID

# 3. Start
magabot start

# 4. Check status
magabot status
```

---

## 📖 Commands

### CLI Commands

```bash
magabot start           # Start daemon
magabot stop            # Stop daemon  
magabot restart         # Restart daemon
magabot status          # Show status
magabot log             # View logs (tail -f)
magabot setup           # First-time setup
magabot config show     # Show config
magabot config edit     # Edit config.yaml
magabot cron list       # List cron jobs
magabot update check    # Check for updates
magabot update apply    # Apply update
```

### Chat Commands (for admins)

```
/config status          Show config status
/config allow user ID   Allow a user
/config allow chat ID   Allow a group
/config admin add ID    Add platform admin
/memory add TEXT        Remember something
/memory search QUERY    Find memories
/task spawn TASK        Run background task
/heartbeat status       Show heartbeat status
```

---

## 📊 Features Summary

| Feature | Description |
|---------|-------------|
| 🔌 6 Platforms | Telegram, Discord, Slack, Lark, WhatsApp, Webhook |
| 🤖 5 LLM Providers | Anthropic, OpenAI, Gemini, DeepSeek, GLM |
| 🛠️ 5 Tools | Search, Maps, Weather, Scraper, Browser |
| 🔐 Security | AES-256-GCM, allowlist, audit, rate limit |
| 🧠 Memory/RAG | Remember context across sessions |
| 💓 Heartbeat | Proactive periodic checks |
| 🔄 Multi-Session | Background tasks, parallel processing |
| ⏰ Cron Jobs | Scheduled notifications, multi-channel |
| 📦 Self-Update | One-command updates with rollback |
| 🐳 Docker | Container-ready |

---

## 📁 File Structure

```
~/.magabot/
├── config.yaml          # All configuration
├── magabot.pid          # PID file
├── data/
│   ├── magabot.db       # SQLite (encrypted)
│   └── memory/          # Per-user memories
├── logs/
│   └── magabot.log
└── skills/              # Custom skills
```

---

## 🔄 Updates

```bash
# Check for updates
magabot update check

# Apply update
magabot update apply

# Rollback if issues
magabot update rollback
```

---

## 📄 License

MIT License - see [LICENSE](LICENSE)

---

## 🙏 Acknowledgments

Built with:
- [discordgo](https://github.com/bwmarrin/discordgo) - Discord library
- [telegram-bot-api](https://github.com/go-telegram-bot-api/telegram-bot-api) - Telegram library
- [rod](https://github.com/go-rod/rod) - Browser automation
- [colly](https://github.com/gocolly/colly) - Web scraping

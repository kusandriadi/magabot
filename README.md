# Magabot

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

A security-first, multi-platform chatbot with multi-LLM integration. Single binary, zero runtime dependencies.

---

## Table of Contents

- [What is Magabot?](#what-is-magabot)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Uninstall](#uninstall)
- [Supported LLM Providers](#supported-llm-providers)
- [Supported Platforms](#supported-platforms)
- [Hooks](#hooks)
- [Setup Guides](#setup-guides)
  - [Vault Setup](#vault-setup)
  - [LLM Setup](#llm-setup)
  - [Platform Setup](#platform-setup)
  - [Hooks Setup](#hooks-setup)
  - [Agent Sessions Setup](#agent-sessions-setup)
- [Multi-Agent System](#multi-agent-system)
- [Plugin System](#plugin-system)
- [Semantic Memory](#semantic-memory)
- [Commands](#commands)
  - [CLI Commands](#cli-commands)
  - [Chat Commands](#chat-commands)
  - [Agent Commands](#agent-commands)
- [Configuration](#configuration)
- [Secrets Management](#secrets-management)
- [Cron Jobs](#cron-jobs)
- [Maintenance](#maintenance)
- [File Structure](#file-structure)
- [Docker](#docker)
- [Building from Source](#building-from-source)
- [License](#license)

---

## What is Magabot?

Magabot is a security-first multi-platform chatbot that connects your messaging platforms to large language models. It ships as a single static binary with zero runtime dependencies -- no Python, no Node.js, no containers required.

### Security by Design

Security is not an afterthought. Every layer of Magabot is built with defense in depth:

- **AES-256-GCM encryption** for secrets at rest, chat history, and platform sessions
- **Allowlist access control** with global admins, platform admins, and per-user/per-chat restrictions
- **Per-user rate limiting** to prevent abuse and runaway costs
- **Input sanitization** with control character stripping on all incoming messages
- **Parameterized SQL queries** throughout -- no string concatenation in database operations
- **Path traversal protection** on all file operations, downloads, and update extraction
- **Secure delete** enabled on the SQLite database (`secure_delete = ON`)
- **TLS 1.3** enforced on all outbound API calls to LLM providers and platform APIs
- **Restrictive file permissions** -- config files `0600`, directories `0700`, no root required
- **Decompression bomb protection** with size limits on archive extraction
- **SHA-256 checksum verification** on binary updates before installation
- **Limited response readers** (`io.LimitReader`) on all HTTP responses to prevent OOM

### Multi-Platform

Connect to users wherever they are:

- Telegram, Discord, Slack, WhatsApp, and generic HTTP Webhooks

### Multi-LLM with Fallback

Use any combination of LLM providers with automatic failover:

- Anthropic (Claude), OpenAI (GPT), Google Gemini, DeepSeek, Zhipu GLM, and Local/self-hosted models (Ollama, vLLM, llama.cpp, LocalAI)

If the primary provider is down or rate-limited, Magabot automatically tries the next provider in the fallback chain.

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

# 2. Quick init (auto-detects env vars, minimal prompts)
magabot init

# 3. Or use the full interactive wizard
magabot setup

# 4. Start
magabot start

# 5. Check status
magabot status
```

The `init` command auto-detects API keys from environment variables (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `TELEGRAM_BOT_TOKEN`, etc.), generates an encryption key, and writes a minimal working config. Use `setup` instead for full interactive configuration of every option.

---

## Uninstall

### Linux

```bash
# 1. Stop the daemon and remove config directory (~/.magabot)
magabot uninstall

# 2. Remove the binary
sudo rm /usr/local/bin/magabot

# 3. Remove the data directory
rm -rf ~/data/magabot
```

### macOS

```bash
# 1. Stop the daemon and remove config directory (~/.magabot)
magabot uninstall

# 2. Remove the binary
sudo rm /usr/local/bin/magabot

# 3. Remove the data directory
rm -rf ~/data/magabot
```

### Windows

```powershell
# 1. Stop the daemon and remove config directory
magabot.exe uninstall

# 2. Remove the binary from your PATH location

# 3. Remove the data directory
Remove-Item -Recurse -Force "$env:USERPROFILE\.magabot"
```

---

## Supported LLM Providers

| Provider | Default Model | Auth Method |
|----------|--------------|-------------|
| Anthropic | `claude-sonnet-4-20250514` | API key or Claude CLI OAuth |
| OpenAI | `gpt-4o` | API key |
| Gemini | `gemini-2.0-flash` | API key |
| DeepSeek | `deepseek-chat` | API key |
| GLM (Zhipu) | `glm-4` | API key |
| Local/self-hosted | `llama3` | None (or optional API key) |

The Local provider works with any server that exposes an OpenAI-compatible chat completions API:

- [Ollama](https://ollama.com) (`http://localhost:11434/v1`)
- [vLLM](https://docs.vllm.ai) (`http://localhost:8000/v1`)
- [llama.cpp server](https://github.com/ggerganov/llama.cpp) (`http://localhost:8080/v1`)
- [LocalAI](https://localai.io) (`http://localhost:8080/v1`)
- text-generation-webui with OpenAI extension

### Fallback Chain

If the default provider fails, Magabot automatically tries the next provider in the chain:

```yaml
llm:
  default: anthropic
  fallback_chain:
    - anthropic
    - deepseek
    - openai
```

---

## Supported Platforms

| Platform | Method | Group Chat | DMs | Status |
|----------|--------|:----------:|:---:|:------:|
| Telegram | Long Polling | Yes | Yes | Stable |
| Discord | Gateway | Yes | Yes | Stable |
| Slack | Socket Mode | Yes | Yes | Stable |
| WhatsApp | WebSocket (whatsmeow) | Yes | Yes | Beta |
| Webhook | HTTP POST | N/A | N/A | Stable |

---

## Hooks

Hooks are event-driven shell commands that run in response to bot lifecycle events. They allow you to extend Magabot with custom scripts for logging, moderation, notifications, analytics, and more.

### Event Types

| Event | Trigger | Can Modify? | Description |
|-------|---------|:-----------:|-------------|
| `pre_message` | Before a message is sent to the LLM | Yes (stdout replaces message) | Filter, transform, or block incoming messages |
| `post_response` | After the LLM responds | Yes (stdout replaces response) | Filter, transform, or augment LLM responses |
| `on_command` | When a chat command is executed | No | Log or react to commands like /help, /status |
| `on_start` | When the bot daemon starts | No | Send startup notifications, initialize resources |
| `on_stop` | When the bot daemon stops | No | Send shutdown notifications, cleanup resources |
| `on_error` | When an error occurs | No | Alert on errors, log to external systems |

Each hook receives event data as JSON on stdin. For `pre_message` and `post_response`, the hook can return modified text on stdout. If a synchronous hook exits with a non-zero status code, the message is blocked.

---

## Setup Guides

### Vault Setup

Magabot integrates with HashiCorp Vault (KV v2) to keep API keys and tokens out of config files.

**1. Start a Vault server** (development mode for testing):

```bash
vault server -dev -dev-root-token-id="dev-token"
```

**2. Store your secrets in Vault:**

```bash
export VAULT_ADDR="http://127.0.0.1:8200"
export VAULT_TOKEN="dev-token"

vault kv put secret/magabot/magabot/llm/anthropic_api_key value="sk-ant-..."
vault kv put secret/magabot/magabot/llm/openai_api_key value="sk-..."
vault kv put secret/magabot/magabot/telegram/bot_token value="123456:ABC-DEF..."
vault kv put secret/magabot/magabot/encryption_key value="your-key-here"
```

**3. Configure Magabot to use Vault:**

```yaml
# config.yaml
secrets:
  backend: "vault"
  vault:
    address: "http://127.0.0.1:8200"   # or set VAULT_ADDR env var
    mount_path: "secret"                 # KV v2 mount point
    secret_path: "magabot"               # base path for secrets
```

**4. Set the Vault token:**

```bash
export VAULT_TOKEN="hvs.your-vault-token"
```

**TLS options for production:**

For production Vault deployments, use HTTPS and configure TLS:

```yaml
secrets:
  backend: "vault"
  vault:
    address: "https://vault.example.com:8200"
    mount_path: "secret"
    secret_path: "magabot"
```

The Vault client respects standard environment variables: `VAULT_ADDR`, `VAULT_TOKEN`, `VAULT_CACERT`, `VAULT_CLIENT_CERT`, `VAULT_CLIENT_KEY`, `VAULT_SKIP_VERIFY`.

---

### LLM Setup

#### Anthropic (Claude)

**Option A: API Key**

1. Get an API key from [console.anthropic.com](https://console.anthropic.com/)
2. Configure:

```bash
magabot setup anthropic
```

Or set manually in `config.yaml`:

```yaml
llm:
  default: "anthropic"
  anthropic:
    enabled: true
    api_key: "sk-ant-api03-..."
    model: "claude-sonnet-4-20250514"
    max_tokens: 4096
    temperature: 0.7
```

**Option B: Claude CLI OAuth**

If you have Claude CLI installed and authenticated:

```bash
magabot setup anthropic
# Select "Claude CLI OAuth" when prompted
# Tokens are loaded from ~/.claude/.credentials.json
```

#### OpenAI (GPT)

1. Get an API key from [platform.openai.com/api-keys](https://platform.openai.com/api-keys)
2. Configure:

```bash
magabot setup openai
```

Or set manually:

```yaml
llm:
  openai:
    enabled: true
    api_key: "sk-..."
    model: "gpt-4o"
    max_tokens: 4096
```

#### Google Gemini

1. Get an API key from [aistudio.google.com/apikey](https://aistudio.google.com/apikey)
2. Configure:

```bash
magabot setup gemini
```

Or set manually:

```yaml
llm:
  gemini:
    enabled: true
    api_key: "AIza..."
    model: "gemini-2.0-flash"
```

#### DeepSeek

1. Get an API key from [platform.deepseek.com](https://platform.deepseek.com/)
2. Configure:

```bash
magabot setup deepseek
```

Or set manually:

```yaml
llm:
  deepseek:
    enabled: true
    api_key: "sk-..."
    model: "deepseek-chat"
```

#### GLM (Zhipu)

1. Get an API key from [open.bigmodel.cn](https://open.bigmodel.cn/)
2. Configure:

```bash
magabot setup glm
```

Or set manually:

```yaml
llm:
  glm:
    enabled: true
    api_key: "your-glm-key"
    model: "glm-4"
```

#### Local/Self-hosted

No API key required. Start your local LLM server, then configure Magabot to connect to it.

**Ollama example:**

```bash
# Start Ollama and pull a model
ollama pull llama3
ollama serve
```

```yaml
llm:
  default: "local"
  local:
    enabled: true
    base_url: "http://localhost:11434/v1"
    model: "llama3"
    max_tokens: 4096
```

**vLLM example:**

```yaml
llm:
  local:
    enabled: true
    base_url: "http://localhost:8000/v1"
    model: "meta-llama/Llama-3-8B-Instruct"
```

**llama.cpp server example:**

```yaml
llm:
  local:
    enabled: true
    base_url: "http://localhost:8080/v1"
    model: "default"
```

You can also configure the local provider via environment variables:

```bash
export LOCAL_LLM_URL="http://localhost:11434/v1"
export LOCAL_LLM_MODEL="llama3"
export LOCAL_LLM_API_KEY="optional-key"   # if your server requires one
```

---

### Platform Setup

#### Telegram

1. Open Telegram and message [@BotFather](https://t.me/BotFather)
2. Send `/newbot` and follow the prompts to create your bot
3. Copy the bot token (format: `123456789:ABCdefGHI...`)
4. Configure:

```bash
magabot setup telegram
```

Or set manually:

```yaml
platforms:
  telegram:
    enabled: true
    token: "123456789:ABCdefGHI..."
    admins: ["your_telegram_user_id"]
    allowed_users: []
    allowed_chats: []
    allow_groups: true
    allow_dms: true
```

To find your Telegram user ID, message [@userinfobot](https://t.me/userinfobot) on Telegram.

#### Discord

1. Go to the [Discord Developer Portal](https://discord.com/developers/applications)
2. Click "New Application" and give it a name
3. Go to "Bot" in the left sidebar and click "Add Bot"
4. Copy the bot token
5. Under "Privileged Gateway Intents", enable "Message Content Intent"
6. Go to "OAuth2" > "URL Generator", select `bot` scope with `Send Messages` + `Read Message History` permissions
7. Use the generated URL to invite the bot to your server
8. Configure:

```bash
magabot setup discord
```

Or set manually:

```yaml
platforms:
  discord:
    enabled: true
    token: "your-discord-bot-token"
    prefix: "!"
    admins: ["your_discord_user_id"]
    allowed_users: []
    allowed_chats: []
```

#### Slack

1. Go to [api.slack.com/apps](https://api.slack.com/apps) and click "Create New App"
2. Choose "From scratch" and name your app
3. Under "Socket Mode", enable it and generate an app-level token (starts with `xapp-`)
4. Under "OAuth & Permissions", add these Bot Token Scopes: `chat:write`, `app_mentions:read`, `im:history`, `im:read`, `im:write`
5. Install the app to your workspace and copy the Bot User OAuth Token (starts with `xoxb-`)
6. Under "Event Subscriptions", enable events and subscribe to: `message.im`, `app_mention`
7. Configure:

```bash
magabot setup slack
```

Or set manually:

```yaml
platforms:
  slack:
    enabled: true
    bot_token: "xoxb-..."
    app_token: "xapp-..."
    admins: ["your_slack_user_id"]
    allowed_users: []
    allowed_chats: []
```

#### WhatsApp

WhatsApp uses the whatsmeow library (pure Go, multi-device protocol). No external bridge or Node.js required.

1. Configure and start:

```bash
magabot setup whatsapp
magabot start
```

2. A QR code will be displayed in the terminal. Scan it with WhatsApp on your phone (Settings > Linked Devices > Link a Device).

```yaml
platforms:
  whatsapp:
    enabled: true
    admins: ["your_phone_number"]
    allowed_users: []
    allowed_chats: []
```

**Note:** WhatsApp support is in beta. The QR code must be scanned each time the session expires.

#### Webhook

The webhook platform exposes an HTTP endpoint that accepts POST requests with JSON payloads.

```bash
magabot setup webhook
```

Or set manually:

```yaml
platforms:
  webhook:
    enabled: true
    port: 8080
    path: "/webhook"
    bind: "127.0.0.1"
    auth_method: "bearer"     # none | bearer | hmac
    bearer_token: "your-secret-token"
    hmac_secret: ""
```

**Authentication methods:**

| Method | Description |
|--------|-------------|
| `none` | No authentication (not recommended) |
| `bearer` | Requires `Authorization: Bearer <token>` header |
| `hmac` | Validates HMAC-SHA256 signature in `X-Signature` header |

---

### Hooks Setup

Hooks are configured in a separate file: `~/.magabot/config-hooks.yml`. This keeps hook definitions independent from the main config.

**File format:**

```yaml
# ~/.magabot/config-hooks.yml
hooks:
  - name: "log-messages"
    event: "pre_message"
    command: "/path/to/log-message.sh"
    timeout: 10                        # seconds (default: 10)
    platforms: ["telegram", "discord"]  # empty = all platforms
    async: false                       # false = synchronous (can modify/block)

  - name: "notify-startup"
    event: "on_start"
    command: "curl -s https://ntfy.sh/my-topic -d 'Magabot started'"
    async: true                        # true = fire-and-forget

  - name: "content-filter"
    event: "post_response"
    command: "/path/to/filter.py"
    timeout: 5

  - name: "command-audit"
    event: "on_command"
    command: "/path/to/audit.sh"
    async: true

  - name: "shutdown-cleanup"
    event: "on_stop"
    command: "/path/to/cleanup.sh"
    timeout: 30

  - name: "error-alert"
    event: "on_error"
    command: "/path/to/alert.sh"
    async: true
```

You can also configure hooks interactively:

```bash
magabot setup hooks
```

**Event data (JSON on stdin):**

Every hook receives a JSON object on stdin with the following fields (present depending on event type):

```json
{
  "event": "pre_message",
  "platform": "telegram",
  "user_id": "123456",
  "chat_id": "-100987654",
  "text": "Hello bot",
  "response": "",
  "command": "",
  "args": [],
  "provider": "anthropic",
  "model": "claude-sonnet-4-20250514",
  "latency_ms": 0,
  "error": "",
  "version": "0.1.9",
  "platforms": ["telegram", "discord"]
}
```

**Example hook script** (content filter):

```bash
#!/bin/bash
# filter.sh - Block messages containing banned words
INPUT=$(cat)
TEXT=$(echo "$INPUT" | jq -r '.text // .response')

if echo "$TEXT" | grep -qi "banned-word"; then
  echo "Message blocked by content filter."
  exit 1  # non-zero exit = block the message
fi

# Output nothing to pass through unchanged, or echo modified text
```

**Sync vs Async:**

- **Synchronous** (`async: false`): The bot waits for the hook to complete. For `pre_message` and `post_response`, stdout replaces the message/response. A non-zero exit code blocks the message.
- **Asynchronous** (`async: true`): Fire-and-forget. The bot does not wait and cannot be blocked. Use for notifications, logging, and analytics.

**Timeout:** Each hook has a configurable timeout (default 10 seconds). If a hook exceeds its timeout, it is killed.

**Platform filtering:** Use the `platforms` list to restrict a hook to specific platforms. An empty list means the hook runs for all platforms.

---

### Agent Sessions Setup

Agent sessions let admins interact with coding agents (Claude Code, Codex, Gemini CLI) directly through chat. Messages prefixed with `:` are routed to the active agent session instead of the LLM.

**Supported agents:**

| Agent | Binary | Description |
|-------|--------|-------------|
| Claude Code | `claude` | Anthropic's CLI coding agent |
| Codex | `codex` | OpenAI's CLI coding agent |
| Gemini CLI | `gemini` | Google's CLI coding agent |

**Prerequisites:** The agent binary must be installed and available in `PATH` on the machine running Magabot.

**Configuration:**

```yaml
# config.yaml
agents:
  default: "claude"    # default agent type
  timeout: 120         # execution timeout in seconds
```

**Usage (from chat):**

```
:new claude ~/projects/myapp     Start a Claude Code session in ~/projects/myapp
:new codex ~/projects/myapp      Start a Codex session
:new gemini ~/projects/myapp     Start a Gemini CLI session
:send Fix the failing tests      Send a message to the active agent
:status                          Show the current agent session info
:quit                            Close the agent session
```

Once a session is active, any message prefixed with `:send` is routed to the coding agent. The agent runs in the specified working directory with one-shot CLI invocations. Conversation continuity is maintained via `--continue` flags (for Claude Code).

**Security notes:**

- Agent sessions are restricted to admins only
- Working directories are validated against an allowed path list (defaults to user's home directory)
- Path traversal is prevented via absolute path resolution
- Output is limited to 10 MB to prevent OOM
- ANSI escape sequences are stripped from output before sending to chat
- Each invocation has a configurable timeout (default 120 seconds)

---

## Multi-Agent System

Magabot supports spawning sub-agents for complex, long-running tasks. Each sub-agent runs in its own isolated session with independent context, allowing parallel execution and agent-to-agent communication.

### Architecture

```
Main Agent (root)
├── Sub-Agent 1 (research task)
│   └── Sub-Agent 1.1 (deep dive)
├── Sub-Agent 2 (code review)
└── Sub-Agent 3 (documentation)
```

### Features

- **Isolated Sessions**: Each sub-agent has its own conversation history and context
- **Parallel Execution**: Multiple sub-agents can run concurrently
- **Hierarchical Nesting**: Agents can spawn child agents (up to configurable depth)
- **Inter-Agent Messaging**: Agents can send messages to each other via inbox channels
- **Automatic Cleanup**: Completed agents are cleaned up after a configurable duration
- **Persistence**: Agent state is persisted to disk for crash recovery
- **Timeout Protection**: Each agent has a configurable timeout (default: 5 minutes)

### Configuration

```yaml
# config.yaml
subagents:
  max_agents: 50       # Maximum concurrent agents
  max_depth: 5         # Maximum nesting depth
  max_history: 100     # Maximum messages per agent
  default_timeout: 5m  # Default task timeout
```

### Chat Commands

| Command | Description |
|---------|-------------|
| `/task spawn <description>` | Spawn a new sub-agent with the given task |
| `/task list` | List all active sub-agents |
| `/task status <id>` | Get status of a specific agent |
| `/task cancel <id>` | Cancel a running agent |
| `/task clear` | Clean up completed agents |

### Agent States

| State | Description |
|-------|-------------|
| `pending` | Agent created but not yet started |
| `running` | Agent is executing its task |
| `complete` | Task finished successfully |
| `failed` | Task failed with an error |
| `canceled` | Agent was manually canceled |
| `timeout` | Task exceeded its timeout |

### Security Considerations

- Maximum concurrent agents limit prevents resource exhaustion
- Nesting depth limit prevents infinite recursion
- Task length validation (max 100KB)
- Context entry limits (max 100 keys, 256 char key length)
- Audit logging for agent lifecycle events

---

## Plugin System

Extend Magabot functionality with plugins. Plugins are Go packages that implement the `Plugin` interface and can register commands, hooks, and event listeners.

### Plugin Lifecycle

```
Unloaded → Loading → Initialized → Started → Stopping → Stopped
                 ↓                      ↓
               Error                  Error
```

### Creating a Plugin

```go
package myplugin

import (
    "context"
    "github.com/kusa/magabot/internal/plugin"
)

type MyPlugin struct {
    logger *slog.Logger
}

func (p *MyPlugin) Metadata() plugin.Metadata {
    return plugin.Metadata{
        ID:          "my-plugin",
        Name:        "My Plugin",
        Version:     "1.0.0",
        Description: "A sample plugin",
        Author:      "Your Name",
        Priority:    plugin.PriorityNormal,
    }
}

func (p *MyPlugin) Init(ctx plugin.Context) error {
    p.logger = ctx.Logger()
    
    // Register a command
    ctx.RegisterCommand("hello", func(ctx context.Context, cmd *plugin.Command) (string, error) {
        return "Hello from my plugin!", nil
    })
    
    // Register an event hook
    ctx.RegisterHook("on_message", func(ctx context.Context, event string, data interface{}) error {
        p.logger.Info("message received", "event", event)
        return nil
    })
    
    return nil
}

func (p *MyPlugin) Start(ctx context.Context) error {
    p.logger.Info("plugin started")
    return nil
}

func (p *MyPlugin) Stop(ctx context.Context) error {
    p.logger.Info("plugin stopped")
    return nil
}
```

### Plugin Priority

Plugins are initialized and started in priority order:

| Priority | Value | Use Case |
|----------|-------|----------|
| `PriorityCore` | 0 | Core system plugins |
| `PriorityHigh` | 100 | Critical dependencies |
| `PriorityNormal` | 500 | Standard plugins |
| `PriorityLow` | 900 | Optional features |
| `PriorityLast` | 999 | Cleanup/finalization |

### Plugin Context Services

Plugins have access to these host services via the `Context` interface:

| Method | Description |
|--------|-------------|
| `Logger()` | Get a namespaced logger |
| `Config()` | Access plugin configuration |
| `SetConfig(key, value)` | Update configuration |
| `DataDir()` | Get plugin's data directory |
| `SendMessage(platform, chatID, msg)` | Send a chat message |
| `RegisterCommand(cmd, handler)` | Register a command |
| `RegisterHook(event, handler)` | Register an event hook |
| `GetPlugin(id)` | Access another plugin |
| `Emit(event, data)` | Emit a custom event |

### Security Considerations

- Plugin IDs are validated to prevent path traversal
- Command names are restricted to alphanumeric, underscores, and hyphens
- Plugin data directories have restrictive permissions (0700)
- Plugin initialization timeout: 30 seconds

---

## Semantic Memory

Magabot includes a semantic memory system powered by vector embeddings. This enables the bot to remember information about users and retrieve contextually relevant memories during conversations.

### Embedding Providers

| Provider | Models | Best For |
|----------|--------|----------|
| OpenAI | `text-embedding-3-small`, `text-embedding-3-large` | General purpose, multilingual |
| Voyage AI | `voyage-3`, `voyage-code-3` | Code understanding |
| Cohere | `embed-english-v3.0`, `embed-multilingual-v3.0` | Multilingual search |
| Local | Any sentence-transformers model | Privacy, offline use |

### Configuration

```yaml
# config.yaml
memory:
  enabled: true
  max_entries: 1000          # Maximum memories per user
  context_limit: 2000        # Max tokens for context injection

embedding:
  provider: "openai"         # openai, voyage, cohere, local
  model: "text-embedding-3-small"
  api_key: "${OPENAI_API_KEY}"
  # For local provider:
  # base_url: "http://localhost:8080"
```

### Memory Types

| Type | Description | Example |
|------|-------------|---------|
| `fact` | Factual information | "User works at Acme Corp" |
| `preference` | User preferences | "User prefers dark mode" |
| `event` | Past events | "User's birthday is March 15" |
| `note` | General notes | "Follow up on project X" |
| `context` | Conversation context | Auto-extracted insights |

### Chat Commands

| Command | Description |
|---------|-------------|
| `/memory add <text>` | Store a new memory |
| `/memory <text>` | Shortcut to add a memory |
| `/memory search <query>` | Semantic search for memories |
| `/memory list [type]` | List memories, optionally by type |
| `/memory delete <id>` | Delete a specific memory |
| `/memory clear` | Clear all memories |
| `/memory stats` | Show memory statistics |

### How It Works

1. **Storage**: When you add a memory, it's embedded using the configured provider and stored in a local SQLite vector database.

2. **Retrieval**: During conversations, relevant memories are retrieved using cosine similarity search and injected into the LLM prompt as context.

3. **Automatic Extraction**: Optionally, the system can auto-extract important facts from conversations.

```
User: "Remember that I prefer Python over JavaScript"
Bot: "Got it! I'll remember you prefer Python over JavaScript."

[Later conversation]
User: "What language should I use for this project?"
Bot: [retrieves memory about Python preference]
     "Based on your preference for Python, I'd recommend..."
```

### Security Considerations

- Memories are stored per-user with filesystem isolation
- Content length limit: 100KB per memory
- Search results limited to prevent OOM (10,000 entries scanned max)
- API responses use io.LimitReader (10MB max)
- User IDs are sanitized for safe filesystem paths
- SSRF protection: cloud provider URLs cannot point to private networks

---

## Commands

### CLI Commands

#### Daemon

```bash
magabot start                # Start daemon (foreground)
magabot stop                 # Stop daemon
magabot restart              # Restart daemon
magabot status               # Show PID, uptime, config, DB size
magabot log                  # Tail log file
```

#### Setup

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
magabot setup hooks          # Configure hooks
```

#### Config

```bash
magabot config show          # Show config summary
magabot config edit          # Open in $EDITOR (nano/vim/vi)
magabot config path          # Print config file path
magabot config admin list          # List global admins
magabot config admin add <id>      # Add global admin
magabot config admin remove <id>   # Remove global admin
```

#### Cron

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

#### Skills

```bash
magabot skill list           # List installed skills
magabot skill info <name>    # Show skill details
magabot skill create <name>  # Create skill template
magabot skill enable <name>  # Enable a skill
magabot skill disable <name> # Disable a skill
magabot skill reload         # Reload all skills
magabot skill builtin        # List built-in skills
```

#### Update

```bash
magabot update check         # Check for new version
magabot update apply         # Download and install update
magabot update rollback      # Restore previous version
```

#### Utilities

```bash
magabot init                 # Zero-config quick init (auto-detects env vars)
magabot genkey               # Generate AES-256 encryption key
magabot reset                # Reset config (keeps platform connections)
magabot uninstall            # Remove magabot and all data
magabot version              # Show version
magabot help                 # Show help
```

---

### Chat Commands

Commands available when messaging the bot on any platform:

| Command | Description |
|---------|-------------|
| `/start` | Welcome message |
| `/help` | Show available commands |
| `/status` | Bot status (provider, message count) |
| `/models` | List available AI models |
| `/providers` | Show LLM providers and availability |

Any other message is sent to the active LLM provider for a response.

#### Admin Commands (in chat)

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

#### Memory Commands

```
/memory add <text>                # Remember something
/memory <text>                    # Shortcut to remember
/memory search <query>            # Search memories
/memory list [type]               # List memories (types: fact, preference, event, note)
/memory delete <id>               # Delete a memory
/memory clear                     # Clear all memories
/memory stats                     # Show statistics
```

#### Task Commands

```
/task spawn <description>         # Run a background task
/task list                        # List active sessions
/task status <id>                 # Show session status
/task cancel <id>                 # Cancel a running task
/task clear                       # Clear completed tasks
```

#### Heartbeat Commands

```
/heartbeat status                 # Show check status
/heartbeat run                    # Run all checks now
/heartbeat enable <name>          # Enable a check
/heartbeat disable <name>         # Disable a check
/heartbeat list                   # List configured checks
```

---

### Agent Commands

Agent commands use the `:` prefix (not `/`) and are available to admins only:

| Command | Description |
|---------|-------------|
| `:new <agent> <directory>` | Start a new agent session (e.g., `:new claude ~/myapp`) |
| `:send <message>` | Send a message to the active agent session |
| `:status` | Show current agent session info (agent type, directory, message count) |
| `:quit` | Close the active agent session |

If no agent type is specified, the default from config is used (defaults to `claude`).

---

## Configuration

Config file location: `~/.magabot/config.yaml`

### View and Edit

```bash
magabot config show          # Show config summary
magabot config edit          # Open in $EDITOR (nano/vim/vi)
magabot config path          # Print config file path
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
    model: "gemini-2.0-flash"

  deepseek:
    enabled: false
    api_key: ""
    model: "deepseek-chat"

  glm:
    enabled: false
    api_key: ""
    model: "glm-4"

  local:
    enabled: false
    base_url: "http://localhost:11434/v1"
    model: "llama3"
    api_key: ""              # optional
    max_tokens: 4096

security:
  encryption_key: ""         # Generate with: magabot genkey

secrets:
  backend: ""               # local | vault (see Secrets Management)

agents:
  default: "claude"          # claude | codex | gemini
  timeout: 120               # seconds

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

The Vault token is read from `VAULT_TOKEN` environment variable:

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

## Cron Jobs

Schedule messages to any platform channel with enhanced scheduling support.

### Channel Format

```
telegram:<chat_id>           # Telegram chat
whatsapp:<phone>             # WhatsApp (+62...)
slack:#<channel>             # Slack channel
discord:<channel_id>         # Discord channel
webhook:<url>                # Custom webhook URL
```

### Schedule Types

Magabot supports three schedule types:

#### Cron Expressions

Standard 5-field or 6-field (with seconds) cron syntax:

```
# 5-field: minute hour day month weekday
0 9 * * 1-5                 # 9:00 AM weekdays
0 */2 * * *                 # Every 2 hours
30 8,12,17 * * *            # 8:30 AM, 12:30 PM, 5:30 PM
0 9 1 * *                   # 9:00 AM on the 1st of each month

# 6-field: second minute hour day month weekday
0 30 9 * * 1-5              # 9:30:00 AM weekdays (with seconds)
*/10 * * * * *              # Every 10 seconds
```

**Supported field values:**

| Field | Range | Special Characters |
|-------|-------|-------------------|
| Second (optional) | 0-59 | `*` `,` `-` `/` |
| Minute | 0-59 | `*` `,` `-` `/` |
| Hour | 0-23 | `*` `,` `-` `/` |
| Day of Month | 1-31 | `*` `,` `-` `/` |
| Month | 1-12 or JAN-DEC | `*` `,` `-` `/` |
| Day of Week | 0-6 or SUN-SAT | `*` `,` `-` `/` |

**Predefined shortcuts:**

| Shortcut | Equivalent | Description |
|----------|------------|-------------|
| `@yearly` | `0 0 0 1 1 *` | Once a year (Jan 1, midnight) |
| `@monthly` | `0 0 0 1 * *` | Once a month (1st, midnight) |
| `@weekly` | `0 0 0 * * 0` | Once a week (Sunday, midnight) |
| `@daily` | `0 0 0 * * *` | Once a day (midnight) |
| `@hourly` | `0 0 * * * *` | Once an hour (top of hour) |

#### Interval Schedules

Simple interval-based scheduling with `every`:

```
every 5m                    # Every 5 minutes
every 2h                    # Every 2 hours
every 1h30m                 # Every 1 hour 30 minutes
every 24h                   # Every day
every minute                # Every minute
every hour                  # Every hour
every day                   # Every 24 hours
```

**Supported units:** `s` (seconds), `m` (minutes), `h` (hours), `d` (days), `w` (weeks)

#### One-Shot Schedules

Schedule a single execution at a specific time with `at`:

```
at 15:30                    # Today at 3:30 PM (or tomorrow if past)
at 2024-12-25T09:00         # December 25, 2024 at 9:00 AM
at 2024-12-31 23:59:59      # New Year's Eve countdown
```

**Supported formats:**
- `HH:MM` - Time today (or tomorrow if past)
- `HH:MM:SS` - Time with seconds
- `YYYY-MM-DD` - Date at midnight
- `YYYY-MM-DD HH:MM` - Date and time
- `YYYY-MM-DDTHH:MM:SS` - ISO 8601 format

### Timezone Support

All schedules support timezone specification:

```yaml
cron:
  jobs:
    - name: "morning-report"
      schedule: "0 9 * * 1-5"
      timezone: "Asia/Jakarta"       # IANA timezone name
      channel: "telegram:123456"
      message: "Good morning! Here's your daily report."
```

**Common timezone aliases:**

| Alias | IANA Name |
|-------|-----------|
| `WIB` | Asia/Jakarta |
| `WITA` | Asia/Makassar |
| `WIT` | Asia/Jayapura |
| `JST` | Asia/Tokyo |
| `SGT` | Asia/Singapore |
| `EST` | America/New_York |
| `PST` | America/Los_Angeles |
| `UTC` | UTC |

### CLI Commands

```bash
magabot cron list            # List all jobs
magabot cron list -a         # Include disabled jobs
magabot cron add             # Add a new job (interactive)
magabot cron edit <id>       # Edit a job
magabot cron delete <id>     # Delete a job
magabot cron enable <id>     # Enable a job
magabot cron disable <id>    # Disable a job
magabot cron run <id>        # Run a job immediately
magabot cron show <id>       # Show job details
```

---

## Maintenance

### Stop the Daemon

```bash
magabot stop
```

### View Logs

```bash
# Tail the log file in real time
magabot log

# Log file is at ~/data/magabot/logs/magabot.log (configurable)
```

### Reset Config

```bash
# Reset config to defaults (keeps platform connections like WhatsApp sessions)
magabot reset
```

### Update

```bash
# Check if a new version is available
magabot update check

# Download and install (stops bot, verifies SHA-256 checksum, installs, backs up old binary)
magabot update apply

# Rollback to the previous version if something goes wrong
magabot update rollback
```

Updates are downloaded from [GitHub Releases](https://github.com/kusandriadi/magabot/releases). The previous binary is saved as a `.backup` file for rollback.

### Complete Uninstall

#### Linux

```bash
magabot uninstall
sudo rm /usr/local/bin/magabot
rm -rf ~/data/magabot
```

#### macOS

```bash
magabot uninstall
sudo rm /usr/local/bin/magabot
rm -rf ~/data/magabot
```

#### Windows

```powershell
magabot.exe uninstall
# Remove the binary from your PATH location
Remove-Item -Recurse -Force "$env:USERPROFILE\.magabot"
```

---

## File Structure

```
~/.magabot/
  config.yaml               # Configuration
  config-hooks.yml           # Hooks configuration
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

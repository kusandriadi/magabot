// Package config provides unified configuration management
package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/kusa/magabot/internal/util"
	"gopkg.in/yaml.v3"
)

// Config is the main configuration structure
type Config struct {
	mu       sync.RWMutex `yaml:"-"`
	filePath string       `yaml:"-"`

	// Bot identity
	Bot BotConfig `yaml:"bot"`

	// Platform configurations
	Platforms PlatformsConfig `yaml:"platforms"`

	// LLM providers
	LLM LLMConfig `yaml:"llm"`

	// Access control
	Access AccessConfig `yaml:"access"`

	// Cron jobs
	Cron CronConfig `yaml:"cron"`

	// Heartbeat settings
	Heartbeat HeartbeatConfig `yaml:"heartbeat"`

	// Memory/RAG settings
	Memory MemoryConfig `yaml:"memory"`

	// Session settings
	Session SessionConfig `yaml:"session"`

	// Logging settings
	Logging LoggingConfig `yaml:"logging"`

	// Security settings
	Security SecurityConfig `yaml:"security"`

	// Storage settings
	Storage StorageConfig `yaml:"storage"`

	// Paths settings
	Paths PathsConfig `yaml:"paths"`

	// Skills settings
	Skills SkillsConfig `yaml:"skills"`

	// Secrets backend
	Secrets SecretsConfig `yaml:"secrets"`

	// Server settings
	Server ServerConfig `yaml:"server"`

	// Hooks (event-driven shell commands)
	Hooks []HookConfig `yaml:"hooks,omitempty"`

	// Agent sessions (coding agents via chat)
	Agent AgentConfig `yaml:"agent"`

	// Sub-agent system settings
	SubAgents SubAgentConfig `yaml:"subagents"`

	// Plugin system settings
	Plugins PluginConfig `yaml:"plugins"`

	// Embedding/Vector settings
	Embedding EmbeddingConfig `yaml:"embedding"`

	// Personas
	Personas PersonasConfig `yaml:"personas"`

	// Metadata
	Version     string    `yaml:"version"`
	LastUpdated time.Time `yaml:"last_updated"`
	UpdatedBy   string    `yaml:"updated_by"`
}

// BotConfig holds bot identity settings
type BotConfig struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Prefix      string `yaml:"prefix"` // Command prefix (default: /)
}

// PlatformsConfig holds all platform configurations
type PlatformsConfig struct {
	Telegram *TelegramConfig `yaml:"telegram,omitempty"`
	Discord  *DiscordConfig  `yaml:"discord,omitempty"`
	Slack    *SlackConfig    `yaml:"slack,omitempty"`
	WhatsApp *WhatsAppConfig `yaml:"whatsapp,omitempty"`
	Webhook  *WebhookConfig  `yaml:"webhook,omitempty"`
}

// TelegramConfig for Telegram platform
type TelegramConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Token    string `yaml:"token"`
	BotToken string `yaml:"bot_token"` // Alias for token
	// Access control for this platform
	Admins       []string `yaml:"admins"`        // Platform admins (can change config)
	AllowedUsers []string `yaml:"allowed_users"` // Allowed user IDs
	AllowedChats []string `yaml:"allowed_chats"` // Allowed group/chat IDs
	AllowGroups  bool     `yaml:"allow_groups"`
	AllowDMs     bool     `yaml:"allow_dms"`
	// Webhook mode (alternative to long polling)
	UseWebhook    bool   `yaml:"use_webhook"`
	WebhookURL    string `yaml:"webhook_url"`    // Public URL (https://yourdomain.com)
	WebhookPort   int    `yaml:"webhook_port"`   // Local port to listen on
	WebhookPath   string `yaml:"webhook_path"`   // Path (e.g., /telegram)
	WebhookSecret string `yaml:"webhook_secret"` // Secret token for verification
}

// DiscordConfig for Discord platform
type DiscordConfig struct {
	Enabled      bool     `yaml:"enabled"`
	Token        string   `yaml:"token"`
	Prefix       string   `yaml:"prefix"` // Command prefix (default: !)
	Admins       []string `yaml:"admins"`
	AllowedUsers []string `yaml:"allowed_users"`
	AllowedChats []string `yaml:"allowed_chats"` // Guild or channel IDs
	AllowGroups  bool     `yaml:"allow_groups"`
	AllowDMs     bool     `yaml:"allow_dms"`
}

// SlackConfig for Slack platform
type SlackConfig struct {
	Enabled      bool     `yaml:"enabled"`
	BotToken     string   `yaml:"bot_token"`
	AppToken     string   `yaml:"app_token"`
	Admins       []string `yaml:"admins"`
	AllowedUsers []string `yaml:"allowed_users"`
	AllowedChats []string `yaml:"allowed_chats"`
	AllowGroups  bool     `yaml:"allow_groups"`
	AllowDMs     bool     `yaml:"allow_dms"`
	// Webhook mode (Events API instead of Socket Mode)
	UseWebhook    bool   `yaml:"use_webhook"`
	WebhookPort   int    `yaml:"webhook_port"` // Local port to listen on
	WebhookPath   string `yaml:"webhook_path"` // Path (e.g., /slack/events)
	SigningSecret string `yaml:"signing_secret"`
}

// WhatsAppConfig for WhatsApp platform
type WhatsAppConfig struct {
	Enabled      bool     `yaml:"enabled"`
	Admins       []string `yaml:"admins"`
	AllowedUsers []string `yaml:"allowed_users"`
	AllowedChats []string `yaml:"allowed_chats"`
	AllowGroups  bool     `yaml:"allow_groups"`
	AllowDMs     bool     `yaml:"allow_dms"`
}

// WebhookConfig for generic webhook
type WebhookConfig struct {
	Enabled      bool              `yaml:"enabled"`
	Port         int               `yaml:"port"`
	Path         string            `yaml:"path"`
	Bind         string            `yaml:"bind"`
	Secret       string            `yaml:"secret,omitempty"`
	AuthMethod   string            `yaml:"auth_method"`             // bearer, hmac, none
	BearerToken  string            `yaml:"bearer_token,omitempty"`  // Legacy single token
	BearerTokens map[string]string `yaml:"bearer_tokens,omitempty"` // token -> user_id (secure)
	HMACSecret   string            `yaml:"hmac_secret,omitempty"`   // Legacy single secret
	HMACUsers    map[string]string `yaml:"hmac_users,omitempty"`    // secret -> user_id (secure)
	Admins       []string          `yaml:"admins"`
	AllowedIPs   []string          `yaml:"allowed_ips"`
	AllowedUsers []string          `yaml:"allowed_users"` // Required: allowed user IDs
}

// LLMConfig holds LLM provider settings
type LLMConfig struct {
	Main               string          `yaml:"main"`                // Main/primary provider
	MainProvider       string          `yaml:"main_provider"`       // Alias for main
	Providers          ProvidersConfig `yaml:"providers,omitempty"` // Alternative structure
	SystemPrompt       string          `yaml:"system_prompt"`
	MaxInputLength     int             `yaml:"max_input_length"`
	Timeout            int             `yaml:"timeout"`           // seconds; idle timeout per chunk during streaming
	MaxContextChars    int             `yaml:"max_context_chars"` // max total chars sent to LLM (trims oldest messages)
	RateLimit          int             `yaml:"rate_limit"`
	MaxContextTokens   int             `yaml:"max_context_tokens"`
	TruncationStrategy string          `yaml:"truncation_strategy"`
	PromptCaching      bool            `yaml:"prompt_caching"`

	// Direct provider configs (preferred structure)
	// omitempty: disabled providers are pruned on save so only active ones appear in YAML
	Anthropic LLMProviderConfig `yaml:"anthropic,omitempty"`
	OpenAI    LLMProviderConfig `yaml:"openai,omitempty"`
	Gemini    LLMProviderConfig `yaml:"gemini,omitempty"`
	GLM       LLMProviderConfig `yaml:"glm,omitempty"`
	DeepSeek  LLMProviderConfig `yaml:"deepseek,omitempty"`
	Local     LLMProviderConfig `yaml:"local,omitempty"` // Self-hosted (Ollama, vLLM, llama.cpp, etc.)
	Kimi      LLMProviderConfig `yaml:"kimi,omitempty"`
	Qwen      LLMProviderConfig `yaml:"qwen,omitempty"`
	MiniMax   LLMProviderConfig `yaml:"minimax,omitempty"`
}

// LLMProviderConfig holds config for a single LLM provider
type LLMProviderConfig struct {
	Enabled       bool           `yaml:"enabled"`
	Mode          string         `yaml:"mode,omitempty"`       // "api" (default) or "cli" (Claude CLI)
	APIKey        string         `yaml:"api_key"`              // #nosec G117 -- config field
	AuthToken     string         `yaml:"auth_token,omitempty"` // OAuth token (Claude Pro/Max)
	Model         string         `yaml:"model"`
	MaxTokens     int            `yaml:"max_tokens"`
	Temperature   float64        `yaml:"temperature"`
	BaseURL       string         `yaml:"base_url,omitempty"`
	CLIPath       string         `yaml:"cli_path,omitempty"`      // Path to claude binary (default: "claude")
	AllowedTools  []string       `yaml:"allowed_tools,omitempty"` // Allowed tools for CLI mode (empty = default: Read,Glob,Grep,WebSearch,WebFetch)
	MaxRetries    int            `yaml:"max_retries"`
	Effort        string         `yaml:"effort,omitempty"`         // CLI effort level: low, medium, high, max
	FallbackModel string         `yaml:"fallback_model,omitempty"` // CLI fallback model
	Agent         LLMAgentConfig `yaml:"agent"`                    // Agent session settings for this provider
}

// LLMAgentConfig holds agent session settings per LLM provider
type LLMAgentConfig struct {
	Timeout    int `yaml:"timeout"`     // seconds per attempt, default 300
	MaxRetries int `yaml:"max_retries"` // auto-retry on timeout, default 2
}

// ProvidersConfig holds individual LLM provider configs (alternative structure)
type ProvidersConfig struct {
	Anthropic *LLMProviderConfig `yaml:"anthropic,omitempty"`
	OpenAI    *LLMProviderConfig `yaml:"openai,omitempty"`
	Gemini    *LLMProviderConfig `yaml:"gemini,omitempty"`
	GLM       *LLMProviderConfig `yaml:"glm,omitempty"`
	DeepSeek  *LLMProviderConfig `yaml:"deepseek,omitempty"`
	Kimi      *LLMProviderConfig `yaml:"kimi,omitempty"`
	Qwen      *LLMProviderConfig `yaml:"qwen,omitempty"`
	MiniMax   *LLMProviderConfig `yaml:"minimax,omitempty"`
}

// AccessConfig holds global access settings
type AccessConfig struct {
	Mode string `yaml:"mode"` // allowlist, denylist, open
}

// CronConfig holds cron job definitions
type CronConfig struct {
	Enabled bool      `yaml:"enabled"`
	Jobs    []CronJob `yaml:"jobs"`
}

// HeartbeatConfig holds heartbeat settings
type HeartbeatConfig struct {
	Enabled  bool          `yaml:"enabled"`
	Interval time.Duration `yaml:"interval"` // e.g., 30m
	Targets  []CronTarget  `yaml:"targets"`  // Where to send alerts
}

// MemoryConfig holds memory/RAG settings
type MemoryConfig struct {
	Enabled      bool `yaml:"enabled"`
	MaxEntries   int  `yaml:"max_entries"`   // Max memories per user
	ContextLimit int  `yaml:"context_limit"` // Max tokens for context
}

// SessionConfig holds session settings
type SessionConfig struct {
	MaxHistory  int           `yaml:"max_history"`  // Max messages per session
	TaskTimeout time.Duration `yaml:"task_timeout"` // Timeout for background tasks
	CleanupAge  time.Duration `yaml:"cleanup_age"`  // When to cleanup old sessions
}

// CronJob defines a scheduled job
type CronJob struct {
	ID          string       `yaml:"id"`
	Name        string       `yaml:"name"`
	Description string       `yaml:"description,omitempty"`
	Schedule    string       `yaml:"schedule"`
	Message     string       `yaml:"message"`
	Channels    []CronTarget `yaml:"channels"`
	Enabled     bool         `yaml:"enabled"`
}

// CronTarget defines where to send cron notifications
type CronTarget struct {
	Platform string `yaml:"platform"`
	ChatID   string `yaml:"chat_id"`
	Name     string `yaml:"name,omitempty"`
}

// ServerConfig holds server settings
type ServerConfig struct {
	DataDir    string `yaml:"data_dir"`
	LogLevel   string `yaml:"log_level"`
	MaxRetries int    `yaml:"max_retries"`
}

// LoggingConfig holds logging settings
type LoggingConfig struct {
	Level  string `yaml:"level"`  // debug, info, warn, error
	File   string `yaml:"file"`   // Log file path (empty = stderr)
	Format string `yaml:"format"` // json or text
}

// SecurityConfig holds security settings
type SecurityConfig struct {
	EncryptionKey string              `yaml:"encryption_key"`
	AllowedUsers  map[string][]string `yaml:"allowed_users"` // platform -> user IDs
	RateLimit     RateLimitConfig     `yaml:"rate_limit"`
}

// RateLimitConfig holds rate limiting settings
type RateLimitConfig struct {
	MessagesPerMinute int `yaml:"messages_per_minute"`
	CommandsPerMinute int `yaml:"commands_per_minute"`
}

// StorageConfig holds storage settings
type StorageConfig struct {
	Database string       `yaml:"database"` // SQLite database path
	Backup   BackupConfig `yaml:"backup"`
}

// PathsConfig holds directory paths
type PathsConfig struct {
	DataDir      string `yaml:"data_dir"`      // Base data directory (default: ~/.magabot/data)
	LogsDir      string `yaml:"logs_dir"`      // Logs directory (default: data_dir/logs)
	MemoryDir    string `yaml:"memory_dir"`    // Memory/RAG directory (default: data_dir/memory)
	CacheDir     string `yaml:"cache_dir"`     // Cache directory (default: data_dir/cache)
	ExportsDir   string `yaml:"exports_dir"`   // Exports directory (default: data_dir/exports)
	DownloadsDir string `yaml:"downloads_dir"` // Downloads directory (default: data_dir/downloads)
}

// SkillsConfig holds skills settings
type SkillsConfig struct {
	Dir        string `yaml:"dir"`         // Skills directory (default: ~/code/magabot-skills)
	AutoReload bool   `yaml:"auto_reload"` // Watch for changes and reload (default: true)
}

// BackupConfig holds backup settings
type BackupConfig struct {
	Enabled   bool   `yaml:"enabled"`
	Path      string `yaml:"path"`
	KeepCount int    `yaml:"keep_count"`
}

// HookConfig defines an event-driven shell command hook.
type HookConfig struct {
	Name      string   `yaml:"name"`
	Event     string   `yaml:"event"` // pre_message, post_response, on_command, on_start, on_stop, on_error
	Command   string   `yaml:"command"`
	Timeout   int      `yaml:"timeout,omitempty"`   // seconds, default 10
	Platforms []string `yaml:"platforms,omitempty"` // empty = all platforms
	Async     bool     `yaml:"async,omitempty"`     // fire-and-forget
}

// AgentConfig holds coding agent session settings
type AgentConfig struct {
	Timeout        int               `yaml:"timeout"`         // seconds per attempt, default 300
	MaxRetries     int               `yaml:"max_retries"`     // auto-retry on timeout, default 2
	SessionTimeout int               `yaml:"session_timeout"` // idle session timeout in seconds (0 = disabled, default 21600 = 6h)
	Shortcuts      map[string]string `yaml:"shortcuts"`       // directory shortcuts, e.g. "myproject": "~/code/myproject"
	DiscoverDepth  int               `yaml:"discover_depth"`  // auto-discover search depth (default 3)
	PlanDelegate   *bool             `yaml:"plan_delegate"`   // plan first, then delegate to subagents (default: true)
}

// HooksFile is the top-level structure for config-hooks.yml
type HooksFile struct {
	Hooks []HookConfig `yaml:"hooks"`
}

// SecretsConfig holds secrets backend settings
type SecretsConfig struct {
	Backend string              `yaml:"backend"` // local, vault
	Vault   *VaultSecretsConfig `yaml:"vault,omitempty"`
	Local   *LocalSecretsConfig `yaml:"local,omitempty"`
}

// VaultSecretsConfig holds Vault-specific secrets settings
type VaultSecretsConfig struct {
	Address       string `yaml:"address"`
	MountPath     string `yaml:"mount_path"`
	SecretPath    string `yaml:"secret_path"`
	TLSCACert     string `yaml:"tls_ca_cert,omitempty"`
	TLSClientCert string `yaml:"tls_client_cert,omitempty"`
	TLSClientKey  string `yaml:"tls_client_key,omitempty"`
	TLSSkipVerify bool   `yaml:"tls_skip_verify,omitempty"`
}

// SubAgentConfig holds sub-agent system settings
type SubAgentConfig struct {
	Enabled    bool          `yaml:"enabled"`     // Enable sub-agent system
	MaxAgents  int           `yaml:"max_agents"`  // Max concurrent agents (default: 50)
	MaxDepth   int           `yaml:"max_depth"`   // Max nesting depth (default: 5)
	MaxHistory int           `yaml:"max_history"` // Max messages per agent (default: 100)
	Timeout    time.Duration `yaml:"timeout"`     // Default task timeout (default: 5m)
	Persist    bool          `yaml:"persist"`     // Persist agent state across restarts
}

// PluginConfig holds plugin system settings
type PluginConfig struct {
	Enabled   bool     `yaml:"enabled"`             // Enable plugin system
	Dirs      []string `yaml:"dirs"`                // Plugin directories to scan
	AutoLoad  bool     `yaml:"auto_load"`           // Auto-load plugins on startup
	AutoStart bool     `yaml:"auto_start"`          // Auto-start plugins after loading
	HotReload bool     `yaml:"hot_reload"`          // Watch for plugin changes
	Allowlist []string `yaml:"allowlist,omitempty"` // Only load these plugins (empty = all)
	Denylist  []string `yaml:"denylist,omitempty"`  // Never load these plugins
}

// EmbeddingConfig holds embedding/vector settings
type EmbeddingConfig struct {
	Enabled      bool   `yaml:"enabled"`              // Enable embedding generation
	Provider     string `yaml:"provider"`             // openai, voyage, cohere, local
	APIKey       string `yaml:"api_key"`              // API key for provider // #nosec G117
	Model        string `yaml:"model"`                // Embedding model name
	BaseURL      string `yaml:"base_url,omitempty"`   // Custom API base URL
	Dimensions   int    `yaml:"dimensions,omitempty"` // Output dimensions
	MaxBatchSize int    `yaml:"max_batch_size"`       // Max texts per batch (default: 100)
	Timeout      int    `yaml:"timeout"`              // API timeout in seconds (default: 30)
	// Memory integration
	AutoEmbed   bool `yaml:"auto_embed"`   // Auto-generate embeddings for memories
	SearchLimit int  `yaml:"search_limit"` // Default search result limit (default: 10)
}

// PersonasConfig holds persona settings
type PersonasConfig struct {
	Default string    `yaml:"default"`
	List    []Persona `yaml:"list"`
}

// Persona defines an AI persona
type Persona struct {
	Name         string `yaml:"name"`
	Description  string `yaml:"description"`
	Personality  string `yaml:"personality"`
	SystemPrompt string `yaml:"system_prompt"`
	FirstMessage string `yaml:"first_message"`
}

// LocalSecretsConfig holds local file-based secrets settings
type LocalSecretsConfig struct {
	Path string `yaml:"path"`
}

// Load reads config from file
func Load(filePath string) (*Config, error) {
	cfg := &Config{
		filePath: filePath,
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return default config
			cfg.setDefaults()
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	// Expand environment variable references before parsing YAML
	// Supports $VAR_NAME and ${VAR_NAME} syntax
	expanded := expandEnvVars(string(data))

	if err := yaml.Unmarshal([]byte(expanded), cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	cfg.setDefaults()
	return cfg, nil
}

// envVarPattern matches $VAR_NAME and ${VAR_NAME} patterns
var envVarPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}|\$([A-Za-z_][A-Za-z0-9_]*)`)

// expandEnvVars replaces $VAR and ${VAR} references with environment variable values
func expandEnvVars(input string) string {
	return envVarPattern.ReplaceAllStringFunc(input, func(match string) string {
		var name string
		if strings.HasPrefix(match, "${") {
			name = match[2 : len(match)-1]
		} else {
			name = match[1:]
		}
		if val, ok := os.LookupEnv(name); ok {
			return val
		}
		return match // Keep original if env var not set
	})
}

// setDefaults sets default values for missing fields
func (c *Config) setDefaults() {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}

	if c.Bot.Prefix == "" {
		c.Bot.Prefix = "/"
	}
	if c.Access.Mode == "" {
		c.Access.Mode = "allowlist"
	}
	if c.Version == "" {
		c.Version = "1"
	}

	// Paths defaults
	if c.Paths.DataDir == "" {
		c.Paths.DataDir = filepath.Join(home, ".magabot", "data")
	}
	// Expand ~ in paths
	c.Paths.DataDir = expandPath(c.Paths.DataDir)

	if c.Paths.LogsDir == "" {
		c.Paths.LogsDir = filepath.Join(c.Paths.DataDir, "logs")
	}
	c.Paths.LogsDir = expandPath(c.Paths.LogsDir)

	if c.Paths.MemoryDir == "" {
		c.Paths.MemoryDir = filepath.Join(c.Paths.DataDir, "memory")
	}
	c.Paths.MemoryDir = expandPath(c.Paths.MemoryDir)

	if c.Paths.CacheDir == "" {
		c.Paths.CacheDir = filepath.Join(c.Paths.DataDir, "cache")
	}
	c.Paths.CacheDir = expandPath(c.Paths.CacheDir)

	if c.Paths.ExportsDir == "" {
		c.Paths.ExportsDir = filepath.Join(c.Paths.DataDir, "exports")
	}
	c.Paths.ExportsDir = expandPath(c.Paths.ExportsDir)

	if c.Paths.DownloadsDir == "" {
		c.Paths.DownloadsDir = filepath.Join(c.Paths.DataDir, "downloads")
	}
	c.Paths.DownloadsDir = expandPath(c.Paths.DownloadsDir)

	// Skills defaults
	if c.Skills.Dir == "" {
		c.Skills.Dir = filepath.Join(home, "code", "magabot-skills")
	}
	c.Skills.Dir = expandPath(c.Skills.Dir)
	// Auto reload enabled by default
	// (already false by default, set explicitly if needed)

	// Agent defaults
	if c.Agent.Timeout <= 0 {
		c.Agent.Timeout = 300
	}
	if c.Agent.MaxRetries <= 0 {
		c.Agent.MaxRetries = 2
	}
	if c.Agent.SessionTimeout == 0 {
		c.Agent.SessionTimeout = 21600 // 6 hours
	}
	if c.Agent.PlanDelegate == nil {
		t := true
		c.Agent.PlanDelegate = &t
	}

	// Platform defaults
	if c.Platforms.Telegram != nil {
		// Coalesce Token / BotToken so either YAML key works
		if c.Platforms.Telegram.BotToken == "" && c.Platforms.Telegram.Token != "" {
			c.Platforms.Telegram.BotToken = c.Platforms.Telegram.Token
		}
	}
	if c.Platforms.Discord != nil {
		if c.Platforms.Discord.Prefix == "" {
			c.Platforms.Discord.Prefix = "!"
		}
	}

	// SubAgent defaults
	if c.SubAgents.MaxAgents <= 0 {
		c.SubAgents.MaxAgents = 50
	}
	if c.SubAgents.MaxDepth <= 0 {
		c.SubAgents.MaxDepth = 5
	}
	if c.SubAgents.MaxHistory <= 0 {
		c.SubAgents.MaxHistory = 100
	}
	if c.SubAgents.Timeout <= 0 {
		c.SubAgents.Timeout = 5 * time.Minute
	}

	// Plugin defaults
	if len(c.Plugins.Dirs) == 0 {
		c.Plugins.Dirs = []string{filepath.Join(home, ".magabot", "plugins")}
	}
	// Expand paths
	for i, dir := range c.Plugins.Dirs {
		c.Plugins.Dirs[i] = expandPath(dir)
	}

	// Embedding defaults
	if c.Embedding.MaxBatchSize <= 0 {
		c.Embedding.MaxBatchSize = 100
	}
	if c.Embedding.Timeout <= 0 {
		c.Embedding.Timeout = 30
	}
	if c.Embedding.SearchLimit <= 0 {
		c.Embedding.SearchLimit = 10
	}
	if c.Embedding.Provider == "" {
		c.Embedding.Provider = "openai"
	}
	if c.Embedding.Model == "" {
		c.Embedding.Model = "text-embedding-3-small"
	}
}

// expandPath expands ~ to home directory
func expandPath(path string) string {
	if len(path) > 0 && path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			home = os.TempDir()
		}
		return filepath.Join(home, path[1:])
	}
	return path
}

// Save writes config to file.
// Disabled LLM providers and platforms are pruned so only active entries
// appear in the YAML file. The live in-memory config is not modified.
func (c *Config) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.LastUpdated = time.Now()

	// Temporarily prune disabled entries for clean serialization.
	// Restore originals after marshaling so the live config is unchanged.
	origLLM := c.LLM
	origPlatforms := c.Platforms
	c.pruneDisabledForSave()
	defer func() {
		c.LLM = origLLM
		c.Platforms = origPlatforms
	}()

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(c.filePath), 0700); err != nil {
		return fmt.Errorf("failed to create config dir: %w", err)
	}

	// Write atomically
	tmpFile := c.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	if err := os.Rename(tmpFile, c.filePath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to save config: %w", err)
	}

	return nil
}

// pruneDisabledForSave zeros disabled LLM providers and nils disabled platform
// pointers so that omitempty causes them to be omitted from the YAML output.
// Must be called while holding mu. Caller must restore original values after marshaling.
func (c *Config) pruneDisabledForSave() {
	// Zero out disabled LLM providers (omitempty skips zero-valued structs)
	z := LLMProviderConfig{}
	if !c.LLM.Anthropic.Enabled {
		c.LLM.Anthropic = z
	}
	if !c.LLM.OpenAI.Enabled {
		c.LLM.OpenAI = z
	}
	if !c.LLM.Gemini.Enabled {
		c.LLM.Gemini = z
	}
	if !c.LLM.GLM.Enabled {
		c.LLM.GLM = z
	}
	if !c.LLM.DeepSeek.Enabled {
		c.LLM.DeepSeek = z
	}
	if !c.LLM.Local.Enabled {
		c.LLM.Local = z
	}
	if !c.LLM.Kimi.Enabled {
		c.LLM.Kimi = z
	}
	if !c.LLM.Qwen.Enabled {
		c.LLM.Qwen = z
	}
	if !c.LLM.MiniMax.Enabled {
		c.LLM.MiniMax = z
	}

	// Also prune the alternative Providers structure
	if c.LLM.Providers.Anthropic != nil && !c.LLM.Providers.Anthropic.Enabled {
		c.LLM.Providers.Anthropic = nil
	}
	if c.LLM.Providers.OpenAI != nil && !c.LLM.Providers.OpenAI.Enabled {
		c.LLM.Providers.OpenAI = nil
	}
	if c.LLM.Providers.Gemini != nil && !c.LLM.Providers.Gemini.Enabled {
		c.LLM.Providers.Gemini = nil
	}
	if c.LLM.Providers.GLM != nil && !c.LLM.Providers.GLM.Enabled {
		c.LLM.Providers.GLM = nil
	}
	if c.LLM.Providers.DeepSeek != nil && !c.LLM.Providers.DeepSeek.Enabled {
		c.LLM.Providers.DeepSeek = nil
	}
	if c.LLM.Providers.Kimi != nil && !c.LLM.Providers.Kimi.Enabled {
		c.LLM.Providers.Kimi = nil
	}
	if c.LLM.Providers.Qwen != nil && !c.LLM.Providers.Qwen.Enabled {
		c.LLM.Providers.Qwen = nil
	}
	if c.LLM.Providers.MiniMax != nil && !c.LLM.Providers.MiniMax.Enabled {
		c.LLM.Providers.MiniMax = nil
	}

	// Nil out disabled platforms (already pointers with omitempty)
	if c.Platforms.Telegram != nil && !c.Platforms.Telegram.Enabled {
		c.Platforms.Telegram = nil
	}
	if c.Platforms.Discord != nil && !c.Platforms.Discord.Enabled {
		c.Platforms.Discord = nil
	}
	if c.Platforms.Slack != nil && !c.Platforms.Slack.Enabled {
		c.Platforms.Slack = nil
	}
	if c.Platforms.WhatsApp != nil && !c.Platforms.WhatsApp.Enabled {
		c.Platforms.WhatsApp = nil
	}
	if c.Platforms.Webhook != nil && !c.Platforms.Webhook.Enabled {
		c.Platforms.Webhook = nil
	}
}

// SaveBy saves config with updater info
func (c *Config) SaveBy(updatedBy string) error {
	c.UpdatedBy = updatedBy
	return c.Save()
}

// IsPlatformAdmin checks if user is an admin for a specific platform
func (c *Config) IsPlatformAdmin(platform, userID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.isPlatformAdmin(platform, userID)
}

// isPlatformAdmin is the lock-free internal version (caller must hold mu)
func (c *Config) isPlatformAdmin(platform, userID string) bool {
	pa := c.getPlatformAccess(platform)
	return pa != nil && util.Contains(pa.Admins, userID)
}

// IsAllowed checks if user/chat is allowed on a platform
func (c *Config) IsAllowed(platform, userID, chatID string, isGroup bool) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Bootstrap: no admins exist at all — allow first user through
	// so PromoteFirstAdmin can fire in the message handler
	if admins := c.platformAdmins(platform); admins != nil && len(*admins) == 0 {
		return true
	}

	// Open mode = everyone allowed
	if c.Access.Mode == "open" {
		return true
	}

	pa := c.getPlatformAccess(platform)
	if pa == nil {
		return false
	}
	return c.isAllowedPlatform(pa, userID, chatID, isGroup)
}

// PlatformAccess holds the common access control fields for any platform.
type PlatformAccess struct {
	Enabled      bool
	Admins       []string
	AllowedUsers []string
	AllowedChats []string
	AllowGroups  bool
	AllowDMs     bool
}

// GetPlatformAccess returns a read-only snapshot of access fields for a platform (thread-safe).
func (c *Config) GetPlatformAccess(platform string) *PlatformAccess {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.getPlatformAccess(platform)
}

// getPlatformAccess returns access fields for a platform (caller must hold mu).
func (c *Config) getPlatformAccess(platform string) *PlatformAccess {
	switch platform {
	case "telegram":
		if c.Platforms.Telegram != nil {
			return &PlatformAccess{
				Enabled:      c.Platforms.Telegram.Enabled,
				Admins:       c.Platforms.Telegram.Admins,
				AllowedUsers: c.Platforms.Telegram.AllowedUsers,
				AllowedChats: c.Platforms.Telegram.AllowedChats,
				AllowGroups:  c.Platforms.Telegram.AllowGroups,
				AllowDMs:     c.Platforms.Telegram.AllowDMs,
			}
		}
	case "discord":
		if c.Platforms.Discord != nil {
			return &PlatformAccess{
				Enabled:      c.Platforms.Discord.Enabled,
				Admins:       c.Platforms.Discord.Admins,
				AllowedUsers: c.Platforms.Discord.AllowedUsers,
				AllowedChats: c.Platforms.Discord.AllowedChats,
				AllowGroups:  c.Platforms.Discord.AllowGroups,
				AllowDMs:     c.Platforms.Discord.AllowDMs,
			}
		}
	case "slack":
		if c.Platforms.Slack != nil {
			return &PlatformAccess{
				Enabled:      c.Platforms.Slack.Enabled,
				Admins:       c.Platforms.Slack.Admins,
				AllowedUsers: c.Platforms.Slack.AllowedUsers,
				AllowedChats: c.Platforms.Slack.AllowedChats,
				AllowGroups:  c.Platforms.Slack.AllowGroups,
				AllowDMs:     c.Platforms.Slack.AllowDMs,
			}
		}
	case "whatsapp":
		if c.Platforms.WhatsApp != nil {
			return &PlatformAccess{
				Enabled:      c.Platforms.WhatsApp.Enabled,
				Admins:       c.Platforms.WhatsApp.Admins,
				AllowedUsers: c.Platforms.WhatsApp.AllowedUsers,
				AllowedChats: c.Platforms.WhatsApp.AllowedChats,
				AllowGroups:  c.Platforms.WhatsApp.AllowGroups,
				AllowDMs:     c.Platforms.WhatsApp.AllowDMs,
			}
		}
	}
	return nil
}

func (c *Config) isAllowedPlatform(pa *PlatformAccess, userID, chatID string, isGroup bool) bool {
	if !pa.Enabled {
		return false
	}
	if isGroup && !pa.AllowGroups {
		return false
	}
	if !isGroup && !pa.AllowDMs {
		return false
	}
	if util.Contains(pa.Admins, userID) {
		return true
	}
	userOK := util.Contains(pa.AllowedUsers, userID)
	if len(pa.AllowedUsers) == 0 {
		userOK = c.Access.Mode != "allowlist"
	}
	chatOK := !isGroup || util.Contains(pa.AllowedChats, chatID)
	if isGroup && len(pa.AllowedChats) == 0 {
		chatOK = c.Access.Mode != "allowlist"
	}
	return userOK && chatOK
}

// LoadHooksFile reads hooks from a separate YAML file.
// Returns nil, nil if the file does not exist.
func LoadHooksFile(filePath string) ([]HookConfig, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read hooks file: %w", err)
	}

	expanded := expandEnvVars(string(data))

	var hf HooksFile
	if err := yaml.Unmarshal([]byte(expanded), &hf); err != nil {
		return nil, fmt.Errorf("parse hooks file: %w", err)
	}

	return hf.Hooks, nil
}

// SaveHooksFile writes hooks to a separate YAML file atomically.
func SaveHooksFile(filePath string, hooks []HookConfig) error {
	hf := HooksFile{Hooks: hooks}
	data, err := yaml.Marshal(&hf)
	if err != nil {
		return fmt.Errorf("marshal hooks: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0700); err != nil {
		return fmt.Errorf("create hooks dir: %w", err)
	}

	tmpFile := filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return fmt.Errorf("write hooks file: %w", err)
	}

	if err := os.Rename(tmpFile, filePath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("save hooks file: %w", err)
	}

	return nil
}

// GetPersona returns a persona by name, or nil if not found.
func (c *Config) GetPersona(name string) *Persona {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for i := range c.Personas.List {
		if c.Personas.List[i].Name == name {
			return &c.Personas.List[i]
		}
	}
	return nil
}

// GetDefaultPersona returns the default persona.
// Falls back to the first persona in the list, or nil if no personas are configured.
func (c *Config) GetDefaultPersona() *Persona {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.Personas.Default != "" {
		for i := range c.Personas.List {
			if c.Personas.List[i].Name == c.Personas.Default {
				return &c.Personas.List[i]
			}
		}
	}
	if len(c.Personas.List) > 0 {
		return &c.Personas.List[0]
	}
	return nil
}

// PatchYAMLField updates a single field in the config YAML file using
// yaml.Node to preserve comments, formatting, and env var references.
// Path is dot-separated (e.g., "llm.anthropic.model").
// An empty value removes the key from the file.
func (c *Config) PatchYAMLField(path, value string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := os.ReadFile(c.filePath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return fmt.Errorf("invalid YAML document")
	}

	parts := strings.Split(path, ".")
	if err := patchYAMLNode(doc.Content[0], parts, value); err != nil {
		return fmt.Errorf("patch %s: %w", path, err)
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	_ = enc.Close()

	// Write atomically
	tmpFile := c.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, buf.Bytes(), 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmpFile, c.filePath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("save config: %w", err)
	}

	return nil
}

// patchYAMLNode navigates the yaml.Node tree and sets or removes a value.
func patchYAMLNode(node *yaml.Node, path []string, value string) error {
	if len(path) == 0 || node.Kind != yaml.MappingNode {
		return fmt.Errorf("invalid path or node type")
	}

	key := path[0]
	rest := path[1:]

	// Search for existing key in mapping (content alternates key, value)
	for i := 0; i < len(node.Content)-1; i += 2 {
		if node.Content[i].Value == key {
			if len(rest) == 0 {
				if value == "" {
					// Remove key-value pair
					node.Content = append(node.Content[:i], node.Content[i+2:]...)
					return nil
				}
				node.Content[i+1] = &yaml.Node{
					Kind:  yaml.ScalarNode,
					Value: value,
					Tag:   "!!str",
				}
				return nil
			}
			return patchYAMLNode(node.Content[i+1], rest, value)
		}
	}

	// Key not found — add it
	if value == "" {
		return nil // nothing to remove
	}

	if len(rest) == 0 {
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: key},
			&yaml.Node{Kind: yaml.ScalarNode, Value: value},
		)
		return nil
	}

	// Create intermediate mapping
	newMap := &yaml.Node{Kind: yaml.MappingNode}
	node.Content = append(node.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key},
		newMap,
	)
	return patchYAMLNode(newMap, rest, value)
}

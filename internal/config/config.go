// Package config provides unified configuration management
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

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
	Agents AgentConfig `yaml:"agents"`

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
	Telegram  *TelegramConfig  `yaml:"telegram,omitempty"`
	Discord   *DiscordConfig   `yaml:"discord,omitempty"`
	Slack     *SlackConfig     `yaml:"slack,omitempty"`
	WhatsApp  *WhatsAppConfig  `yaml:"whatsapp,omitempty"`
	Webhook   *WebhookConfig   `yaml:"webhook,omitempty"`
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
}

// WhatsAppConfig for WhatsApp platform
type WhatsAppConfig struct {
	Enabled      bool     `yaml:"enabled"`
	DBPath       string   `yaml:"db_path"` // SQLite database for whatsmeow session
	Admins       []string `yaml:"admins"`
	AllowedUsers []string `yaml:"allowed_users"`
	AllowedChats []string `yaml:"allowed_chats"`
	AllowGroups  bool     `yaml:"allow_groups"`
	AllowDMs     bool     `yaml:"allow_dms"`
}

// WebhookConfig for generic webhook
type WebhookConfig struct {
	Enabled     bool     `yaml:"enabled"`
	Port        int      `yaml:"port"`
	Path        string   `yaml:"path"`
	Bind        string   `yaml:"bind"`
	Secret      string   `yaml:"secret,omitempty"`
	AuthMethod  string   `yaml:"auth_method"` // bearer, hmac, none
	BearerToken string   `yaml:"bearer_token,omitempty"`
	HMACSecret  string   `yaml:"hmac_secret,omitempty"`
	Admins      []string `yaml:"admins"`
	AllowedIPs  []string `yaml:"allowed_ips"`
}

// LLMConfig holds LLM provider settings
type LLMConfig struct {
	Default         string          `yaml:"default"`          // Default provider
	DefaultProvider string          `yaml:"default_provider"` // Alias for default
	Providers       ProvidersConfig `yaml:"providers"`        // Alternative structure
	FallbackChain   []string        `yaml:"fallback_chain"`
	SystemPrompt    string          `yaml:"system_prompt"`
	MaxInputLength  int             `yaml:"max_input_length"`
	Timeout         int             `yaml:"timeout"` // seconds
	RateLimit       int             `yaml:"rate_limit"`

	// Direct provider configs (preferred structure)
	Anthropic LLMProviderConfig `yaml:"anthropic"`
	OpenAI    LLMProviderConfig `yaml:"openai"`
	Gemini    LLMProviderConfig `yaml:"gemini"`
	GLM       LLMProviderConfig `yaml:"glm"`
	DeepSeek  LLMProviderConfig `yaml:"deepseek"`
	Local     LLMProviderConfig `yaml:"local"` // Self-hosted (Ollama, vLLM, llama.cpp, etc.)
}

// LLMProviderConfig holds config for a single LLM provider
type LLMProviderConfig struct {
	Enabled     bool    `yaml:"enabled"`
	APIKey      string  `yaml:"api_key"`
	Model       string  `yaml:"model"`
	MaxTokens   int     `yaml:"max_tokens"`
	Temperature float64 `yaml:"temperature"`
	BaseURL     string  `yaml:"base_url,omitempty"`
}

// ProvidersConfig holds individual LLM provider configs (alternative structure)
type ProvidersConfig struct {
	Anthropic *LLMProviderConfig `yaml:"anthropic,omitempty"`
	OpenAI    *LLMProviderConfig `yaml:"openai,omitempty"`
	Gemini    *LLMProviderConfig `yaml:"gemini,omitempty"`
	GLM       *LLMProviderConfig `yaml:"glm,omitempty"`
	DeepSeek  *LLMProviderConfig `yaml:"deepseek,omitempty"`
}

// AccessConfig holds global access settings
type AccessConfig struct {
	Mode         string   `yaml:"mode"` // allowlist, denylist, open
	GlobalAdmins []string `yaml:"global_admins"`
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
	MaxHistory   int           `yaml:"max_history"`   // Max messages per session
	TaskTimeout  time.Duration `yaml:"task_timeout"`  // Timeout for background tasks
	CleanupAge   time.Duration `yaml:"cleanup_age"`   // When to cleanup old sessions
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
	DataDir      string `yaml:"data_dir"`      // Base data directory (default: ~/data/magabot)
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
	Event     string   `yaml:"event"`                // pre_message, post_response, on_command, on_start, on_stop, on_error
	Command   string   `yaml:"command"`
	Timeout   int      `yaml:"timeout,omitempty"`     // seconds, default 10
	Platforms []string `yaml:"platforms,omitempty"`    // empty = all platforms
	Async     bool     `yaml:"async,omitempty"`        // fire-and-forget
}

// AgentConfig holds coding agent session settings
type AgentConfig struct {
	Default string `yaml:"default"` // "claude", "codex", "gemini"
	Timeout int    `yaml:"timeout"` // seconds, default 120
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
		c.Paths.DataDir = filepath.Join(home, "data", "magabot")
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
	if c.Agents.Default == "" {
		c.Agents.Default = "claude"
	}
	if c.Agents.Timeout <= 0 {
		c.Agents.Timeout = 120
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

// Save writes config to file
func (c *Config) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.LastUpdated = time.Now()

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
		os.Remove(tmpFile)
		return fmt.Errorf("failed to save config: %w", err)
	}

	return nil
}

// SaveBy saves config with updater info
func (c *Config) SaveBy(updatedBy string) error {
	c.UpdatedBy = updatedBy
	return c.Save()
}

// IsGlobalAdmin checks if user is a global admin
func (c *Config) IsGlobalAdmin(userID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.isGlobalAdmin(userID)
}

// isGlobalAdmin is the lock-free internal version (caller must hold mu)
func (c *Config) isGlobalAdmin(userID string) bool {
	for _, admin := range c.Access.GlobalAdmins {
		if admin == userID {
			return true
		}
	}
	return false
}

// IsPlatformAdmin checks if user is an admin for a specific platform
func (c *Config) IsPlatformAdmin(platform, userID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.isPlatformAdmin(platform, userID)
}

// isPlatformAdmin is the lock-free internal version (caller must hold mu)
func (c *Config) isPlatformAdmin(platform, userID string) bool {
	if c.isGlobalAdmin(userID) {
		return true
	}

	switch platform {
	case "telegram":
		if c.Platforms.Telegram != nil {
			return contains(c.Platforms.Telegram.Admins, userID)
		}
	case "discord":
		if c.Platforms.Discord != nil {
			return contains(c.Platforms.Discord.Admins, userID)
		}
	case "slack":
		if c.Platforms.Slack != nil {
			return contains(c.Platforms.Slack.Admins, userID)
		}
	case "whatsapp":
		if c.Platforms.WhatsApp != nil {
			return contains(c.Platforms.WhatsApp.Admins, userID)
		}
	}
	return false
}

// IsAllowed checks if user/chat is allowed on a platform
func (c *Config) IsAllowed(platform, userID, chatID string, isGroup bool) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Global admins always allowed
	if c.isGlobalAdmin(userID) {
		return true
	}

	// Open mode = everyone allowed
	if c.Access.Mode == "open" {
		return true
	}

	var pa *platformAccess
	switch platform {
	case "telegram":
		if c.Platforms.Telegram != nil {
			pa = &platformAccess{
				Enabled: c.Platforms.Telegram.Enabled, Admins: c.Platforms.Telegram.Admins,
				AllowedUsers: c.Platforms.Telegram.AllowedUsers, AllowedChats: c.Platforms.Telegram.AllowedChats,
				AllowGroups: c.Platforms.Telegram.AllowGroups, AllowDMs: c.Platforms.Telegram.AllowDMs,
			}
		}
	case "discord":
		if c.Platforms.Discord != nil {
			pa = &platformAccess{
				Enabled: c.Platforms.Discord.Enabled, Admins: c.Platforms.Discord.Admins,
				AllowedUsers: c.Platforms.Discord.AllowedUsers, AllowedChats: c.Platforms.Discord.AllowedChats,
				AllowGroups: c.Platforms.Discord.AllowGroups, AllowDMs: c.Platforms.Discord.AllowDMs,
			}
		}
	case "slack":
		if c.Platforms.Slack != nil {
			pa = &platformAccess{
				Enabled: c.Platforms.Slack.Enabled, Admins: c.Platforms.Slack.Admins,
				AllowedUsers: c.Platforms.Slack.AllowedUsers, AllowedChats: c.Platforms.Slack.AllowedChats,
				AllowGroups: c.Platforms.Slack.AllowGroups, AllowDMs: c.Platforms.Slack.AllowDMs,
			}
		}
	case "whatsapp":
		if c.Platforms.WhatsApp != nil {
			pa = &platformAccess{
				Enabled: c.Platforms.WhatsApp.Enabled, Admins: c.Platforms.WhatsApp.Admins,
				AllowedUsers: c.Platforms.WhatsApp.AllowedUsers, AllowedChats: c.Platforms.WhatsApp.AllowedChats,
				AllowGroups: c.Platforms.WhatsApp.AllowGroups, AllowDMs: c.Platforms.WhatsApp.AllowDMs,
			}
		}
	}
	if pa == nil {
		return false
	}
	return c.isAllowedPlatform(pa, userID, chatID, isGroup)
}

// platformAccess holds the common access control fields for any platform
type platformAccess struct {
	Enabled      bool
	Admins       []string
	AllowedUsers []string
	AllowedChats []string
	AllowGroups  bool
	AllowDMs     bool
}

func (c *Config) isAllowedPlatform(pa *platformAccess, userID, chatID string, isGroup bool) bool {
	if !pa.Enabled {
		return false
	}
	if isGroup && !pa.AllowGroups {
		return false
	}
	if !isGroup && !pa.AllowDMs {
		return false
	}
	if contains(pa.Admins, userID) {
		return true
	}
	userOK := contains(pa.AllowedUsers, userID)
	if len(pa.AllowedUsers) == 0 {
		userOK = c.Access.Mode != "allowlist"
	}
	chatOK := !isGroup || contains(pa.AllowedChats, chatID)
	if isGroup && len(pa.AllowedChats) == 0 {
		chatOK = c.Access.Mode != "allowlist"
	}
	return userOK && chatOK
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func remove(slice []string, item string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	return result
}

func addUnique(slice []string, item string) []string {
	if !contains(slice, item) {
		return append(slice, item)
	}
	return slice
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
		os.Remove(tmpFile)
		return fmt.Errorf("save hooks file: %w", err)
	}

	return nil
}

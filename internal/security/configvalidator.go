// Package security - Config validation for secure defaults (A05 fix)
package security

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ConfigIssue represents a security configuration issue
type ConfigIssue struct {
	Severity    string // critical, warning, info
	Category    string // permissions, secrets, access, etc.
	Description string
	Suggestion  string
}

// ConfigValidator validates security configuration
type ConfigValidator struct {
	issues []ConfigIssue
}

// NewConfigValidator creates a new config validator
func NewConfigValidator() *ConfigValidator {
	return &ConfigValidator{
		issues: make([]ConfigIssue, 0),
	}
}

// ValidateAll runs all security validations
func (v *ConfigValidator) ValidateAll(configPath, dataDir string, cfg *SecurityConfig) []ConfigIssue {
	v.issues = make([]ConfigIssue, 0)

	v.validateFilePermissions(configPath, dataDir)
	v.validateSecrets(cfg)
	v.validateAccessControl(cfg)
	v.validateEncryption(cfg)
	v.validateRateLimits(cfg)

	return v.issues
}

// SecurityConfig represents security-related configuration
type SecurityConfig struct {
	EncryptionKey     string
	AccessMode        string   // allowlist, denylist, open
	GlobalAdmins      []string
	PlatformAdmins    map[string][]string
	AllowedUsers      map[string][]string
	RateLimitMessages int
	RateLimitCommands int
	HasTelegramToken  bool
	HasDiscordToken   bool
	HasSlackToken     bool
	HasLLMKey         bool
}

func (v *ConfigValidator) addIssue(severity, category, description, suggestion string) {
	v.issues = append(v.issues, ConfigIssue{
		Severity:    severity,
		Category:    category,
		Description: description,
		Suggestion:  suggestion,
	})
}

func (v *ConfigValidator) validateFilePermissions(configPath, dataDir string) {
	// Check config file permissions
	if info, err := os.Stat(configPath); err == nil {
		mode := info.Mode().Perm()
		if mode&0077 != 0 { // Check if group/other have any access
			v.addIssue(
				"critical",
				"permissions",
				fmt.Sprintf("Config file has insecure permissions: %o", mode),
				fmt.Sprintf("Run: chmod 600 %s", configPath),
			)
		}
	}

	// Check data directory permissions
	if info, err := os.Stat(dataDir); err == nil {
		mode := info.Mode().Perm()
		if mode&0077 != 0 {
			v.addIssue(
				"critical",
				"permissions",
				fmt.Sprintf("Data directory has insecure permissions: %o", mode),
				fmt.Sprintf("Run: chmod 700 %s", dataDir),
			)
		}
	}

	// Check database file if exists
	dbPath := filepath.Join(dataDir, "magabot.db")
	if info, err := os.Stat(dbPath); err == nil {
		mode := info.Mode().Perm()
		if mode&0077 != 0 {
			v.addIssue(
				"critical",
				"permissions",
				fmt.Sprintf("Database file has insecure permissions: %o", mode),
				fmt.Sprintf("Run: chmod 600 %s", dbPath),
			)
		}
	}
}

func (v *ConfigValidator) validateSecrets(cfg *SecurityConfig) {
	// Check encryption key
	if cfg.EncryptionKey == "" {
		v.addIssue(
			"critical",
			"secrets",
			"No encryption key configured",
			"Run: magabot genkey",
		)
	} else if len(cfg.EncryptionKey) < 32 {
		v.addIssue(
			"critical",
			"secrets",
			"Encryption key too short",
			"Run: magabot genkey (generates 256-bit key)",
		)
	}

	// Check for weak/common patterns in key (base64 of zeros, etc.)
	if cfg.EncryptionKey != "" {
		weakPatterns := []string{
			"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", // All zeros
			"////////////////////////////////////////////////////////////////", // All ones
		}
		for _, pattern := range weakPatterns {
			if cfg.EncryptionKey == pattern {
				v.addIssue(
					"critical",
					"secrets",
					"Encryption key appears to be weak/default",
					"Run: magabot genkey",
				)
				break
			}
		}
	}

	// Check for at least one platform token
	if !cfg.HasTelegramToken && !cfg.HasDiscordToken && !cfg.HasSlackToken {
		v.addIssue(
			"warning",
			"secrets",
			"No platform tokens configured",
			"Configure at least one platform in config.yaml",
		)
	}

	// Check for LLM key
	if !cfg.HasLLMKey {
		v.addIssue(
			"warning",
			"secrets",
			"No LLM API key configured",
			"Add an LLM provider key (Anthropic, OpenAI, etc.)",
		)
	}
}

func (v *ConfigValidator) validateAccessControl(cfg *SecurityConfig) {
	// Check access mode
	if cfg.AccessMode == "open" {
		v.addIssue(
			"critical",
			"access",
			"Access mode is 'open' - anyone can use the bot",
			"Change to 'allowlist' mode for production",
		)
	}

	// Check for global admins
	if len(cfg.GlobalAdmins) == 0 {
		v.addIssue(
			"critical",
			"access",
			"No global admins configured",
			"Run: magabot config admin add YOUR_USER_ID",
		)
	}

	// Check if allowlist is empty in allowlist mode
	if cfg.AccessMode == "allowlist" {
		totalAllowed := len(cfg.GlobalAdmins)
		for _, users := range cfg.AllowedUsers {
			totalAllowed += len(users)
		}
		if totalAllowed == 0 {
			v.addIssue(
				"warning",
				"access",
				"Allowlist mode but no users allowed",
				"Add users: magabot config allow user USER_ID",
			)
		}
	}

	// Warn about admin in multiple places (redundant but ok)
	for _, globalAdmin := range cfg.GlobalAdmins {
		for platform, admins := range cfg.PlatformAdmins {
			for _, admin := range admins {
				if admin == globalAdmin {
					v.addIssue(
						"info",
						"access",
						fmt.Sprintf("User %s is both global and %s admin (redundant)", maskID(globalAdmin), platform),
						"Global admin already has full access",
					)
				}
			}
		}
	}
}

func (v *ConfigValidator) validateEncryption(cfg *SecurityConfig) {
	// Encryption key validation is done in validateSecrets
	// Additional checks could go here (e.g., key rotation reminders)
}

func (v *ConfigValidator) validateRateLimits(cfg *SecurityConfig) {
	// Check rate limits are reasonable
	if cfg.RateLimitMessages <= 0 {
		v.addIssue(
			"warning",
			"ratelimit",
			"Message rate limit not configured",
			"Set rate_limit.messages_per_minute in config (recommended: 30)",
		)
	} else if cfg.RateLimitMessages > 100 {
		v.addIssue(
			"warning",
			"ratelimit",
			fmt.Sprintf("Message rate limit very high: %d/min", cfg.RateLimitMessages),
			"Consider lowering to prevent abuse",
		)
	}

	if cfg.RateLimitCommands <= 0 {
		v.addIssue(
			"warning",
			"ratelimit",
			"Command rate limit not configured",
			"Set rate_limit.commands_per_minute in config (recommended: 10)",
		)
	} else if cfg.RateLimitCommands > 30 {
		v.addIssue(
			"warning",
			"ratelimit",
			fmt.Sprintf("Command rate limit very high: %d/min", cfg.RateLimitCommands),
			"Consider lowering to prevent abuse",
		)
	}
}

// maskID masks a user ID for logging (show first 3 and last 2 chars)
func maskID(id string) string {
	if len(id) <= 5 {
		return "***"
	}
	return id[:3] + "***" + id[len(id)-2:]
}

// HasCriticalIssues returns true if any critical issues exist
func (v *ConfigValidator) HasCriticalIssues() bool {
	for _, issue := range v.issues {
		if issue.Severity == "critical" {
			return true
		}
	}
	return false
}

// FormatIssues returns a formatted string of all issues
func (v *ConfigValidator) FormatIssues() string {
	if len(v.issues) == 0 {
		return "✅ No security issues found"
	}

	var sb strings.Builder
	
	criticals := 0
	warnings := 0
	infos := 0
	
	for _, issue := range v.issues {
		switch issue.Severity {
		case "critical":
			criticals++
		case "warning":
			warnings++
		case "info":
			infos++
		}
	}

	sb.WriteString(fmt.Sprintf("🔐 Security Check: %d critical, %d warnings, %d info\n\n",
		criticals, warnings, infos))

	for _, issue := range v.issues {
		var icon string
		switch issue.Severity {
		case "critical":
			icon = "🔴"
		case "warning":
			icon = "🟡"
		case "info":
			icon = "🔵"
		}

		sb.WriteString(fmt.Sprintf("%s [%s] %s\n", icon, issue.Category, issue.Description))
		sb.WriteString(fmt.Sprintf("   💡 %s\n\n", issue.Suggestion))
	}

	return sb.String()
}

// FixPermissions attempts to fix file permission issues
func FixPermissions(configPath, dataDir string) error {
	// Fix config file
	if err := os.Chmod(configPath, 0600); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("chmod config: %w", err)
	}

	// Fix data directory
	if err := os.Chmod(dataDir, 0700); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("chmod data dir: %w", err)
	}

	// Fix database file
	dbPath := filepath.Join(dataDir, "magabot.db")
	if err := os.Chmod(dbPath, 0600); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("chmod database: %w", err)
	}

	// Fix log directory (ignore errors — log dir may not exist yet)
	logDir := filepath.Join(dataDir, "..", "logs")
	_ = os.Chmod(logDir, 0700)

	return nil
}

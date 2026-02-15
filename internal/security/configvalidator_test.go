package security

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewConfigValidator(t *testing.T) {
	v := NewConfigValidator()
	if v == nil {
		t.Fatal("ConfigValidator should not be nil")
	}
	if v.issues == nil {
		t.Error("issues should be initialized")
	}
}

func TestConfigValidatorValidateAll(t *testing.T) {
	v := NewConfigValidator()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(configPath, []byte("test: true"), 0644)

	cfg := &SecurityConfig{
		EncryptionKey:     GenerateKey(),
		AccessMode:        "allowlist",
		GlobalAdmins:      []string{"admin1"},
		RateLimitMessages: 30,
		RateLimitCommands: 10,
		HasTelegramToken:  true,
		HasLLMKey:         true,
	}

	issues := v.ValidateAll(configPath, dir, cfg)
	// Might have permission warnings, but shouldn't panic
	_ = issues
}

func TestConfigValidatorFilePermissions(t *testing.T) {
	dir := t.TempDir()

	t.Run("InsecureConfigPermissions", func(t *testing.T) {
		v := NewConfigValidator()
		configPath := filepath.Join(dir, "config.yaml")
		os.WriteFile(configPath, []byte("test"), 0644) // world readable

		cfg := &SecurityConfig{EncryptionKey: GenerateKey()}
		issues := v.ValidateAll(configPath, dir, cfg)

		hasPermIssue := false
		for _, issue := range issues {
			if issue.Category == "permissions" {
				hasPermIssue = true
				break
			}
		}
		if !hasPermIssue {
			t.Error("Should detect insecure file permissions")
		}
	})

	t.Run("SecureConfigPermissions", func(t *testing.T) {
		configPath := filepath.Join(dir, "secure_config.yaml")
		os.WriteFile(configPath, []byte("test"), 0600)
		os.Chmod(dir, 0700)

		v2 := NewConfigValidator()
		cfg := &SecurityConfig{
			EncryptionKey:     GenerateKey(),
			AccessMode:        "allowlist",
			GlobalAdmins:      []string{"admin"},
			RateLimitMessages: 30,
			RateLimitCommands: 10,
			HasTelegramToken:  true,
			HasLLMKey:         true,
		}
		issues := v2.ValidateAll(configPath, dir, cfg)

		for _, issue := range issues {
			if issue.Category == "permissions" && issue.Severity == "critical" {
				// May still get warnings depending on system umask
				if strings.Contains(issue.Description, "Config file") {
					t.Error("Should not have config file permission issue")
				}
			}
		}
	})
}

func TestConfigValidatorSecrets(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(configPath, []byte("test"), 0600)
	os.Chmod(dir, 0700)

	t.Run("NoEncryptionKey", func(t *testing.T) {
		v2 := NewConfigValidator()
		cfg := &SecurityConfig{
			EncryptionKey: "",
			GlobalAdmins:  []string{"admin"},
		}
		issues := v2.ValidateAll(configPath, dir, cfg)

		hasKeyIssue := false
		for _, issue := range issues {
			if issue.Category == "secrets" && strings.Contains(issue.Description, "encryption key") {
				hasKeyIssue = true
				break
			}
		}
		if !hasKeyIssue {
			t.Error("Should detect missing encryption key")
		}
	})

	t.Run("ShortEncryptionKey", func(t *testing.T) {
		v2 := NewConfigValidator()
		cfg := &SecurityConfig{
			EncryptionKey: "short",
			GlobalAdmins:  []string{"admin"},
		}
		issues := v2.ValidateAll(configPath, dir, cfg)

		hasKeyIssue := false
		for _, issue := range issues {
			if issue.Category == "secrets" && strings.Contains(issue.Description, "too short") {
				hasKeyIssue = true
				break
			}
		}
		if !hasKeyIssue {
			t.Error("Should detect short encryption key")
		}
	})

	t.Run("WeakEncryptionKey", func(t *testing.T) {
		v2 := NewConfigValidator()
		cfg := &SecurityConfig{
			EncryptionKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", // All zeros
			GlobalAdmins:  []string{"admin"},
		}
		issues := v2.ValidateAll(configPath, dir, cfg)

		hasKeyIssue := false
		for _, issue := range issues {
			if issue.Category == "secrets" && strings.Contains(issue.Description, "weak") {
				hasKeyIssue = true
				break
			}
		}
		if !hasKeyIssue {
			t.Error("Should detect weak encryption key")
		}
	})

	t.Run("NoPlatformTokens", func(t *testing.T) {
		v2 := NewConfigValidator()
		cfg := &SecurityConfig{
			EncryptionKey:    GenerateKey(),
			GlobalAdmins:     []string{"admin"},
			HasTelegramToken: false,
			HasDiscordToken:  false,
			HasSlackToken:    false,
		}
		issues := v2.ValidateAll(configPath, dir, cfg)

		hasTokenIssue := false
		for _, issue := range issues {
			if issue.Category == "secrets" && strings.Contains(issue.Description, "platform tokens") {
				hasTokenIssue = true
				break
			}
		}
		if !hasTokenIssue {
			t.Error("Should detect missing platform tokens")
		}
	})

	t.Run("NoLLMKey", func(t *testing.T) {
		v2 := NewConfigValidator()
		cfg := &SecurityConfig{
			EncryptionKey:    GenerateKey(),
			GlobalAdmins:     []string{"admin"},
			HasTelegramToken: true,
			HasLLMKey:        false,
		}
		issues := v2.ValidateAll(configPath, dir, cfg)

		hasLLMIssue := false
		for _, issue := range issues {
			if issue.Category == "secrets" && strings.Contains(issue.Description, "LLM API") {
				hasLLMIssue = true
				break
			}
		}
		if !hasLLMIssue {
			t.Error("Should detect missing LLM key")
		}
	})
}

func TestConfigValidatorAccessControl(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(configPath, []byte("test"), 0600)
	os.Chmod(dir, 0700)

	t.Run("OpenAccessMode", func(t *testing.T) {
		v2 := NewConfigValidator()
		cfg := &SecurityConfig{
			EncryptionKey:    GenerateKey(),
			AccessMode:       "open",
			GlobalAdmins:     []string{"admin"},
			HasTelegramToken: true,
			HasLLMKey:        true,
		}
		issues := v2.ValidateAll(configPath, dir, cfg)

		hasAccessIssue := false
		for _, issue := range issues {
			if issue.Category == "access" && strings.Contains(issue.Description, "open") {
				hasAccessIssue = true
				break
			}
		}
		if !hasAccessIssue {
			t.Error("Should warn about open access mode")
		}
	})

	t.Run("NoGlobalAdmins", func(t *testing.T) {
		v2 := NewConfigValidator()
		cfg := &SecurityConfig{
			EncryptionKey:    GenerateKey(),
			AccessMode:       "allowlist",
			GlobalAdmins:     []string{},
			HasTelegramToken: true,
			HasLLMKey:        true,
		}
		issues := v2.ValidateAll(configPath, dir, cfg)

		hasAdminIssue := false
		for _, issue := range issues {
			if issue.Category == "access" && strings.Contains(issue.Description, "global admins") {
				hasAdminIssue = true
				break
			}
		}
		if !hasAdminIssue {
			t.Error("Should detect no global admins")
		}
	})

	t.Run("EmptyAllowlistMode", func(t *testing.T) {
		v2 := NewConfigValidator()
		cfg := &SecurityConfig{
			EncryptionKey:    GenerateKey(),
			AccessMode:       "allowlist",
			GlobalAdmins:     []string{},
			AllowedUsers:     map[string][]string{},
			HasTelegramToken: true,
			HasLLMKey:        true,
		}
		issues := v2.ValidateAll(configPath, dir, cfg)

		hasAllowlistIssue := false
		for _, issue := range issues {
			if strings.Contains(issue.Description, "no users allowed") {
				hasAllowlistIssue = true
				break
			}
		}
		if !hasAllowlistIssue {
			t.Error("Should warn about empty allowlist")
		}
	})

	t.Run("RedundantAdmin", func(t *testing.T) {
		v2 := NewConfigValidator()
		cfg := &SecurityConfig{
			EncryptionKey: GenerateKey(),
			AccessMode:    "allowlist",
			GlobalAdmins:  []string{"admin1"},
			PlatformAdmins: map[string][]string{
				"telegram": {"admin1"}, // Same as global admin
			},
			HasTelegramToken: true,
			HasLLMKey:        true,
		}
		issues := v2.ValidateAll(configPath, dir, cfg)

		hasInfoIssue := false
		for _, issue := range issues {
			if issue.Severity == "info" && strings.Contains(issue.Description, "redundant") {
				hasInfoIssue = true
				break
			}
		}
		if !hasInfoIssue {
			t.Error("Should note redundant admin")
		}
	})
}

func TestConfigValidatorRateLimits(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(configPath, []byte("test"), 0600)
	os.Chmod(dir, 0700)

	t.Run("NoRateLimits", func(t *testing.T) {
		v2 := NewConfigValidator()
		cfg := &SecurityConfig{
			EncryptionKey:     GenerateKey(),
			GlobalAdmins:      []string{"admin"},
			RateLimitMessages: 0,
			RateLimitCommands: 0,
			HasTelegramToken:  true,
			HasLLMKey:         true,
		}
		issues := v2.ValidateAll(configPath, dir, cfg)

		hasRateLimitIssue := false
		for _, issue := range issues {
			if issue.Category == "ratelimit" {
				hasRateLimitIssue = true
				break
			}
		}
		if !hasRateLimitIssue {
			t.Error("Should warn about no rate limits")
		}
	})

	t.Run("HighRateLimits", func(t *testing.T) {
		v2 := NewConfigValidator()
		cfg := &SecurityConfig{
			EncryptionKey:     GenerateKey(),
			GlobalAdmins:      []string{"admin"},
			RateLimitMessages: 200,
			RateLimitCommands: 50,
			HasTelegramToken:  true,
			HasLLMKey:         true,
		}
		issues := v2.ValidateAll(configPath, dir, cfg)

		hasHighRateIssue := false
		for _, issue := range issues {
			if issue.Category == "ratelimit" && strings.Contains(issue.Description, "very high") {
				hasHighRateIssue = true
				break
			}
		}
		if !hasHighRateIssue {
			t.Error("Should warn about high rate limits")
		}
	})
}

func TestMaskID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"12345678", "123***78"},
		{"abcd", "***"},
		{"ab", "***"},
		{"", "***"},
		{"123456789012", "123***12"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := maskID(tt.input)
			if result != tt.expected {
				t.Errorf("maskID(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestHasCriticalIssues(t *testing.T) {
	t.Run("NoCritical", func(t *testing.T) {
		v := NewConfigValidator()
		v.addIssue("warning", "test", "desc", "suggestion")
		v.addIssue("info", "test", "desc", "suggestion")

		if v.HasCriticalIssues() {
			t.Error("Should not have critical issues")
		}
	})

	t.Run("HasCritical", func(t *testing.T) {
		v := NewConfigValidator()
		v.addIssue("warning", "test", "desc", "suggestion")
		v.addIssue("critical", "test", "desc", "suggestion")

		if !v.HasCriticalIssues() {
			t.Error("Should have critical issues")
		}
	})
}

func TestFormatIssues(t *testing.T) {
	t.Run("NoIssues", func(t *testing.T) {
		v := NewConfigValidator()
		result := v.FormatIssues()
		if !strings.Contains(result, "No security issues") {
			t.Error("Should show no issues message")
		}
	})

	t.Run("WithIssues", func(t *testing.T) {
		v := NewConfigValidator()
		v.addIssue("critical", "secrets", "Missing key", "Add key")
		v.addIssue("warning", "access", "Open mode", "Change mode")
		v.addIssue("info", "other", "FYI", "Note")

		result := v.FormatIssues()
		if !strings.Contains(result, "1 critical") {
			t.Error("Should show critical count")
		}
		if !strings.Contains(result, "1 warnings") {
			t.Error("Should show warning count")
		}
		if !strings.Contains(result, "🔴") {
			t.Error("Should have critical icon")
		}
		if !strings.Contains(result, "🟡") {
			t.Error("Should have warning icon")
		}
		if !strings.Contains(result, "🔵") {
			t.Error("Should have info icon")
		}
	})
}

func TestFixPermissions(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	dbPath := filepath.Join(dir, "magabot.db")

	// Create files with wrong permissions
	os.WriteFile(configPath, []byte("test"), 0644)
	os.WriteFile(dbPath, []byte("test"), 0644)

	err := FixPermissions(configPath, dir)
	if err != nil {
		t.Fatalf("FixPermissions failed: %v", err)
	}

	// Check permissions were fixed
	info, _ := os.Stat(configPath)
	if info.Mode().Perm() != 0600 {
		t.Errorf("Config should be 0600, got %o", info.Mode().Perm())
	}

	info, _ = os.Stat(dbPath)
	if info.Mode().Perm() != 0600 {
		t.Errorf("DB should be 0600, got %o", info.Mode().Perm())
	}

	info, _ = os.Stat(dir)
	if info.Mode().Perm() != 0700 {
		t.Errorf("Dir should be 0700, got %o", info.Mode().Perm())
	}
}

func TestFixPermissionsNonexistent(t *testing.T) {
	// Should not error for nonexistent files
	err := FixPermissions("/nonexistent/config.yaml", "/nonexistent/dir")
	if err != nil {
		t.Errorf("Should not error for nonexistent files: %v", err)
	}
}

func TestConfigIssue(t *testing.T) {
	issue := ConfigIssue{
		Severity:    "critical",
		Category:    "secrets",
		Description: "Missing key",
		Suggestion:  "Add key",
	}

	if issue.Severity != "critical" {
		t.Error("Severity mismatch")
	}
	if issue.Category != "secrets" {
		t.Error("Category mismatch")
	}
	if issue.Description != "Missing key" {
		t.Error("Description mismatch")
	}
	if issue.Suggestion != "Add key" {
		t.Error("Suggestion mismatch")
	}
}

func TestSecurityConfig(t *testing.T) {
	cfg := SecurityConfig{
		EncryptionKey:     "key",
		AccessMode:        "allowlist",
		GlobalAdmins:      []string{"admin"},
		PlatformAdmins:    map[string][]string{"telegram": {"padmin"}},
		AllowedUsers:      map[string][]string{"telegram": {"user1"}},
		RateLimitMessages: 30,
		RateLimitCommands: 10,
		HasTelegramToken:  true,
		HasDiscordToken:   false,
		HasSlackToken:     false,
		HasLLMKey:         true,
	}

	if cfg.EncryptionKey != "key" {
		t.Error("EncryptionKey mismatch")
	}
	if cfg.AccessMode != "allowlist" {
		t.Error("AccessMode mismatch")
	}
	if len(cfg.GlobalAdmins) != 1 {
		t.Error("GlobalAdmins mismatch")
	}
}

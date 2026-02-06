package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigLoad(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "magabot-config-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")

	// Test loading non-existent config (should return defaults)
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load should not fail for non-existent config: %v", err)
	}

	// Check defaults
	if cfg.Access.Mode != "allowlist" {
		t.Errorf("Default mode should be 'allowlist', got %s", cfg.Access.Mode)
	}
	if cfg.Bot.Prefix != "/" {
		t.Errorf("Default prefix should be '/', got %s", cfg.Bot.Prefix)
	}
}

func TestConfigSave(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "magabot-config-test-*")
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")

	cfg, _ := Load(configPath)
	cfg.Bot.Name = "TestBot"
	cfg.Access.GlobalAdmins = []string{"123456"}

	err := cfg.Save()
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Reload and verify
	cfg2, err := Load(configPath)
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	if cfg2.Bot.Name != "TestBot" {
		t.Errorf("Expected bot name 'TestBot', got %s", cfg2.Bot.Name)
	}
	if len(cfg2.Access.GlobalAdmins) != 1 || cfg2.Access.GlobalAdmins[0] != "123456" {
		t.Error("Global admins not saved correctly")
	}
}

func TestIsGlobalAdmin(t *testing.T) {
	cfg := &Config{
		Access: AccessConfig{
			GlobalAdmins: []string{"admin1", "admin2"},
		},
	}

	if !cfg.IsGlobalAdmin("admin1") {
		t.Error("admin1 should be global admin")
	}
	if !cfg.IsGlobalAdmin("admin2") {
		t.Error("admin2 should be global admin")
	}
	if cfg.IsGlobalAdmin("user1") {
		t.Error("user1 should not be global admin")
	}
}

func TestIsPlatformAdmin(t *testing.T) {
	cfg := &Config{
		Access: AccessConfig{
			GlobalAdmins: []string{"global_admin"},
		},
		Platforms: PlatformsConfig{
			Telegram: &TelegramConfig{
				Enabled: true,
				Admins:  []string{"tg_admin"},
			},
		},
	}

	// Global admin is admin everywhere
	if !cfg.IsPlatformAdmin("telegram", "global_admin") {
		t.Error("Global admin should be platform admin")
	}

	// Platform admin
	if !cfg.IsPlatformAdmin("telegram", "tg_admin") {
		t.Error("tg_admin should be telegram admin")
	}

	// Regular user
	if cfg.IsPlatformAdmin("telegram", "user1") {
		t.Error("user1 should not be platform admin")
	}

	// Wrong platform
	if cfg.IsPlatformAdmin("discord", "tg_admin") {
		t.Error("tg_admin should not be discord admin")
	}
}

func TestIsAllowed(t *testing.T) {
	cfg := &Config{
		Access: AccessConfig{
			Mode:         "allowlist",
			GlobalAdmins: []string{"global_admin"},
		},
		Platforms: PlatformsConfig{
			Telegram: &TelegramConfig{
				Enabled:      true,
				Admins:       []string{"tg_admin"},
				AllowedUsers: []string{"allowed_user"},
				AllowedChats: []string{"-100123"},
				AllowGroups:  true,
				AllowDMs:     true,
			},
		},
	}

	// Global admin always allowed
	if !cfg.IsAllowed("telegram", "global_admin", "", false) {
		t.Error("Global admin should be allowed")
	}

	// Platform admin always allowed
	if !cfg.IsAllowed("telegram", "tg_admin", "", false) {
		t.Error("Platform admin should be allowed")
	}

	// Allowed user
	if !cfg.IsAllowed("telegram", "allowed_user", "", false) {
		t.Error("Allowed user should be allowed")
	}

	// Not allowed user
	if cfg.IsAllowed("telegram", "random_user", "", false) {
		t.Error("Random user should not be allowed in allowlist mode")
	}

	// Group chat with allowed chat
	if !cfg.IsAllowed("telegram", "allowed_user", "-100123", true) {
		t.Error("User in allowed chat should be allowed")
	}
}

func TestIsAllowedOpenMode(t *testing.T) {
	cfg := &Config{
		Access: AccessConfig{
			Mode: "open",
		},
		Platforms: PlatformsConfig{
			Telegram: &TelegramConfig{
				Enabled:     true,
				AllowGroups: true,
				AllowDMs:    true,
			},
		},
	}

	// Anyone should be allowed in open mode
	if !cfg.IsAllowed("telegram", "random_user", "", false) {
		t.Error("Anyone should be allowed in open mode")
	}
}

func TestContains(t *testing.T) {
	slice := []string{"a", "b", "c"}

	if !contains(slice, "b") {
		t.Error("Should find 'b'")
	}
	if contains(slice, "d") {
		t.Error("Should not find 'd'")
	}
}

func TestRemove(t *testing.T) {
	slice := []string{"a", "b", "c"}
	result := remove(slice, "b")

	if len(result) != 2 {
		t.Errorf("Expected 2 elements, got %d", len(result))
	}
	if contains(result, "b") {
		t.Error("Should not contain 'b'")
	}
}

func TestAddUnique(t *testing.T) {
	slice := []string{"a", "b"}

	// Add new
	result := addUnique(slice, "c")
	if len(result) != 3 {
		t.Errorf("Expected 3 elements, got %d", len(result))
	}

	// Add duplicate
	result2 := addUnique(slice, "a")
	if len(result2) != 2 {
		t.Errorf("Expected 2 elements (no duplicate), got %d", len(result2))
	}
}

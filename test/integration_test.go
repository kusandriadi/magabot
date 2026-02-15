// Package test contains integration tests
package test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kusa/magabot/internal/config"
	"github.com/kusa/magabot/internal/cron"
	"github.com/kusa/magabot/internal/memory"
	"github.com/kusa/magabot/internal/session"
)

// TestFullWorkflow tests a complete user workflow
func TestFullWorkflow(t *testing.T) {
	// Setup
	tmpDir, err := os.MkdirTemp("", "magabot-integration-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")

	// 1. Create and configure
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	cfg.Bot.Name = "IntegrationBot"
	cfg.Access.GlobalAdmins = []string{"admin123"}
	cfg.Platforms.Telegram = &config.TelegramConfig{
		Enabled:      true,
		Admins:       []string{"admin123"},
		AllowedUsers: []string{"user456"},
		AllowGroups:  true,
		AllowDMs:     true,
	}

	if err := cfg.Save(); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// 2. Verify access control
	if !cfg.IsGlobalAdmin("admin123") {
		t.Error("admin123 should be global admin")
	}

	if !cfg.IsAllowed("telegram", "user456", "", false) {
		t.Error("user456 should be allowed")
	}

	if cfg.IsAllowed("telegram", "random", "", false) {
		t.Error("random should not be allowed")
	}

	// 3. Test memory
	memStore, err := memory.NewStore(tmpDir, "user456")
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}

	_, err = memStore.Remember("I like trading stocks", "telegram")
	if err != nil {
		t.Fatalf("Failed to remember: %v", err)
	}

	results := memStore.Search("trading", 5)
	if len(results) == 0 {
		t.Error("Should find memory about trading")
	}

	// 4. Test cron jobs
	cronStore, err := cron.NewJobStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create cron store: %v", err)
	}

	job := &cron.Job{
		Name:     "Morning Alert",
		Schedule: "0 9 * * *",
		Message:  "Good morning!",
		Channels: []cron.NotifyChannel{
			{Type: "telegram", Target: "user456"},
		},
		Enabled: true,
	}

	if err := cronStore.Create(job); err != nil {
		t.Fatalf("Failed to create job: %v", err)
	}

	jobs := cronStore.ListEnabled()
	if len(jobs) != 1 {
		t.Errorf("Expected 1 enabled job, got %d", len(jobs))
	}

	t.Log("Integration test passed!")
}

// TestSessionWorkflow tests session management
func TestSessionWorkflow(t *testing.T) {
	notify := func(platform, chatID, message string) error {
		return nil
	}

	manager := session.NewManager(notify, 50, nil)

	// Create session
	sess := manager.GetOrCreate("telegram", "chat123", "user456")
	if sess == nil {
		t.Fatal("Failed to create session")
	}

	// Add messages
	manager.AddMessage(sess, "user", "Hello")
	manager.AddMessage(sess, "assistant", "Hi there!")

	history := manager.GetHistory(sess, 10)
	if len(history) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(history))
	}

	// Test context
	manager.SetContext(sess, "last_topic", "greetings")
	topic := manager.GetContext(sess, "last_topic")
	if topic != "greetings" {
		t.Errorf("Expected 'greetings', got %v", topic)
	}

	// List sessions
	sessions := manager.List("user456", false)
	if len(sessions) != 1 {
		t.Errorf("Expected 1 session, got %d", len(sessions))
	}
}

// TestConfigAdminOperations tests admin operations
func TestConfigAdminOperations(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "magabot-admin-*")
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.yaml")
	cfg, _ := config.Load(configPath)

	// Setup initial admin
	cfg.Access.GlobalAdmins = []string{"admin1"}
	cfg.Platforms.Telegram = &config.TelegramConfig{
		Enabled:      true,
		Admins:       []string{"admin1"},
		AllowedUsers: []string{"user1", "newadmin"}, // Pre-allow newadmin
		AllowGroups:  true,
		AllowDMs:     true,
	}
	_ = cfg.Save()

	// Test AllowUser (as admin)
	result := cfg.AllowUser("telegram", "admin1", "user2")
	if !result.Success {
		t.Errorf("AllowUser should succeed: %s", result.Message)
	}

	// Verify user is allowed (check the actual platform config)
	found := false
	for _, u := range cfg.Platforms.Telegram.AllowedUsers {
		if u == "user2" {
			found = true
			break
		}
	}
	if !found {
		t.Error("user2 should be in allowed users list")
	}

	// Test AddPlatformAdmin (newadmin already in allowlist)
	result = cfg.AddPlatformAdmin("telegram", "admin1", "newadmin")
	if !result.Success {
		t.Errorf("AddPlatformAdmin should succeed: %s", result.Message)
	}

	// Verify new admin
	if !cfg.IsPlatformAdmin("telegram", "newadmin") {
		t.Error("newadmin should be platform admin")
	}

	// Test unauthorized operation
	result = cfg.AllowUser("telegram", "user1", "user3")
	if result.Success {
		t.Error("Non-admin should not be able to modify allowlist")
	}
}

// TestConcurrentAccess tests thread safety
func TestConcurrentAccess(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "magabot-concurrent-*")
	defer os.RemoveAll(tmpDir)

	memStore, _ := memory.NewStore(tmpDir, "concurrent-user")

	// Concurrent writes
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(n int) {
			_, _ = memStore.Remember("Concurrent memory "+string(rune('0'+n)), "test")
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all memories saved
	memories := memStore.List("")
	if len(memories) != 10 {
		t.Errorf("Expected 10 memories, got %d", len(memories))
	}
}

// TestTimeouts tests timeout handling
func TestTimeouts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// Simulate a slow operation
	select {
	case <-ctx.Done():
		// Expected timeout
	case <-time.After(1 * time.Second):
		t.Error("Context should have timed out")
	}
}

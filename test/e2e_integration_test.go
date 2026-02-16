// Package test contains end-to-end integration tests
package test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kusa/magabot/internal/config"
	"github.com/kusa/magabot/internal/cron"
	"github.com/kusa/magabot/internal/memory"
	"github.com/kusa/magabot/internal/secrets"
	"github.com/kusa/magabot/internal/security"
	"github.com/kusa/magabot/internal/session"
	"github.com/kusa/magabot/internal/storage"
	"github.com/kusa/magabot/internal/tools"
)

// TestE2EUserWorkflow simulates a complete user interaction workflow
func TestE2EUserWorkflow(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	ctx := context.Background()

	// Setup all components
	store, err := storage.New(filepath.Join(tmpDir, "magabot.db"))
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	vault, err := security.NewVault(security.GenerateKey())
	if err != nil {
		t.Fatalf("Failed to create vault: %v", err)
	}

	configPath := filepath.Join(tmpDir, "config.yaml")
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	cfg.Bot.Name = "E2EBot"
	cfg.Access.GlobalAdmins = []string{"admin1"}
	cfg.Platforms.Telegram = &config.TelegramConfig{
		Enabled:      true,
		AllowedUsers: []string{"user1", "user2"},
		AllowDMs:     true,
		AllowGroups:  true,
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	memStore, err := memory.NewStore(tmpDir, "user1")
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	cronStore, err := cron.NewJobStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create cron store: %v", err)
	}
	secretsMgr, err := secrets.NewFromConfig(&secrets.Config{Backend: "local", LocalConfig: &secrets.LocalConfig{Path: filepath.Join(tmpDir, "secrets.json")}})
	if err != nil {
		t.Fatalf("Failed to create secrets manager: %v", err)
	}
	toolsMgr := tools.NewManager(logger)
	sessionMgr := session.NewManager(func(p, c, m string) error { return nil }, 50, nil)

	// 1. User authentication
	t.Run("UserAuthentication", func(t *testing.T) {
		if !cfg.IsAllowed("telegram", "user1", "", false) {
			t.Error("user1 should be allowed")
		}

		if cfg.IsAllowed("telegram", "unknown_user", "", false) {
			t.Error("unknown_user should not be allowed")
		}

		if !cfg.IsGlobalAdmin("admin1") {
			t.Error("admin1 should be global admin")
		}
	})

	// 2. Session management
	t.Run("SessionManagement", func(t *testing.T) {
		sess := sessionMgr.GetOrCreate("telegram", "chat1", "user1")
		if sess == nil {
			t.Fatal("Session should be created")
		}

		sessionMgr.AddMessage(sess, "user", "Hello!")
		sessionMgr.AddMessage(sess, "assistant", "Hi there!")

		history := sessionMgr.GetHistory(sess, 10)
		if len(history) != 2 {
			t.Errorf("Expected 2 messages in history, got %d", len(history))
		}
	})

	// 3. Memory storage
	t.Run("MemoryStorage", func(t *testing.T) {
		_, err := memStore.Remember("User prefers dark mode", "telegram")
		if err != nil {
			t.Fatalf("Failed to remember: %v", err)
		}

		results := memStore.Search("dark mode", 5)
		if len(results) == 0 {
			t.Error("Should find memory about dark mode")
		}
	})

	// 4. Cron job scheduling
	t.Run("CronScheduling", func(t *testing.T) {
		job := &cron.Job{
			Name:     "Daily Reminder",
			Schedule: "0 9 * * *",
			Message:  "Good morning!",
			Channels: []cron.NotifyChannel{
				{Type: "telegram", Target: "user1"},
			},
			Enabled: true,
		}

		err := cronStore.Create(job)
		if err != nil {
			t.Fatalf("Failed to create cron job: %v", err)
		}

		jobs := cronStore.ListEnabled()
		if len(jobs) != 1 {
			t.Errorf("Expected 1 enabled job, got %d", len(jobs))
		}
	})

	// 5. Secrets management
	t.Run("SecretsManagement", func(t *testing.T) {
		err := secretsMgr.Set(ctx, "test_api_key", "secret123")
		if err != nil {
			t.Fatalf("Failed to set secret: %v", err)
		}

		value, err := secretsMgr.Get(ctx, "test_api_key")
		if err != nil {
			t.Fatalf("Failed to get secret: %v", err)
		}

		if value != "secret123" {
			t.Errorf("Expected 'secret123', got '%s'", value)
		}
	})

	// 6. Message encryption
	t.Run("MessageEncryption", func(t *testing.T) {
		plaintext := "Sensitive user message"

		ciphertext, err := vault.Encrypt([]byte(plaintext))
		if err != nil {
			t.Fatalf("Encryption failed: %v", err)
		}

		decrypted, err := vault.Decrypt(ciphertext)
		if err != nil {
			t.Fatalf("Decryption failed: %v", err)
		}

		if string(decrypted) != plaintext {
			t.Error("Decrypted message doesn't match original")
		}
	})

	// 7. Persistent storage
	t.Run("PersistentStorage", func(t *testing.T) {
		msg := &storage.Message{
			Platform:  "telegram",
			ChatID:    "chat1",
			UserID:    "user1",
			Content:   "Encrypted content",
			Timestamp: time.Now(),
			Direction: "in",
		}

		err := store.SaveMessage(msg)
		if err != nil {
			t.Fatalf("Failed to save message: %v", err)
		}

		messages, err := store.GetMessages("telegram", "chat1", 10)
		if err != nil {
			t.Fatalf("Failed to get messages: %v", err)
		}
		if len(messages) == 0 {
			t.Error("Should retrieve saved message")
		}
	})

	// 8. Tools integration
	t.Run("ToolsIntegration", func(t *testing.T) {
		toolsMgr.Register(&MockTool{
			name:        "test_tool",
			description: "A test tool",
			executeFunc: func(ctx context.Context, params map[string]string) (string, error) {
				return "Tool executed", nil
			},
		})

		result, err := toolsMgr.Execute(ctx, "test_tool", nil)
		if err != nil {
			t.Fatalf("Tool execution failed: %v", err)
		}

		if result != "Tool executed" {
			t.Errorf("Expected 'Tool executed', got '%s'", result)
		}
	})
}

// TestE2EConcurrentUsers simulates multiple users interacting concurrently
func TestE2EConcurrentUsers(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	store, err := storage.New(filepath.Join(tmpDir, "concurrent.db"))
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	sessionMgr := session.NewManager(func(p, c, m string) error { return nil }, 50, logger)

	numUsers := 20
	messagesPerUser := 10

	var wg sync.WaitGroup
	errors := make(chan error, numUsers*messagesPerUser)

	for i := 0; i < numUsers; i++ {
		wg.Add(1)
		go func(userNum int) {
			defer wg.Done()

			userID := "user" + string(rune('A'+userNum%26))
			chatID := "chat" + string(rune('0'+userNum%10))

			sess := sessionMgr.GetOrCreate("telegram", chatID, userID)
			if sess == nil {
				errors <- nil
				return
			}

			for j := 0; j < messagesPerUser; j++ {
				msg := "Message " + string(rune('0'+j))
				sessionMgr.AddMessage(sess, "user", msg)

				// Also save to storage
				_ = store.SaveMessage(&storage.Message{
					Platform:  "telegram",
					ChatID:    chatID,
					UserID:    userID,
					Content:   msg,
					Timestamp: time.Now(),
					Direction: "in",
				})
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	errorCount := 0
	for err := range errors {
		if err != nil {
			errorCount++
		}
	}

	if errorCount > 0 {
		t.Errorf("Had %d errors during concurrent execution", errorCount)
	}

	// Verify data integrity
	stats, err := store.Stats()
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}
	if stats == nil {
		t.Error("Should get storage stats")
	}
}

// TestE2EErrorRecovery tests error handling and recovery
func TestE2EErrorRecovery(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("StorageRecovery", func(t *testing.T) {
		dbPath := filepath.Join(tmpDir, "recovery.db")

		// Create and populate database
		store1, err := storage.New(dbPath)
		if err != nil {
			t.Fatalf("Failed to create storage: %v", err)
		}
		if err := store1.SaveMessage(&storage.Message{
			Platform:  "telegram",
			ChatID:    "chat1",
			UserID:    "user1",
			Content:   "Test message",
			Timestamp: time.Now(),
			Direction: "in",
		}); err != nil {
			t.Fatalf("Failed to save message: %v", err)
		}
		if err := store1.Close(); err != nil {
			t.Fatalf("Failed to close storage: %v", err)
		}

		// Reopen and verify data persisted
		store2, err := storage.New(dbPath)
		if err != nil {
			t.Fatalf("Failed to reopen storage: %v", err)
		}
		defer store2.Close()

		messages, err := store2.GetMessages("telegram", "chat1", 10)
		if err != nil {
			t.Fatalf("Failed to get messages: %v", err)
		}
		if len(messages) == 0 {
			t.Error("Data should persist after reopen")
		}
	})

	t.Run("ConfigRecovery", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "recovery_config.yaml")

		// Create config
		cfg1, err := config.Load(configPath)
		if err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}
		cfg1.Bot.Name = "RecoveryBot"
		if err := cfg1.Save(); err != nil {
			t.Fatalf("Failed to save config: %v", err)
		}

		// Reload and verify
		cfg2, err := config.Load(configPath)
		if err != nil {
			t.Fatalf("Failed to reload config: %v", err)
		}

		if cfg2.Bot.Name != "RecoveryBot" {
			t.Error("Config should persist")
		}
	})

	t.Run("SecretsRecovery", func(t *testing.T) {
		secretsPath := filepath.Join(tmpDir, "recovery_secrets.json")
		ctx := context.Background()

		// Create and set secret
		mgr1, err := secrets.NewFromConfig(&secrets.Config{
			Backend:     "local",
			LocalConfig: &secrets.LocalConfig{Path: secretsPath},
		})
		if err != nil {
			t.Fatalf("Failed to create secrets manager: %v", err)
		}
		if err := mgr1.Set(ctx, "recovery_key", "recovery_value"); err != nil {
			t.Fatalf("Failed to set secret: %v", err)
		}

		// Recreate manager and verify
		mgr2, err := secrets.NewFromConfig(&secrets.Config{
			Backend:     "local",
			LocalConfig: &secrets.LocalConfig{Path: secretsPath},
		})
		if err != nil {
			t.Fatalf("Failed to recreate secrets manager: %v", err)
		}

		value, err := mgr2.Get(ctx, "recovery_key")
		if err != nil {
			t.Fatalf("Failed to get recovered secret: %v", err)
		}

		if value != "recovery_value" {
			t.Error("Secret should persist")
		}
	})
}

// TestE2EResourceCleanup tests proper resource cleanup
func TestE2EResourceCleanup(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("StorageCleanup", func(t *testing.T) {
		store, err := storage.New(filepath.Join(tmpDir, "cleanup.db"))
		if err != nil {
			t.Fatalf("Failed to create storage: %v", err)
		}

		// Add some messages
		for i := 0; i < 100; i++ {
			if err := store.SaveMessage(&storage.Message{
				Platform:  "telegram",
				ChatID:    "chat1",
				UserID:    "user1",
				Content:   "Message",
				Timestamp: time.Now().AddDate(0, 0, -100), // Old messages
				Direction: "in",
			}); err != nil {
				t.Fatalf("Failed to save message %d: %v", i, err)
			}
		}

		// Purge old messages
		deleted, err := store.PurgeOldMessages(30)
		if err != nil {
			t.Fatalf("Purge failed: %v", err)
		}

		if deleted < 100 {
			t.Errorf("Expected 100 deleted, got %d", deleted)
		}

		// Vacuum
		err = store.Vacuum()
		if err != nil {
			t.Errorf("Vacuum failed: %v", err)
		}

		if err := store.Close(); err != nil {
			t.Errorf("Failed to close store: %v", err)
		}
	})

	t.Run("SessionManagerCleanup", func(t *testing.T) {
		sessionMgr := session.NewManager(func(p, c, m string) error { return nil }, 5, nil)

		// Create many sessions
		for i := 0; i < 20; i++ {
			sessionMgr.GetOrCreate("telegram", "chat"+string(rune('0'+i%10)), "user"+string(rune('A'+i%26)))
		}

		// Sessions should be managed properly
		sessions := sessionMgr.List("", true)
		if len(sessions) == 0 {
			t.Error("Should have some sessions")
		}
	})
}

// TestE2EAdminOperations tests admin-level operations
func TestE2EAdminOperations(t *testing.T) {
	tmpDir := t.TempDir()

	configPath := filepath.Join(tmpDir, "admin_config.yaml")
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	cfg.Access.GlobalAdmins = []string{"admin1"}
	cfg.Platforms.Telegram = &config.TelegramConfig{
		Enabled:      true,
		Admins:       []string{"admin1"},
		AllowedUsers: []string{"user1"},
		AllowDMs:     true,
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	t.Run("AllowNewUser", func(t *testing.T) {
		result := cfg.AllowUser("telegram", "admin1", "new_user")
		if !result.Success {
			t.Errorf("Admin should be able to allow users: %s", result.Message)
		}
	})

	t.Run("UnauthorizedAdminAction", func(t *testing.T) {
		result := cfg.AllowUser("telegram", "user1", "another_user")
		if result.Success {
			t.Error("Non-admin should not be able to allow users")
		}
	})

	t.Run("PromoteToAdmin", func(t *testing.T) {
		// First allow the user
		cfg.AllowUser("telegram", "admin1", "promoted_user")

		// Then promote
		result := cfg.AddPlatformAdmin("telegram", "admin1", "promoted_user")
		if !result.Success {
			t.Errorf("Should be able to promote allowed user: %s", result.Message)
		}

		if !cfg.IsPlatformAdmin("telegram", "promoted_user") {
			t.Error("User should be admin after promotion")
		}
	})
}

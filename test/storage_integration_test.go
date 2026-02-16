// Package test contains integration tests for storage module
package test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/kusa/magabot/internal/storage"
)

func TestStorageIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := storage.New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	t.Run("SaveAndRetrieveMessages", func(t *testing.T) {
		msg := &storage.Message{
			Platform:  "telegram",
			ChatID:    "chat123",
			UserID:    "user456",
			Username:  "testuser",
			Content:   "Hello, world!",
			Timestamp: time.Now(),
			Direction: "in",
		}

		if err := store.SaveMessage(msg); err != nil {
			t.Fatalf("Failed to save message: %v", err)
		}

		messages, err := store.GetMessages("telegram", "chat123", 10)
		if err != nil {
			t.Fatalf("Failed to get messages: %v", err)
		}

		if len(messages) != 1 {
			t.Errorf("Expected 1 message, got %d", len(messages))
		}

		if messages[0].Content != "Hello, world!" {
			t.Errorf("Expected 'Hello, world!', got %s", messages[0].Content)
		}
	})

	t.Run("MultipleMessagesOrdering", func(t *testing.T) {
		// Add multiple messages
		for i := 0; i < 5; i++ {
			msg := &storage.Message{
				Platform:  "telegram",
				ChatID:    "chat_order",
				UserID:    "user",
				Content:   string(rune('A' + i)),
				Timestamp: time.Now().Add(time.Duration(i) * time.Second),
				Direction: "in",
			}
			if err := store.SaveMessage(msg); err != nil {
				t.Fatalf("Failed to save message %d: %v", i, err)
			}
		}

		messages, err := store.GetMessages("telegram", "chat_order", 10)
		if err != nil {
			t.Fatalf("Failed to get messages: %v", err)
		}
		if len(messages) != 5 {
			t.Errorf("Expected 5 messages, got %d", len(messages))
		}

		// Should be in reverse chronological order
		if messages[0].Content != "E" {
			t.Errorf("Expected latest message 'E', got %s", messages[0].Content)
		}
	})

	t.Run("SessionManagement", func(t *testing.T) {
		// Save session
		if err := store.SaveSession("whatsapp", "session_data_123"); err != nil {
			t.Fatalf("Failed to save session: %v", err)
		}

		// Retrieve session
		data, err := store.GetSession("whatsapp")
		if err != nil {
			t.Fatalf("Failed to get session: %v", err)
		}

		if data != "session_data_123" {
			t.Errorf("Expected 'session_data_123', got %s", data)
		}

		// Update session
		if err := store.SaveSession("whatsapp", "updated_session"); err != nil {
			t.Fatalf("Failed to update session: %v", err)
		}

		data, err = store.GetSession("whatsapp")
		if err != nil {
			t.Fatalf("Failed to get updated session: %v", err)
		}
		if data != "updated_session" {
			t.Errorf("Expected 'updated_session', got %s", data)
		}

		// Delete session
		if err := store.DeleteSession("whatsapp"); err != nil {
			t.Fatalf("Failed to delete session: %v", err)
		}

		data, err = store.GetSession("whatsapp")
		if err != nil {
			t.Fatalf("Failed to get deleted session: %v", err)
		}
		if data != "" {
			t.Errorf("Expected empty string after delete, got %s", data)
		}
	})

	t.Run("ConfigStorage", func(t *testing.T) {
		// Set config
		if err := store.SetConfig("api_key", "secret123"); err != nil {
			t.Fatalf("Failed to set config: %v", err)
		}

		// Get config
		value, err := store.GetConfig("api_key")
		if err != nil {
			t.Fatalf("Failed to get config: %v", err)
		}

		if value != "secret123" {
			t.Errorf("Expected 'secret123', got %s", value)
		}

		// Get non-existent config — returns empty string, no error
		value, err = store.GetConfig("nonexistent")
		if err != nil {
			t.Fatalf("GetConfig nonexistent should not error: %v", err)
		}
		if value != "" {
			t.Errorf("Expected empty for nonexistent key, got %s", value)
		}
	})

	t.Run("AuditLog", func(t *testing.T) {
		err := store.AuditLog("telegram", "user123", "login", "IP: 192.168.1.1")
		if err != nil {
			t.Errorf("Failed to write audit log: %v", err)
		}
	})

	t.Run("PurgeOldMessages", func(t *testing.T) {
		// Add old message
		oldMsg := &storage.Message{
			Platform:  "telegram",
			ChatID:    "purge_test",
			UserID:    "user",
			Content:   "old message",
			Timestamp: time.Now().AddDate(0, 0, -100), // 100 days ago
			Direction: "in",
		}
		if err := store.SaveMessage(oldMsg); err != nil {
			t.Fatalf("Failed to save old message: %v", err)
		}

		// Add recent message
		newMsg := &storage.Message{
			Platform:  "telegram",
			ChatID:    "purge_test",
			UserID:    "user",
			Content:   "new message",
			Timestamp: time.Now(),
			Direction: "in",
		}
		if err := store.SaveMessage(newMsg); err != nil {
			t.Fatalf("Failed to save new message: %v", err)
		}

		// Purge messages older than 30 days
		deleted, err := store.PurgeOldMessages(30)
		if err != nil {
			t.Fatalf("Failed to purge: %v", err)
		}

		if deleted < 1 {
			t.Errorf("Expected at least 1 deleted message, got %d", deleted)
		}

		// Verify new message still exists
		messages, err := store.GetMessages("telegram", "purge_test", 10)
		if err != nil {
			t.Fatalf("Failed to get messages after purge: %v", err)
		}
		found := false
		for _, m := range messages {
			if m.Content == "new message" {
				found = true
			}
		}
		if !found {
			t.Error("New message should not be purged")
		}
	})

	t.Run("Stats", func(t *testing.T) {
		stats, err := store.Stats()
		if err != nil {
			t.Fatalf("Failed to get stats: %v", err)
		}

		if _, ok := stats["messages"]; !ok {
			t.Error("Stats should include messages")
		}

		if _, ok := stats["db_size_bytes"]; !ok {
			t.Error("Stats should include db_size_bytes")
		}
	})

	t.Run("Vacuum", func(t *testing.T) {
		if err := store.Vacuum(); err != nil {
			t.Errorf("Vacuum failed: %v", err)
		}
	})
}

func TestStorageConcurrency(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "concurrent.db")

	store, err := storage.New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Concurrent writes
	done := make(chan bool, 20)
	for i := 0; i < 20; i++ {
		go func(n int) {
			msg := &storage.Message{
				Platform:  "telegram",
				ChatID:    "concurrent",
				UserID:    "user",
				Content:   string(rune('A' + n%26)),
				Timestamp: time.Now(),
				Direction: "in",
			}
			_ = store.SaveMessage(msg)
			done <- true
		}(i)
	}

	for i := 0; i < 20; i++ {
		<-done
	}

	// Verify all messages saved
	messages, err := store.GetMessages("telegram", "concurrent", 100)
	if err != nil {
		t.Fatalf("Failed to get messages: %v", err)
	}

	if len(messages) != 20 {
		t.Errorf("Expected 20 messages, got %d", len(messages))
	}
}

func TestStorageReopen(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "reopen.db")

	// Create and write
	store, err := storage.New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	if err := store.SetConfig("test_key", "test_value"); err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Failed to close store: %v", err)
	}

	// Reopen and verify
	store2, err := storage.New(dbPath)
	if err != nil {
		t.Fatalf("Failed to reopen: %v", err)
	}
	defer store2.Close()

	value, err := store2.GetConfig("test_key")
	if err != nil {
		t.Fatalf("Failed to get config: %v", err)
	}
	if value != "test_value" {
		t.Errorf("Expected 'test_value' after reopen, got %s", value)
	}
}

func TestStorageInvalidPath(t *testing.T) {
	_, err := storage.New("/nonexistent/path/to/db")
	if err == nil {
		t.Error("Expected error for invalid path")
	}
}

func TestStoragePlatformIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := storage.New(filepath.Join(tmpDir, "isolation.db"))
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Save messages for different platforms
	platforms := []string{"telegram", "whatsapp", "slack"}
	for _, p := range platforms {
		msg := &storage.Message{
			Platform:  p,
			ChatID:    "chat1",
			UserID:    "user1",
			Content:   "Message for " + p,
			Timestamp: time.Now(),
			Direction: "in",
		}
		if err := store.SaveMessage(msg); err != nil {
			t.Fatalf("Failed to save message for %s: %v", p, err)
		}
	}

	// Verify isolation
	for _, p := range platforms {
		messages, err := store.GetMessages(p, "chat1", 10)
		if err != nil {
			t.Fatalf("Failed to get messages for %s: %v", p, err)
		}
		if len(messages) != 1 {
			t.Errorf("Platform %s: expected 1 message, got %d", p, len(messages))
		}
		if messages[0].Content != "Message for "+p {
			t.Errorf("Platform %s: wrong content", p)
		}
	}
}

package storage_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/kusa/magabot/internal/storage"
	_ "github.com/mattn/go-sqlite3"
)

func newTestStore(t *testing.T) *storage.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := storage.New(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestNew_ValidPath(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "valid.db")
	store, err := storage.New(dbPath)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	defer store.Close()
}

func TestNew_InvalidPath(t *testing.T) {
	// Attempt to create a database in a path that does not exist and is not writable.
	_, err := storage.New("/nonexistent/dir/that/should/fail/test.db")
	if err == nil {
		t.Fatal("expected error for invalid path, got nil")
	}
}

func TestSaveAndGetMessages(t *testing.T) {
	store := newTestStore(t)

	msgs := []storage.Message{
		{
			Platform:  "telegram",
			ChatID:    "chat1",
			UserID:    "user1",
			Username:  "alice",
			Content:   "hello world",
			Timestamp: time.Now().Add(-2 * time.Second),
			Direction: "in",
		},
		{
			Platform:  "telegram",
			ChatID:    "chat1",
			UserID:    "bot",
			Username:  "magabot",
			Content:   "hi there",
			Timestamp: time.Now().Add(-1 * time.Second),
			Direction: "out",
		},
	}

	for i := range msgs {
		if err := store.SaveMessage(&msgs[i]); err != nil {
			t.Fatalf("SaveMessage(%d): %v", i, err)
		}
	}

	got, err := store.GetMessages("telegram", "chat1", 10)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(got))
	}

	// Results are ordered by timestamp DESC, so newest first.
	if got[0].Content != "hi there" {
		t.Errorf("expected newest message first, got %q", got[0].Content)
	}
	if got[1].Content != "hello world" {
		t.Errorf("expected oldest message second, got %q", got[1].Content)
	}
	if got[0].Direction != "out" {
		t.Errorf("expected direction 'out', got %q", got[0].Direction)
	}
	if got[1].Platform != "telegram" {
		t.Errorf("expected platform 'telegram', got %q", got[1].Platform)
	}
}

func TestGetMessages_Empty(t *testing.T) {
	store := newTestStore(t)

	got, err := store.GetMessages("telegram", "nonexistent", 10)
	if err != nil {
		t.Fatalf("GetMessages on empty table: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(got))
	}
}

func TestGetMessages_Limit(t *testing.T) {
	store := newTestStore(t)

	for i := 0; i < 5; i++ {
		msg := &storage.Message{
			Platform:  "slack",
			ChatID:    "ch1",
			UserID:    "u1",
			Username:  "bob",
			Content:   "msg",
			Timestamp: time.Now().Add(time.Duration(i) * time.Second),
			Direction: "in",
		}
		if err := store.SaveMessage(msg); err != nil {
			t.Fatalf("SaveMessage: %v", err)
		}
	}

	got, err := store.GetMessages("slack", "ch1", 3)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 messages with limit, got %d", len(got))
	}
}

func TestGetMessages_PlatformIsolation(t *testing.T) {
	store := newTestStore(t)

	for _, plat := range []string{"telegram", "slack"} {
		msg := &storage.Message{
			Platform:  plat,
			ChatID:    "chat1",
			UserID:    "u1",
			Username:  "u1",
			Content:   plat + " msg",
			Timestamp: time.Now(),
			Direction: "in",
		}
		if err := store.SaveMessage(msg); err != nil {
			t.Fatalf("SaveMessage: %v", err)
		}
	}

	got, err := store.GetMessages("telegram", "chat1", 10)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 telegram message, got %d", len(got))
	}
	if got[0].Content != "telegram msg" {
		t.Errorf("expected 'telegram msg', got %q", got[0].Content)
	}
}

func TestStats(t *testing.T) {
	store := newTestStore(t)

	// Insert messages across platforms.
	platforms := map[string]int{
		"telegram": 3,
		"slack":    2,
	}
	for plat, count := range platforms {
		for i := 0; i < count; i++ {
			msg := &storage.Message{
				Platform:  plat,
				ChatID:    "c1",
				UserID:    "u1",
				Username:  "user",
				Content:   "content",
				Timestamp: time.Now(),
				Direction: "in",
			}
			if err := store.SaveMessage(msg); err != nil {
				t.Fatalf("SaveMessage: %v", err)
			}
		}
	}

	stats, err := store.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	msgCounts, ok := stats["messages"].(map[string]int64)
	if !ok {
		t.Fatalf("expected messages to be map[string]int64, got %T", stats["messages"])
	}
	if msgCounts["telegram"] != 3 {
		t.Errorf("expected 3 telegram messages, got %d", msgCounts["telegram"])
	}
	if msgCounts["slack"] != 2 {
		t.Errorf("expected 2 slack messages, got %d", msgCounts["slack"])
	}

	sessionCount, ok := stats["sessions"].(int)
	if !ok {
		t.Fatalf("expected sessions to be int, got %T", stats["sessions"])
	}
	if sessionCount != 0 {
		t.Errorf("expected 0 sessions, got %d", sessionCount)
	}

	dbSize, ok := stats["db_size_bytes"].(int64)
	if !ok {
		t.Fatalf("expected db_size_bytes to be int64, got %T", stats["db_size_bytes"])
	}
	if dbSize <= 0 {
		t.Errorf("expected positive db size, got %d", dbSize)
	}
}

func TestStats_Empty(t *testing.T) {
	store := newTestStore(t)

	stats, err := store.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	msgCounts, ok := stats["messages"].(map[string]int64)
	if !ok {
		t.Fatalf("expected messages to be map[string]int64, got %T", stats["messages"])
	}
	if len(msgCounts) != 0 {
		t.Errorf("expected empty message counts, got %v", msgCounts)
	}
}

func TestPurgeOldMessages(t *testing.T) {
	store := newTestStore(t)

	// Insert an old message (31 days ago) and a new message (now).
	oldMsg := &storage.Message{
		Platform:  "telegram",
		ChatID:    "c1",
		UserID:    "u1",
		Username:  "user",
		Content:   "old message",
		Timestamp: time.Now().AddDate(0, 0, -31),
		Direction: "in",
	}
	newMsg := &storage.Message{
		Platform:  "telegram",
		ChatID:    "c1",
		UserID:    "u1",
		Username:  "user",
		Content:   "new message",
		Timestamp: time.Now(),
		Direction: "in",
	}

	if err := store.SaveMessage(oldMsg); err != nil {
		t.Fatalf("SaveMessage(old): %v", err)
	}
	if err := store.SaveMessage(newMsg); err != nil {
		t.Fatalf("SaveMessage(new): %v", err)
	}

	purged, err := store.PurgeOldMessages(30)
	if err != nil {
		t.Fatalf("PurgeOldMessages: %v", err)
	}
	if purged != 1 {
		t.Errorf("expected 1 purged message, got %d", purged)
	}

	remaining, err := store.GetMessages("telegram", "c1", 10)
	if err != nil {
		t.Fatalf("GetMessages after purge: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("expected 1 remaining message, got %d", len(remaining))
	}
	if remaining[0].Content != "new message" {
		t.Errorf("expected 'new message', got %q", remaining[0].Content)
	}
}

func TestPurgeOldMessages_ZeroDays(t *testing.T) {
	store := newTestStore(t)

	tests := []struct {
		name string
		days int
	}{
		{"zero", 0},
		{"negative", -5},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			purged, err := store.PurgeOldMessages(tc.days)
			if err != nil {
				t.Fatalf("PurgeOldMessages(%d): %v", tc.days, err)
			}
			if purged != 0 {
				t.Errorf("expected 0, got %d", purged)
			}
		})
	}
}

func TestSetGetConfig(t *testing.T) {
	store := newTestStore(t)

	tests := []struct {
		key   string
		value string
	}{
		{"theme", "dark"},
		{"language", "en"},
		{"empty_value", ""},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			if err := store.SetConfig(tc.key, tc.value); err != nil {
				t.Fatalf("SetConfig(%q, %q): %v", tc.key, tc.value, err)
			}
			got, err := store.GetConfig(tc.key)
			if err != nil {
				t.Fatalf("GetConfig(%q): %v", tc.key, err)
			}
			if got != tc.value {
				t.Errorf("GetConfig(%q) = %q, want %q", tc.key, got, tc.value)
			}
		})
	}
}

func TestSetConfig_Overwrite(t *testing.T) {
	store := newTestStore(t)

	if err := store.SetConfig("key1", "original"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if err := store.SetConfig("key1", "updated"); err != nil {
		t.Fatalf("SetConfig overwrite: %v", err)
	}

	got, err := store.GetConfig("key1")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if got != "updated" {
		t.Errorf("expected 'updated', got %q", got)
	}
}

func TestGetConfig_NotFound(t *testing.T) {
	store := newTestStore(t)

	value, err := store.GetConfig("nonexistent_key")
	if err != nil {
		t.Fatalf("expected nil error for missing config, got %v", err)
	}
	if value != "" {
		t.Errorf("expected empty string for missing config, got %q", value)
	}
}

func TestAuditLog(t *testing.T) {
	store := newTestStore(t)

	err := store.AuditLog("telegram", "user123", "login", "successful login")
	if err != nil {
		t.Fatalf("AuditLog: %v", err)
	}

	// Write multiple audit entries to verify no errors.
	entries := []struct {
		platform string
		userID   string
		action   string
		details  string
	}{
		{"slack", "u2", "message_send", "sent message to #general"},
		{"telegram", "u3", "command", "/help"},
		{"webhook", "", "health_check", ""},
	}

	for _, e := range entries {
		if err := store.AuditLog(e.platform, e.userID, e.action, e.details); err != nil {
			t.Fatalf("AuditLog(%q, %q, %q): %v", e.platform, e.userID, e.action, err)
		}
	}
}

func TestClose(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "close_test.db")
	store, err := storage.New(dbPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Operations after close should fail.
	if err := store.SaveMessage(&storage.Message{
		Platform:  "test",
		ChatID:    "c",
		UserID:    "u",
		Content:   "msg",
		Timestamp: time.Now(),
		Direction: "in",
	}); err == nil {
		t.Error("expected error after Close, got nil")
	}
}

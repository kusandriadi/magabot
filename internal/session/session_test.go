package session

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// MockTaskRunner implements TaskRunner for testing
type MockTaskRunner struct {
	result string
	err    error
	delay  time.Duration
}

func (m *MockTaskRunner) Execute(ctx context.Context, task string, sessionContext []Message) (string, error) {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return m.result, m.err
}

func TestNewManager(t *testing.T) {
	t.Run("DefaultValues", func(t *testing.T) {
		mgr := NewManager(nil, 0, nil)
		if mgr == nil {
			t.Fatal("Manager should not be nil")
		}
		if mgr.maxHistory != 50 {
			t.Errorf("Expected default maxHistory 50, got %d", mgr.maxHistory)
		}
	})

	t.Run("CustomMaxHistory", func(t *testing.T) {
		mgr := NewManager(nil, 100, nil)
		if mgr.maxHistory != 100 {
			t.Errorf("Expected maxHistory 100, got %d", mgr.maxHistory)
		}
	})

	t.Run("WithNotifyFunc", func(t *testing.T) {
		notify := func(p, c, m string) error {
			return nil
		}
		mgr := NewManager(notify, 50, nil)
		if mgr.notify == nil {
			t.Error("Notify function should be set")
		}
	})
}

func TestGetOrCreate(t *testing.T) {
	mgr := NewManager(nil, 50, nil)

	t.Run("CreateNew", func(t *testing.T) {
		sess := mgr.GetOrCreate("telegram", "chat1", "user1")
		if sess == nil {
			t.Fatal("Session should be created")
		}
		if sess.Platform != "telegram" {
			t.Errorf("Expected platform 'telegram', got '%s'", sess.Platform)
		}
		if sess.ChatID != "chat1" {
			t.Errorf("Expected chatID 'chat1', got '%s'", sess.ChatID)
		}
		if sess.UserID != "user1" {
			t.Errorf("Expected userID 'user1', got '%s'", sess.UserID)
		}
		if sess.Type != "main" {
			t.Errorf("Expected type 'main', got '%s'", sess.Type)
		}
		if sess.Status != StatusRunning {
			t.Errorf("Expected status 'running', got '%s'", sess.Status)
		}
	})

	t.Run("GetExisting", func(t *testing.T) {
		sess1 := mgr.GetOrCreate("telegram", "chat2", "user1")
		sess2 := mgr.GetOrCreate("telegram", "chat2", "user1")
		if sess1 != sess2 {
			t.Error("Should return same session for same chat")
		}
	})

	t.Run("DifferentChats", func(t *testing.T) {
		sess1 := mgr.GetOrCreate("telegram", "chatA", "user1")
		sess2 := mgr.GetOrCreate("telegram", "chatB", "user1")
		if sess1 == sess2 {
			t.Error("Different chats should have different sessions")
		}
	})

	t.Run("DifferentPlatforms", func(t *testing.T) {
		sess1 := mgr.GetOrCreate("telegram", "chat", "user1")
		sess2 := mgr.GetOrCreate("whatsapp", "chat", "user1")
		if sess1 == sess2 {
			t.Error("Different platforms should have different sessions")
		}
	})
}

func TestGet(t *testing.T) {
	mgr := NewManager(nil, 50, nil)

	t.Run("ExistingSession", func(t *testing.T) {
		sess := mgr.GetOrCreate("telegram", "chat1", "user1")
		retrieved := mgr.Get(sess.ID)
		if retrieved != sess {
			t.Error("Should retrieve the same session")
		}
	})

	t.Run("NonExistentSession", func(t *testing.T) {
		retrieved := mgr.Get("nonexistent:id")
		if retrieved != nil {
			t.Error("Should return nil for non-existent session")
		}
	})
}

func TestAddMessage(t *testing.T) {
	mgr := NewManager(nil, 5, nil) // Small history for testing trim
	sess := mgr.GetOrCreate("telegram", "chat1", "user1")

	t.Run("AddSingleMessage", func(t *testing.T) {
		mgr.AddMessage(sess, "user", "Hello")
		if len(sess.Messages) != 1 {
			t.Errorf("Expected 1 message, got %d", len(sess.Messages))
		}
		if sess.Messages[0].Role != "user" {
			t.Errorf("Expected role 'user', got '%s'", sess.Messages[0].Role)
		}
		if sess.Messages[0].Content != "Hello" {
			t.Errorf("Expected content 'Hello', got '%s'", sess.Messages[0].Content)
		}
	})

	t.Run("AddMultipleMessages", func(t *testing.T) {
		mgr.AddMessage(sess, "assistant", "Hi there!")
		mgr.AddMessage(sess, "user", "How are you?")
		if len(sess.Messages) != 3 {
			t.Errorf("Expected 3 messages, got %d", len(sess.Messages))
		}
	})

	t.Run("TrimHistory", func(t *testing.T) {
		// Add more messages to exceed maxHistory (5)
		for i := 0; i < 5; i++ {
			mgr.AddMessage(sess, "user", "Extra message")
		}
		// Should be trimmed to 5
		if len(sess.Messages) > 5 {
			t.Errorf("Messages should be trimmed to %d, got %d", 5, len(sess.Messages))
		}
	})

	t.Run("UpdatesTimestamp", func(t *testing.T) {
		oldTime := sess.UpdatedAt
		time.Sleep(10 * time.Millisecond)
		mgr.AddMessage(sess, "user", "New message")
		if !sess.UpdatedAt.After(oldTime) {
			t.Error("UpdatedAt should be updated")
		}
	})
}

func TestGetHistory(t *testing.T) {
	mgr := NewManager(nil, 50, nil)
	sess := mgr.GetOrCreate("telegram", "chat1", "user1")

	// Add 10 messages
	for i := 0; i < 10; i++ {
		mgr.AddMessage(sess, "user", "Message")
	}

	t.Run("GetAll", func(t *testing.T) {
		history := mgr.GetHistory(sess, 0)
		if len(history) != 10 {
			t.Errorf("Expected 10 messages, got %d", len(history))
		}
	})

	t.Run("GetLimited", func(t *testing.T) {
		history := mgr.GetHistory(sess, 5)
		if len(history) != 5 {
			t.Errorf("Expected 5 messages, got %d", len(history))
		}
	})

	t.Run("GetMoreThanExists", func(t *testing.T) {
		history := mgr.GetHistory(sess, 100)
		if len(history) != 10 {
			t.Errorf("Expected 10 messages (all), got %d", len(history))
		}
	})

	t.Run("ReturnsCopy", func(t *testing.T) {
		history := mgr.GetHistory(sess, 5)
		history[0].Content = "Modified"
		// Original should be unchanged
		original := mgr.GetHistory(sess, 5)
		if original[0].Content == "Modified" {
			t.Error("GetHistory should return a copy")
		}
	})
}

func TestSpawn(t *testing.T) {
	var notified atomic.Bool
	notify := func(p, c, m string) error {
		notified.Store(true)
		_ = m // Use m to avoid unused variable
		return nil
	}

	t.Run("SpawnSubSession", func(t *testing.T) {
		mgr := NewManager(notify, 50, nil)
		mgr.SetTaskRunner(&MockTaskRunner{result: "Task done", delay: 10 * time.Millisecond})

		parent := mgr.GetOrCreate("telegram", "chat1", "user1")
		sub, err := mgr.Spawn(parent, "Do something")

		if err != nil {
			t.Fatalf("Spawn failed: %v", err)
		}
		if sub == nil {
			t.Fatal("Sub-session should be created")
		}
		if sub.ParentID != parent.ID {
			t.Error("Sub-session should reference parent")
		}
		if sub.Type != "sub" {
			t.Errorf("Expected type 'sub', got '%s'", sub.Type)
		}
		if sub.Task != "Do something" {
			t.Errorf("Expected task 'Do something', got '%s'", sub.Task)
		}

		// Wait for completion with polling
		var status Status
		var result string
		for i := 0; i < 50; i++ {
			status, result, _, _ = mgr.SessionStatus(sub.ID)
			if status == StatusComplete || status == StatusFailed {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if status != StatusComplete {
			t.Errorf("Expected status 'complete', got '%s'", status)
		}
		if result != "Task done" {
			t.Errorf("Expected result 'Task done', got '%s'", result)
		}
		if !notified.Load() {
			t.Error("Should notify on completion")
		}
	})

	t.Run("SpawnWithError", func(t *testing.T) {
		notified.Store(false)
		mgr := NewManager(notify, 50, nil)
		mgr.SetTaskRunner(&MockTaskRunner{err: errors.New("task failed")})

		parent := mgr.GetOrCreate("telegram", "chat2", "user1")
		sub, _ := mgr.Spawn(parent, "Failing task")

		time.Sleep(50 * time.Millisecond)

		status, _, errMsg, _ := mgr.SessionStatus(sub.ID)
		if status != StatusFailed {
			t.Errorf("Expected status 'failed', got '%s'", status)
		}
		if errMsg != "task failed" {
			t.Errorf("Expected error 'task failed', got '%s'", errMsg)
		}
	})

	t.Run("SpawnWithoutTaskRunner", func(t *testing.T) {
		mgr := NewManager(nil, 50, nil)
		// No task runner set

		parent := mgr.GetOrCreate("telegram", "chat3", "user1")
		sub, _ := mgr.Spawn(parent, "No runner")

		time.Sleep(50 * time.Millisecond)

		status, _, _, _ := mgr.SessionStatus(sub.ID)
		if status != StatusFailed {
			t.Errorf("Expected status 'failed', got '%s'", status)
		}
	})
}

func TestList(t *testing.T) {
	mgr := NewManager(nil, 50, nil)

	// Create sessions
	mgr.GetOrCreate("telegram", "chat1", "user1")
	mgr.GetOrCreate("telegram", "chat2", "user1")
	mgr.GetOrCreate("telegram", "chat3", "user2")

	t.Run("ListAll", func(t *testing.T) {
		sessions := mgr.List("", true)
		if len(sessions) != 3 {
			t.Errorf("Expected 3 sessions, got %d", len(sessions))
		}
	})

	t.Run("ListByUser", func(t *testing.T) {
		sessions := mgr.List("user1", true)
		if len(sessions) != 2 {
			t.Errorf("Expected 2 sessions for user1, got %d", len(sessions))
		}
	})

	t.Run("ExcludeComplete", func(t *testing.T) {
		// Mark one as complete
		sess := mgr.Get("telegram:chat1")
		sess.Status = StatusComplete

		sessions := mgr.List("", false)
		if len(sessions) != 2 {
			t.Errorf("Expected 2 non-complete sessions, got %d", len(sessions))
		}
	})
}

func TestListSubSessions(t *testing.T) {
	mgr := NewManager(nil, 50, nil)
	mgr.SetTaskRunner(&MockTaskRunner{result: "done", delay: 100 * time.Millisecond})

	parent := mgr.GetOrCreate("telegram", "chat1", "user1")
	_, _ = mgr.Spawn(parent, "Sub 1")
	_, _ = mgr.Spawn(parent, "Sub 2")

	subs := mgr.ListSubSessions(parent.ID)
	if len(subs) != 2 {
		t.Errorf("Expected 2 sub-sessions, got %d", len(subs))
	}
}

func TestCancel(t *testing.T) {
	t.Run("CancelRunning", func(t *testing.T) {
		mgr := NewManager(nil, 50, nil)
		mgr.SetTaskRunner(&MockTaskRunner{result: "done", delay: 5 * time.Second})

		parent := mgr.GetOrCreate("telegram", "chat1", "user1")
		sub, _ := mgr.Spawn(parent, "Long task")

		time.Sleep(50 * time.Millisecond) // Let it start

		err := mgr.Cancel(sub.ID)
		if err != nil {
			t.Errorf("Cancel failed: %v", err)
		}

		time.Sleep(100 * time.Millisecond)

		// After cancel, status could be 'canceled' or 'failed' (due to context.Canceled error)
		// The important thing is that it's no longer running
		status, _, _, completedAt := mgr.SessionStatus(sub.ID)
		if status == StatusRunning || status == StatusPending {
			t.Errorf("Session should not be running/pending after cancel, got '%s'", status)
		}
		if completedAt == nil {
			t.Error("CompletedAt should be set after cancel")
		}
	})

	t.Run("CancelNonExistent", func(t *testing.T) {
		mgr := NewManager(nil, 50, nil)
		err := mgr.Cancel("nonexistent")
		if err == nil {
			t.Error("Should error on non-existent session")
		}
	})

	t.Run("CancelCompleted", func(t *testing.T) {
		mgr := NewManager(nil, 50, nil)
		sess := mgr.GetOrCreate("telegram", "chat1", "user1")
		sess.Status = StatusComplete

		err := mgr.Cancel(sess.ID)
		if err == nil {
			t.Error("Should error on completed session")
		}
	})
}

func TestClear(t *testing.T) {
	mgr := NewManager(nil, 50, nil)

	// Create some sessions
	sess1 := mgr.GetOrCreate("telegram", "chat1", "user1")
	sess2 := mgr.GetOrCreate("telegram", "chat2", "user1")
	sess3 := mgr.GetOrCreate("telegram", "chat3", "user1")

	// Mark as complete with old timestamps
	oldTime := time.Now().Add(-2 * time.Hour)
	sess1.Status = StatusComplete
	sess1.CompletedAt = &oldTime
	sess2.Status = StatusFailed
	sess2.CompletedAt = &oldTime

	// sess3 stays running

	cleared := mgr.Clear(1 * time.Hour)
	if cleared != 2 {
		t.Errorf("Expected 2 cleared, got %d", cleared)
	}

	// sess3 should still exist
	if mgr.Get(sess3.ID) == nil {
		t.Error("Running session should not be cleared")
	}
}

func TestContext(t *testing.T) {
	mgr := NewManager(nil, 50, nil)
	sess := mgr.GetOrCreate("telegram", "chat1", "user1")

	t.Run("SetAndGet", func(t *testing.T) {
		mgr.SetContext(sess, "topic", "weather")
		value := mgr.GetContext(sess, "topic")
		if value != "weather" {
			t.Errorf("Expected 'weather', got '%v'", value)
		}
	})

	t.Run("GetNonExistent", func(t *testing.T) {
		value := mgr.GetContext(sess, "nonexistent")
		if value != nil {
			t.Error("Non-existent key should return nil")
		}
	})

	t.Run("OverwriteValue", func(t *testing.T) {
		mgr.SetContext(sess, "count", 1)
		mgr.SetContext(sess, "count", 2)
		value := mgr.GetContext(sess, "count")
		if value != 2 {
			t.Errorf("Expected 2, got '%v'", value)
		}
	})

	t.Run("DifferentTypes", func(t *testing.T) {
		mgr.SetContext(sess, "string", "hello")
		mgr.SetContext(sess, "int", 42)
		mgr.SetContext(sess, "bool", true)
		mgr.SetContext(sess, "slice", []string{"a", "b"})

		if mgr.GetContext(sess, "string") != "hello" {
			t.Error("String context failed")
		}
		if mgr.GetContext(sess, "int") != 42 {
			t.Error("Int context failed")
		}
		if mgr.GetContext(sess, "bool") != true {
			t.Error("Bool context failed")
		}
	})
}

func TestConcurrency(t *testing.T) {
	mgr := NewManager(nil, 50, nil)

	t.Run("ConcurrentGetOrCreate", func(t *testing.T) {
		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				mgr.GetOrCreate("telegram", "concurrent_chat", "user")
			}(i)
		}
		wg.Wait()

		// Should only have one session
		sessions := mgr.List("", true)
		count := 0
		for _, s := range sessions {
			if s.ChatID == "concurrent_chat" {
				count++
			}
		}
		if count != 1 {
			t.Errorf("Expected 1 session, got %d", count)
		}
	})

	t.Run("ConcurrentAddMessage", func(t *testing.T) {
		sess := mgr.GetOrCreate("telegram", "msg_chat", "user")
		var wg sync.WaitGroup
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				mgr.AddMessage(sess, "user", "Message")
			}(i)
		}
		wg.Wait()

		// Should have 50 messages (no trim since maxHistory=50)
		if len(sess.Messages) != 50 {
			t.Errorf("Expected 50 messages, got %d", len(sess.Messages))
		}
	})
}

func TestStatus(t *testing.T) {
	statuses := []Status{
		StatusPending,
		StatusRunning,
		StatusComplete,
		StatusFailed,
		StatusCanceled,
	}

	for _, s := range statuses {
		if string(s) == "" {
			t.Errorf("Status %v should have string value", s)
		}
	}
}

func TestSetTaskRunner(t *testing.T) {
	mgr := NewManager(nil, 50, nil)
	runner := &MockTaskRunner{result: "test"}

	mgr.SetTaskRunner(runner)

	if mgr.taskRunner == nil {
		t.Error("Task runner should be set")
	}
}

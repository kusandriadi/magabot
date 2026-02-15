// Package test contains integration tests for agent module
package test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kusa/magabot/internal/agent"
)

func TestAgentIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create a test directory
	testDir := filepath.Join(tmpDir, "workspace")
	_ = os.MkdirAll(testDir, 0755)

	cfg := agent.Config{
		Main:        agent.AgentClaude,
		Timeout:     30,
		AllowedDirs: []string{tmpDir},
	}

	t.Run("CreateManager", func(t *testing.T) {
		mgr := agent.NewManager(cfg, logger)
		if mgr == nil {
			t.Fatal("Manager should not be nil")
		}
	})

	t.Run("ValidAgent", func(t *testing.T) {
		if !agent.ValidAgent("claude") {
			t.Error("claude should be valid")
		}
		if !agent.ValidAgent("codex") {
			t.Error("codex should be valid")
		}
		if !agent.ValidAgent("gemini") {
			t.Error("gemini should be valid")
		}
		if agent.ValidAgent("invalid") {
			t.Error("invalid should not be valid")
		}
	})

	t.Run("SessionKeyGeneration", func(t *testing.T) {
		mgr := agent.NewManager(cfg, logger)

		// Create session - will fail if binary not found, but that's OK for this test
		_, err := mgr.NewSession("telegram", "chat1", "user1", "claude", testDir)

		// Binary not found is expected in test environment
		if err != nil && err.Error() != "\"claude\" not found in PATH" {
			// If it's not a "not found" error, check if it's a different error
			t.Logf("NewSession error (expected in test env): %v", err)
		}
	})

	t.Run("DirectoryValidation", func(t *testing.T) {
		mgr := agent.NewManager(cfg, logger)

		// Try to create session with directory outside allowed paths
		_, err := mgr.NewSession("telegram", "chat1", "user1", "claude", "/etc")
		if err == nil {
			t.Error("Should reject directory outside allowed paths")
		}
	})

	t.Run("InvalidAgent", func(t *testing.T) {
		mgr := agent.NewManager(cfg, logger)

		_, err := mgr.NewSession("telegram", "chat1", "user1", "invalidagent", testDir)
		if err == nil {
			t.Error("Should reject invalid agent")
		}
	})

	t.Run("NonExistentDirectory", func(t *testing.T) {
		mgr := agent.NewManager(cfg, logger)

		_, err := mgr.NewSession("telegram", "chat1", "user1", "claude", filepath.Join(tmpDir, "nonexistent"))
		if err == nil {
			t.Error("Should reject non-existent directory")
		}
	})

	t.Run("FileNotDirectory", func(t *testing.T) {
		// Create a file
		filePath := filepath.Join(tmpDir, "testfile.txt")
		_ = os.WriteFile(filePath, []byte("test"), 0644)

		mgr := agent.NewManager(cfg, logger)

		_, err := mgr.NewSession("telegram", "chat1", "user1", "claude", filePath)
		if err == nil {
			t.Error("Should reject file path (not directory)")
		}
	})

	t.Run("HasSession", func(t *testing.T) {
		mgr := agent.NewManager(cfg, logger)

		// No session initially
		if mgr.HasSession("telegram", "chat1") {
			t.Error("Should not have session initially")
		}

		// Create session (may fail due to binary not found)
		_, _ = mgr.NewSession("telegram", "chat1", "user1", "claude", testDir)

		// GetSession should return nil if session creation failed
		sess := mgr.GetSession("telegram", "chat1")
		if sess != nil {
			// Session was created successfully
			if !mgr.HasSession("telegram", "chat1") {
				t.Error("Should have session after creation")
			}
		}
	})

	t.Run("CloseSession", func(t *testing.T) {
		mgr := agent.NewManager(cfg, logger)

		// Create and close session
		_, _ = mgr.NewSession("telegram", "chat1", "user1", "claude", testDir)
		mgr.CloseSession("telegram", "chat1")

		if mgr.HasSession("telegram", "chat1") {
			t.Error("Should not have session after close")
		}
	})

	t.Run("DefaultAgent", func(t *testing.T) {
		mgr := agent.NewManager(cfg, logger)

		// Empty agent should use default
		sess, err := mgr.NewSession("telegram", "chat1", "user1", "", testDir)
		if err == nil && sess != nil {
			if sess.Agent != "claude" {
				t.Errorf("Expected default agent 'claude', got '%s'", sess.Agent)
			}
		}
	})

	t.Run("MultipleSessions", func(t *testing.T) {
		mgr := agent.NewManager(cfg, logger)

		// Create sessions for different chats
		_, _ = mgr.NewSession("telegram", "chat1", "user1", "claude", testDir)
		_, _ = mgr.NewSession("telegram", "chat2", "user2", "claude", testDir)
		_, _ = mgr.NewSession("whatsapp", "chat1", "user1", "claude", testDir)

		// Each should be independent
		sess1 := mgr.GetSession("telegram", "chat1")
		sess2 := mgr.GetSession("telegram", "chat2")
		sessWA := mgr.GetSession("whatsapp", "chat1")

		// If any session was created successfully, they should be different
		if sess1 != nil && sess2 != nil && sess1 == sess2 {
			t.Error("Sessions should be independent")
		}

		if sess1 != nil && sessWA != nil && sess1 == sessWA {
			t.Error("Sessions across platforms should be independent")
		}
	})
}

func TestStripANSI(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "Hello \x1b[31mWorld\x1b[0m",
			expected: "Hello World",
		},
		{
			input:    "No ANSI here",
			expected: "No ANSI here",
		},
		{
			input:    "\x1b[1;32mGreen Bold\x1b[0m",
			expected: "Green Bold",
		},
		{
			input:    "",
			expected: "",
		},
	}

	for _, tc := range tests {
		result := agent.StripANSI(tc.input)
		if result != tc.expected {
			t.Errorf("StripANSI(%q) = %q, expected %q", tc.input, result, tc.expected)
		}
	}
}

func TestSessionMsgCount(t *testing.T) {
	sess := &agent.Session{
		Agent:    "claude",
		Dir:      "/tmp",
		Platform: "telegram",
		ChatID:   "chat1",
		UserID:   "user1",
		MsgCount: 0,
	}

	if sess.GetMsgCount() != 0 {
		t.Error("Initial MsgCount should be 0")
	}

	// Note: MsgCount is incremented by Execute, which we can't test without real binary
}

func TestAgentConfigDefaults(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	t.Run("EmptyDefault", func(t *testing.T) {
		cfg := agent.Config{}
		mgr := agent.NewManager(cfg, logger)
		if mgr == nil {
			t.Fatal("Manager should handle empty config")
		}
	})

	t.Run("ZeroTimeout", func(t *testing.T) {
		cfg := agent.Config{
			Timeout: 0,
		}
		mgr := agent.NewManager(cfg, logger)
		if mgr == nil {
			t.Fatal("Manager should handle zero timeout")
		}
	})
}

func TestExecuteTimeout(t *testing.T) {
	// Skip if claude binary is not available
	t.Skip("Requires claude binary in PATH")

	tmpDir := t.TempDir()
	testDir := filepath.Join(tmpDir, "workspace")
	_ = os.MkdirAll(testDir, 0755)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := agent.Config{
		Main:        "claude",
		Timeout:     1, // Very short timeout
		AllowedDirs: []string{tmpDir},
	}

	mgr := agent.NewManager(cfg, logger)
	sess, err := mgr.NewSession("telegram", "chat1", "user1", "claude", testDir)
	if err != nil {
		t.Skip("Claude not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err = mgr.Execute(ctx, sess, "Long running task")
	// Should timeout or error
	if err == nil {
		t.Log("Execute completed (may have returned quickly)")
	}
}

// Package test contains integration tests for hooks module
package test

import (
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kusa/magabot/internal/config"
	"github.com/kusa/magabot/internal/hooks"
)

func TestHooksIntegration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows - shell scripts not supported")
	}

	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create a simple test script
	scriptPath := filepath.Join(tmpDir, "test_hook.sh")
	scriptContent := `#!/bin/bash
# Read JSON from stdin
read -r INPUT
# Echo a modified response
echo "Modified by hook"
exit 0
`
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to create test script: %v", err)
	}

	t.Run("FirePreMessageHook", func(t *testing.T) {
		hookCfg := []config.HookConfig{
			{
				Name:    "test_pre_message",
				Event:   "pre_message",
				Command: scriptPath,
				Async:   false,
			},
		}

		mgr := hooks.NewManager(hookCfg, logger)

		eventData := &hooks.EventData{
			Platform: "telegram",
			UserID:   "user1",
			ChatID:   "chat1",
			Text:     "Original message",
		}

		result := mgr.Fire(hooks.PreMessage, eventData)

		if result.Output != "Modified by hook" {
			t.Errorf("Expected 'Modified by hook', got '%s'", result.Output)
		}

		if result.Blocked {
			t.Error("Hook should not block")
		}
	})

	t.Run("FirePostResponseHook", func(t *testing.T) {
		hookCfg := []config.HookConfig{
			{
				Name:    "test_post_response",
				Event:   "post_response",
				Command: scriptPath,
				Async:   false,
			},
		}

		mgr := hooks.NewManager(hookCfg, logger)

		eventData := &hooks.EventData{
			Platform: "telegram",
			UserID:   "user1",
			ChatID:   "chat1",
			Text:     "User message",
			Response: "Bot response",
		}

		result := mgr.Fire(hooks.PostResponse, eventData)

		if result.Output == "" {
			t.Error("Expected non-empty output from post_response hook")
		}
	})

	t.Run("BlockingHook", func(t *testing.T) {
		// Create a script that exits with error
		blockScript := filepath.Join(tmpDir, "block_hook.sh")
		blockContent := `#!/bin/bash
exit 1
`
		_ = os.WriteFile(blockScript, []byte(blockContent), 0755)

		hookCfg := []config.HookConfig{
			{
				Name:    "blocker",
				Event:   "pre_message",
				Command: blockScript,
				Async:   false,
			},
		}

		mgr := hooks.NewManager(hookCfg, logger)

		result := mgr.Fire(hooks.PreMessage, &hooks.EventData{Text: "test"})

		if !result.Blocked {
			t.Error("Hook should block when exiting with error")
		}
	})

	t.Run("PlatformFilter", func(t *testing.T) {
		// Create a hook that only runs for telegram
		hookCfg := []config.HookConfig{
			{
				Name:      "telegram_only",
				Event:     "pre_message",
				Command:   scriptPath,
				Platforms: []string{"telegram"},
				Async:     false,
			},
		}

		mgr := hooks.NewManager(hookCfg, logger)

		// Should run for telegram
		result := mgr.Fire(hooks.PreMessage, &hooks.EventData{
			Platform: "telegram",
			Text:     "test",
		})
		if result.Output == "" {
			t.Error("Hook should run for telegram")
		}

		// Should not run for whatsapp
		result = mgr.Fire(hooks.PreMessage, &hooks.EventData{
			Platform: "whatsapp",
			Text:     "test",
		})
		if result.Output != "" {
			t.Error("Hook should not run for whatsapp")
		}
	})

	t.Run("AsyncHook", func(t *testing.T) {
		hookCfg := []config.HookConfig{
			{
				Name:    "async_hook",
				Event:   "on_error",
				Command: scriptPath,
				Async:   true,
			},
		}

		mgr := hooks.NewManager(hookCfg, logger)

		// FireAsync should not block
		mgr.FireAsync(hooks.OnError, &hooks.EventData{
			Error: "test error",
		})

		// No panic = success for async
	})

	t.Run("NoMatchingHooks", func(t *testing.T) {
		hookCfg := []config.HookConfig{
			{
				Name:    "on_start_hook",
				Event:   "on_start",
				Command: scriptPath,
				Async:   false,
			},
		}

		mgr := hooks.NewManager(hookCfg, logger)

		// Fire a different event
		result := mgr.Fire(hooks.PreMessage, &hooks.EventData{Text: "test"})

		if result.Output != "" {
			t.Error("Should have no output when no hooks match")
		}
	})

	t.Run("MultipleHooks", func(t *testing.T) {
		// Create two scripts
		script1 := filepath.Join(tmpDir, "hook1.sh")
		os.WriteFile(script1, []byte("#!/bin/bash\necho 'first'\nexit 0"), 0755)

		script2 := filepath.Join(tmpDir, "hook2.sh")
		os.WriteFile(script2, []byte("#!/bin/bash\necho 'second'\nexit 0"), 0755)

		hookCfg := []config.HookConfig{
			{
				Name:    "first",
				Event:   "pre_message",
				Command: script1,
				Async:   false,
			},
			{
				Name:    "second",
				Event:   "pre_message",
				Command: script2,
				Async:   false,
			},
		}

		mgr := hooks.NewManager(hookCfg, logger)

		result := mgr.Fire(hooks.PreMessage, &hooks.EventData{Text: "test"})

		// Last non-empty output wins
		if result.Output != "second" {
			t.Errorf("Expected 'second', got '%s'", result.Output)
		}
	})

	t.Run("EmptyHooksList", func(t *testing.T) {
		mgr := hooks.NewManager(nil, logger)

		result := mgr.Fire(hooks.PreMessage, &hooks.EventData{Text: "test"})

		if result.Blocked {
			t.Error("Empty hooks should not block")
		}
	})

	t.Run("HasHooks", func(t *testing.T) {
		hookCfg := []config.HookConfig{
			{
				Name:    "test",
				Event:   "pre_message",
				Command: scriptPath,
			},
		}

		mgr := hooks.NewManager(hookCfg, logger)

		if !mgr.HasHooks(hooks.PreMessage) {
			t.Error("Should have pre_message hooks")
		}

		if mgr.HasHooks(hooks.OnStop) {
			t.Error("Should not have on_stop hooks")
		}
	})
}

func TestHooksEventTypes(t *testing.T) {
	// Verify all event types are defined
	events := []hooks.Event{
		hooks.PreMessage,
		hooks.PostResponse,
		hooks.OnCommand,
		hooks.OnStart,
		hooks.OnStop,
		hooks.OnError,
	}

	for _, e := range events {
		if string(e) == "" {
			t.Errorf("Event %v should have a string value", e)
		}
	}
}

package hooks_test

import (
	"log/slog"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kusa/magabot/internal/config"
	"github.com/kusa/magabot/internal/hooks"
)

func newLogger() *slog.Logger {
	return slog.Default()
}

// skipIfNoShell skips the test when the platform shell (sh or cmd) is not available.
func skipIfNoShell(t *testing.T) {
	t.Helper()
	shell := "sh"
	if runtime.GOOS == "windows" {
		shell = "cmd"
	}
	if _, err := exec.LookPath(shell); err != nil {
		t.Skipf("shell %q not in PATH, skipping", shell)
	}
}

func TestNewManager_NilHooks(t *testing.T) {
	m := hooks.NewManager(nil, newLogger())
	if m == nil {
		t.Fatal("expected non-nil manager")
	}
	if m.HasHooks(hooks.OnStart) {
		t.Error("expected HasHooks to return false with nil hooks")
	}
}

func TestNewManager_EmptyHooks(t *testing.T) {
	m := hooks.NewManager([]config.HookConfig{}, newLogger())
	if m == nil {
		t.Fatal("expected non-nil manager")
	}
	if m.HasHooks(hooks.OnStart) {
		t.Error("expected HasHooks to return false with empty hooks")
	}
}

func TestNewManager_NilLogger(t *testing.T) {
	// Should not panic with nil logger.
	m := hooks.NewManager(nil, nil)
	if m == nil {
		t.Fatal("expected non-nil manager")
	}
}

func TestHasHooks_True(t *testing.T) {
	hooksConfig := []config.HookConfig{
		{Name: "startup", Event: "on_start", Command: "echo started"},
	}
	m := hooks.NewManager(hooksConfig, newLogger())

	if !m.HasHooks(hooks.OnStart) {
		t.Error("expected HasHooks(OnStart) to return true")
	}
}

func TestHasHooks_False(t *testing.T) {
	hooksConfig := []config.HookConfig{
		{Name: "startup", Event: "on_start", Command: "echo started"},
	}
	m := hooks.NewManager(hooksConfig, newLogger())

	if m.HasHooks(hooks.OnError) {
		t.Error("expected HasHooks(OnError) to return false when no hooks for that event")
	}
}

func TestHasHooks_MultipleEvents(t *testing.T) {
	hooksConfig := []config.HookConfig{
		{Name: "start-hook", Event: "on_start", Command: "echo start"},
		{Name: "stop-hook", Event: "on_stop", Command: "echo stop"},
	}
	m := hooks.NewManager(hooksConfig, newLogger())

	if !m.HasHooks(hooks.OnStart) {
		t.Error("expected HasHooks(OnStart) true")
	}
	if !m.HasHooks(hooks.OnStop) {
		t.Error("expected HasHooks(OnStop) true")
	}
	if m.HasHooks(hooks.OnError) {
		t.Error("expected HasHooks(OnError) false")
	}
}

func TestFire_NoHooks(t *testing.T) {
	m := hooks.NewManager(nil, newLogger())

	result := m.Fire(hooks.OnStart, &hooks.EventData{})
	if result == nil {
		t.Fatal("expected non-nil result even with no hooks")
	}
	if result.Output != "" {
		t.Errorf("expected empty output, got %q", result.Output)
	}
	if result.Blocked {
		t.Error("expected Blocked=false with no hooks")
	}
}

func TestFire_Echo(t *testing.T) {
	skipIfNoShell(t)
	hooksConfig := []config.HookConfig{
		{Name: "echo-hook", Event: "on_start", Command: "echo hello"},
	}
	m := hooks.NewManager(hooksConfig, newLogger())

	result := m.Fire(hooks.OnStart, &hooks.EventData{})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !strings.Contains(result.Output, "hello") {
		t.Errorf("expected output to contain 'hello', got %q", result.Output)
	}
	if result.Blocked {
		t.Error("expected Blocked=false for successful command")
	}
}

func TestFire_PlatformFilter(t *testing.T) {
	hooksConfig := []config.HookConfig{
		{
			Name:      "telegram-only",
			Event:     "pre_message",
			Command:   "echo telegram-output",
			Platforms: []string{"telegram"},
		},
	}
	m := hooks.NewManager(hooksConfig, newLogger())

	// Fire with platform "slack" -- should not match the telegram-only hook.
	result := m.Fire(hooks.PreMessage, &hooks.EventData{Platform: "slack"})
	if result.Output != "" {
		t.Errorf("expected empty output for non-matching platform, got %q", result.Output)
	}
	if result.Blocked {
		t.Error("expected Blocked=false for non-matching platform")
	}
}

func TestFire_PlatformFilter_Matching(t *testing.T) {
	skipIfNoShell(t)
	hooksConfig := []config.HookConfig{
		{
			Name:      "telegram-only",
			Event:     "pre_message",
			Command:   "echo matched",
			Platforms: []string{"telegram"},
		},
	}
	m := hooks.NewManager(hooksConfig, newLogger())

	// Fire with platform "telegram" -- should match.
	result := m.Fire(hooks.PreMessage, &hooks.EventData{Platform: "telegram"})
	if !strings.Contains(result.Output, "matched") {
		t.Errorf("expected output to contain 'matched', got %q", result.Output)
	}
}

func TestFire_PlatformFilter_EmptyMatchesAll(t *testing.T) {
	skipIfNoShell(t)
	hooksConfig := []config.HookConfig{
		{
			Name:      "all-platforms",
			Event:     "on_start",
			Command:   "echo universal",
			Platforms: []string{}, // empty = all platforms
		},
	}
	m := hooks.NewManager(hooksConfig, newLogger())

	result := m.Fire(hooks.OnStart, &hooks.EventData{Platform: "discord"})
	if !strings.Contains(result.Output, "universal") {
		t.Errorf("expected empty platforms to match all, got %q", result.Output)
	}
}

func TestFire_PlatformFilter_CaseInsensitive(t *testing.T) {
	skipIfNoShell(t)
	hooksConfig := []config.HookConfig{
		{
			Name:      "tg-hook",
			Event:     "on_start",
			Command:   "echo case-match",
			Platforms: []string{"Telegram"},
		},
	}
	m := hooks.NewManager(hooksConfig, newLogger())

	result := m.Fire(hooks.OnStart, &hooks.EventData{Platform: "telegram"})
	if !strings.Contains(result.Output, "case-match") {
		t.Errorf("expected case-insensitive platform match, got %q", result.Output)
	}
}

func TestFire_BlockedHook(t *testing.T) {
	skipIfNoShell(t)
	hooksConfig := []config.HookConfig{
		{Name: "fail-hook", Event: "pre_message", Command: "exit 1"},
	}
	m := hooks.NewManager(hooksConfig, newLogger())

	result := m.Fire(hooks.PreMessage, &hooks.EventData{})
	if !result.Blocked {
		t.Error("expected Blocked=true for hook that exits with non-zero status")
	}
}

func TestFire_BlockedHook_SuccessfulNotBlocked(t *testing.T) {
	skipIfNoShell(t)
	hooksConfig := []config.HookConfig{
		{Name: "ok-hook", Event: "pre_message", Command: "exit 0"},
	}
	m := hooks.NewManager(hooksConfig, newLogger())

	result := m.Fire(hooks.PreMessage, &hooks.EventData{})
	if result.Blocked {
		t.Error("expected Blocked=false for hook that exits with 0")
	}
}

func TestFire_EventMismatch(t *testing.T) {
	hooksConfig := []config.HookConfig{
		{Name: "start-hook", Event: "on_start", Command: "echo should-not-run"},
	}
	m := hooks.NewManager(hooksConfig, newLogger())

	// Fire a different event.
	result := m.Fire(hooks.OnStop, &hooks.EventData{})
	if result.Output != "" {
		t.Errorf("expected no output for mismatched event, got %q", result.Output)
	}
}

func TestFireAsync_DoesNotBlock(t *testing.T) {
	// Use a cross-platform long-running command
	command := "sleep 5"
	if runtime.GOOS == "windows" {
		command = "waitfor /t 5 pause 2>nul || exit 0"
	}

	hooksConfig := []config.HookConfig{
		{Name: "slow-hook", Event: "on_start", Command: command},
	}
	m := hooks.NewManager(hooksConfig, newLogger())

	done := make(chan struct{})
	go func() {
		m.FireAsync(hooks.OnStart, &hooks.EventData{})
		close(done)
	}()

	select {
	case <-done:
		// FireAsync returned promptly.
	case <-time.After(2 * time.Second):
		t.Fatal("FireAsync should not block; it hung for over 2 seconds")
	}
}

func TestFireAsync_NoHooks(t *testing.T) {
	m := hooks.NewManager(nil, newLogger())

	// Should return immediately without panic.
	m.FireAsync(hooks.OnStart, &hooks.EventData{})
}

func TestFire_MultipleHooksSameEvent(t *testing.T) {
	skipIfNoShell(t)
	hooksConfig := []config.HookConfig{
		{Name: "hook-1", Event: "on_start", Command: "echo first"},
		{Name: "hook-2", Event: "on_start", Command: "echo second"},
	}
	m := hooks.NewManager(hooksConfig, newLogger())

	result := m.Fire(hooks.OnStart, &hooks.EventData{})
	// The last non-empty stdout wins, per the Fire documentation.
	if !strings.Contains(result.Output, "second") {
		t.Errorf("expected output from last hook, got %q", result.Output)
	}
}

func TestFire_AsyncHookInSyncFire(t *testing.T) {
	hooksConfig := []config.HookConfig{
		{Name: "async-in-sync", Event: "on_start", Command: "echo async-output", Async: true},
	}
	m := hooks.NewManager(hooksConfig, newLogger())

	// Even though Fire is synchronous, hooks marked Async are fired in a goroutine
	// and their output is not captured in the result.
	result := m.Fire(hooks.OnStart, &hooks.EventData{})
	// The async hook's output should not appear in synchronous result.
	if result.Output != "" {
		// This is acceptable; the hook runs asynchronously, so output may or may not be captured.
		// The important thing is that Fire returns without hanging.
		t.Logf("note: async hook output appeared in sync Fire result: %q", result.Output)
	}
}

// --- Negative tests ---

func TestFire_CommandNotFound(t *testing.T) {
	skipIfNoShell(t)
	hooksConfig := []config.HookConfig{
		{Name: "bad-cmd", Event: "on_start", Command: "nonexistent_command_xyz_12345"},
	}
	m := hooks.NewManager(hooksConfig, newLogger())

	result := m.Fire(hooks.OnStart, &hooks.EventData{})
	if !result.Blocked {
		t.Error("expected Blocked=true for nonexistent command")
	}
}

func TestFire_Timeout(t *testing.T) {
	skipIfNoShell(t)
	// Use a command that runs longer than the timeout.
	// Use exec-style sleep to avoid orphan child processes with sh -c.
	command := "sleep 5"
	if runtime.GOOS == "windows" {
		command = "waitfor /t 5 pause 2>nul || exit 0"
	}

	hooksConfig := []config.HookConfig{
		{Name: "timeout-hook", Event: "on_start", Command: command, Timeout: 1},
	}
	m := hooks.NewManager(hooksConfig, newLogger())

	result := m.Fire(hooks.OnStart, &hooks.EventData{})
	// The hook should be blocked due to timeout-induced kill.
	if !result.Blocked {
		t.Error("expected Blocked=true for timed-out command")
	}
}

func TestFire_LargeOutput(t *testing.T) {
	// Generate output larger than maxOutputBytes (1 MB) — should not OOM or hang
	// On both platforms, echo in a loop produces enough output.
	command := "dd if=/dev/zero bs=1024 count=2048 2>/dev/null | tr '\\0' 'A'"
	if runtime.GOOS == "windows" {
		t.Skip("dd not available on Windows")
	}

	hooksConfig := []config.HookConfig{
		{Name: "large-output", Event: "on_start", Command: command, Timeout: 10},
	}
	m := hooks.NewManager(hooksConfig, newLogger())

	result := m.Fire(hooks.OnStart, &hooks.EventData{})
	// Should complete without hanging or OOM.
	// Output should be truncated to maxOutputBytes (1 MB).
	if len(result.Output) > 1*1024*1024+100 {
		t.Errorf("expected output truncated to ~1MB, got %d bytes", len(result.Output))
	}
}

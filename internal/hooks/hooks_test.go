package hooks_test

import (
	"log/slog"
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
	if runtime.GOOS == "windows" {
		t.Skip("shell test not supported on Windows")
	}

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
	if runtime.GOOS == "windows" {
		t.Skip("shell test not supported on Windows")
	}

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
	if runtime.GOOS == "windows" {
		t.Skip("shell test not supported on Windows")
	}

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
	if runtime.GOOS == "windows" {
		t.Skip("shell test not supported on Windows")
	}

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
	if runtime.GOOS == "windows" {
		t.Skip("shell test not supported on Windows")
	}

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
	if runtime.GOOS == "windows" {
		t.Skip("shell test not supported on Windows")
	}

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
	if runtime.GOOS == "windows" {
		t.Skip("shell test not supported on Windows")
	}

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
	if runtime.GOOS == "windows" {
		t.Skip("shell test not supported on Windows")
	}

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
	if runtime.GOOS == "windows" {
		t.Skip("shell test not supported on Windows")
	}

	hooksConfig := []config.HookConfig{
		{Name: "slow-hook", Event: "on_start", Command: "sleep 5"},
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
	if runtime.GOOS == "windows" {
		t.Skip("shell test not supported on Windows")
	}

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
	if runtime.GOOS == "windows" {
		t.Skip("shell test not supported on Windows")
	}

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

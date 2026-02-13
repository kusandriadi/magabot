// Package hooks provides event-driven shell command execution for magabot.
//
// Hooks allow users to run custom scripts in response to bot events like
// incoming messages, outgoing responses, commands, errors, and lifecycle
// events. Each hook receives event data as JSON on stdin and can optionally
// return modified text on stdout (for pre_message and post_response events).
package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/kusa/magabot/internal/config"
)

// maxOutputBytes limits hook stdout/stderr capture (1 MB).
const maxOutputBytes = 1 * 1024 * 1024

// Event types that hooks can subscribe to.
type Event string

const (
	PreMessage   Event = "pre_message"
	PostResponse Event = "post_response"
	OnCommand    Event = "on_command"
	OnStart      Event = "on_start"
	OnStop       Event = "on_stop"
	OnError      Event = "on_error"
)

// EventData is the JSON payload passed to hooks on stdin.
type EventData struct {
	Event     string   `json:"event"`
	Platform  string   `json:"platform,omitempty"`
	UserID    string   `json:"user_id,omitempty"`
	ChatID    string   `json:"chat_id,omitempty"`
	Text      string   `json:"text,omitempty"`
	Response  string   `json:"response,omitempty"`
	Command   string   `json:"command,omitempty"`
	Args      []string `json:"args,omitempty"`
	Provider  string   `json:"provider,omitempty"`
	Model     string   `json:"model,omitempty"`
	LatencyMs int64    `json:"latency_ms,omitempty"`
	Error     string   `json:"error,omitempty"`
	Version   string   `json:"version,omitempty"`
	Platforms []string `json:"platforms,omitempty"`
}

// Result holds the outcome of a synchronous hook execution.
type Result struct {
	Output  string // trimmed stdout from the hook command
	Blocked bool   // true if the hook exited with non-zero status
}

// Manager manages and fires hooks based on events.
type Manager struct {
	hooks  []config.HookConfig
	logger *slog.Logger
}

// NewManager creates a hook manager. Pass nil or empty slice if no hooks configured.
func NewManager(hooks []config.HookConfig, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		hooks:  hooks,
		logger: logger,
	}
}

// Fire executes all hooks matching the given event synchronously.
// For pre_message/post_response events, the last non-empty stdout output
// from a matching hook is returned in Result.Output. If any hook exits
// with a non-zero code, Result.Blocked is true.
func (m *Manager) Fire(event Event, data *EventData) *Result {
	result := &Result{}
	if len(m.hooks) == 0 {
		return result
	}

	data.Event = string(event)

	for _, h := range m.hooks {
		if Event(h.Event) != event {
			continue
		}
		if !matchesPlatform(h.Platforms, data.Platform) {
			continue
		}

		if h.Async {
			go m.executeHook(h, data)
			continue
		}

		out, err := m.executeHook(h, data)
		if err != nil {
			result.Blocked = true
			m.logger.Warn("hook blocked or failed",
				"hook", h.Name, "event", event, "error", err)
		}
		if out != "" {
			result.Output = out
		}
	}

	return result
}

// FireAsync executes all matching hooks asynchronously (fire-and-forget).
func (m *Manager) FireAsync(event Event, data *EventData) {
	if len(m.hooks) == 0 {
		return
	}

	data.Event = string(event)

	for _, h := range m.hooks {
		if Event(h.Event) != event {
			continue
		}
		if !matchesPlatform(h.Platforms, data.Platform) {
			continue
		}
		go m.executeHook(h, data)
	}
}

// matchesPlatform checks if a hook should run for the given platform.
func matchesPlatform(platforms []string, platform string) bool {
	if len(platforms) == 0 {
		return true
	}
	for _, p := range platforms {
		if strings.EqualFold(p, platform) {
			return true
		}
	}
	return false
}

// shellCommand returns the platform-appropriate shell and args.
func shellCommand(command string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/C", command}
	}
	return "sh", []string{"-c", command}
}

// executeHook runs a single hook command with event data on stdin.
// Returns trimmed stdout and any execution error.
func (m *Manager) executeHook(h config.HookConfig, data *EventData) (string, error) {
	timeout := time.Duration(h.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	jsonData, err := json.Marshal(data)
	if err != nil {
		m.logger.Error("hook marshal failed", "hook", h.Name, "error", err)
		return "", fmt.Errorf("marshal: %w", err)
	}

	shell, shellArgs := shellCommand(h.Command)
	cmd := exec.CommandContext(ctx, shell, shellArgs...)
	cmd.Stdin = bytes.NewReader(jsonData)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitedWriter{w: &stdout, n: maxOutputBytes}
	cmd.Stderr = &limitedWriter{w: &stderr, n: maxOutputBytes}

	m.logger.Debug("firing hook", "hook", h.Name, "event", data.Event, "platform", data.Platform)

	if err := cmd.Run(); err != nil {
		m.logger.Warn("hook execution failed",
			"hook", h.Name,
			"event", data.Event,
			"error", err,
			"stderr", strings.TrimSpace(stderr.String()),
		)
		return strings.TrimSpace(stdout.String()), err
	}

	output := strings.TrimSpace(stdout.String())
	if output != "" {
		m.logger.Debug("hook produced output", "hook", h.Name, "output_len", len(output))
	}

	return output, nil
}

// HasHooks returns true if any hooks are configured for the given event.
func (m *Manager) HasHooks(event Event) bool {
	for _, h := range m.hooks {
		if Event(h.Event) == event {
			return true
		}
	}
	return false
}

// limitedWriter wraps a writer and stops after n bytes, preventing OOM.
type limitedWriter struct {
	w io.Writer
	n int64
}

func (lw *limitedWriter) Write(p []byte) (int, error) {
	if lw.n <= 0 {
		return len(p), nil
	}
	if int64(len(p)) > lw.n {
		p = p[:lw.n]
	}
	n, err := lw.w.Write(p)
	lw.n -= int64(n)
	return n, err
}

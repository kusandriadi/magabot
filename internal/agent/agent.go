// Package agent manages coding agent sessions (Claude Code, Codex, Gemini CLI)
// that users can control through chat commands.
package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Supported agent types.
const (
	AgentClaude = "claude"
	AgentCodex  = "codex"
	AgentGemini = "gemini"
)

// maxOutputBytes limits CLI stdout/stderr capture to prevent OOM (10 MB).
const maxOutputBytes = 10 * 1024 * 1024

// CLISettings returns the current model and effort for Claude CLI.
// This is called on each execution to pick up runtime changes from /model and /effort.
type CLISettings func() (model, effort string)

// NotifyFunc sends a message to a specific platform+chat (for idle timeout notifications).
type NotifyFunc func(platform, chatID, message string)

// Config holds agent manager settings.
type Config struct {
	Main           string            // main/primary agent type
	Timeout        int               // execution timeout in seconds
	MaxRetries     int               // auto-retry on timeout (0 = no retry)
	SessionTimeout int               // idle session timeout in seconds (0 = disabled, default 21600 = 6h)
	AllowedDirs    []string          // directories users may target (empty = user home only)
	Shortcuts      map[string]string // directory shortcuts, e.g. "myproject": "~/code/myproject"
	DiscoverDepth  int               // auto-discover search depth (default 3)
	GetCLISettings CLISettings       // optional: returns current model+effort for Claude CLI
	PlanDelegate   bool              // plan first, then delegate to subagents
	OnSessionClose NotifyFunc        // optional: called when a session is auto-closed
}

// Session represents an active agent session tied to a chat.
type Session struct {
	mu           sync.Mutex
	Agent        string // agent type: claude, codex, gemini
	Dir          string // working directory (resolved absolute path)
	Platform     string
	ChatID       string
	UserID       string
	MsgCount     int       // tracks messages sent (for --continue)
	LastActivity time.Time // last Execute() call (for idle timeout)
}

// Manager manages agent sessions across chats.
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session // key: "platform:chatID"
	config   Config
	logger   *slog.Logger
	done     chan struct{} // signals idle cleanup goroutine to stop
	stopOnce sync.Once
}

// agentInfo maps agent type to its binary name.
var agentInfo = map[string]string{
	AgentClaude: "claude",
	AgentCodex:  "codex",
	AgentGemini: "gemini",
}

// ansiRegex matches ANSI escape sequences for stripping from CLI output.
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\].*?\x07|\x1b\[.*?[@-~]`)

// NewManager creates a new agent session manager.
func NewManager(cfg Config, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.Main == "" {
		cfg.Main = AgentClaude
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 300
	}
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = 2
	}
	if cfg.SessionTimeout < 0 {
		cfg.SessionTimeout = 0
	}
	m := &Manager{
		sessions: make(map[string]*Session),
		config:   cfg,
		logger:   logger,
		done:     make(chan struct{}),
	}
	if cfg.SessionTimeout > 0 {
		go m.idleCleanupLoop()
	}
	return m
}

// Stop signals the idle cleanup goroutine to exit.
func (m *Manager) Stop() {
	m.stopOnce.Do(func() { close(m.done) })
}

// sessionKey returns the map key for a platform+chatID pair.
func sessionKey(platform, chatID string) string {
	return platform + ":" + chatID
}

// ValidAgent returns true if the agent type is recognized.
func ValidAgent(agent string) bool {
	_, ok := agentInfo[agent]
	return ok
}

// NewSession creates and registers a new agent session.
// Returns an error if the directory is not allowed, doesn't exist, or the agent binary is not in PATH.
func (m *Manager) NewSession(platform, chatID, userID, agent, dir string) (*Session, error) {
	if agent == "" {
		agent = m.config.Main
	}
	bin, ok := agentInfo[agent]
	if !ok {
		return nil, fmt.Errorf("unknown agent %q (supported: claude, codex, gemini)", agent)
	}

	// Resolve to absolute path to prevent traversal
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve path %q: %w", dir, err)
	}

	// Validate directory is under an allowed parent
	if err := m.validateDir(absDir); err != nil {
		return nil, err
	}

	// Validate directory exists
	info, err := os.Stat(absDir)
	if err != nil {
		return nil, fmt.Errorf("directory %q: %w", absDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%q is not a directory", absDir)
	}

	// Check binary is in PATH
	if _, err := exec.LookPath(bin); err != nil {
		return nil, fmt.Errorf("%q not found in PATH", bin)
	}

	key := sessionKey(platform, chatID)
	sess := &Session{
		Agent:        agent,
		Dir:          absDir,
		Platform:     platform,
		ChatID:       chatID,
		UserID:       userID,
		LastActivity: time.Now(),
	}

	m.mu.Lock()
	m.sessions[key] = sess
	m.mu.Unlock()

	m.logger.Info("agent session created",
		"agent", agent, "dir", absDir,
		"platform", platform, "chat_id", chatID,
	)

	return sess, nil
}

// validateDir checks that the directory is under an allowed parent.
// If AllowedDirs is empty, only the user's home directory is allowed.
func (m *Manager) validateDir(absDir string) error {
	allowed := m.config.AllowedDirs
	if len(allowed) == 0 {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot determine home directory: %w", err)
		}
		allowed = []string{home}
	}

	for _, parent := range allowed {
		absParent, err := filepath.Abs(parent)
		if err != nil {
			continue
		}
		if absDir == absParent || strings.HasPrefix(absDir, absParent+string(filepath.Separator)) {
			return nil
		}
	}

	return fmt.Errorf("directory %q is not under an allowed path", absDir)
}

// ResolveDir resolves a directory shortcut or name to an absolute path.
// Resolution order:
//  1. "home" or "~" → user's home directory
//  2. "~/path" → expand tilde
//  3. Configured shortcuts
//  4. Auto-discover: search ~/<name> and ~/*/<name> for a matching directory
//  5. Return as-is (let NewSession validate)
func (m *Manager) ResolveDir(name string) (string, error) {
	home, homeErr := os.UserHomeDir()

	// 1. Built-in "home" / "~"
	if name == "home" || name == "~" {
		if homeErr != nil {
			return "", fmt.Errorf("cannot determine home directory: %w", homeErr)
		}
		return home, nil
	}

	// 2. Expand ~/path
	if strings.HasPrefix(name, "~/") {
		if homeErr != nil {
			return "", fmt.Errorf("cannot determine home directory: %w", homeErr)
		}
		return filepath.Join(home, name[2:]), nil
	}

	// 3. Config shortcuts
	if m.config.Shortcuts != nil {
		if path, ok := m.config.Shortcuts[name]; ok {
			if strings.HasPrefix(path, "~/") && homeErr == nil {
				path = filepath.Join(home, path[2:])
			}
			return path, nil
		}
	}

	// 4. Auto-discover simple names (no path separator)
	if !strings.Contains(name, string(filepath.Separator)) && homeErr == nil {
		// Check ~/name
		direct := filepath.Join(home, name)
		if info, err := os.Stat(direct); err == nil && info.IsDir() {
			return direct, nil
		}

		// Search ~/*/name, ~/*/*/name, ... up to configured depth
		depth := m.config.DiscoverDepth
		if depth <= 0 {
			depth = 3
		}
		var dirs []string
		for level := 1; level <= depth; level++ {
			pattern := home
			for i := 0; i < level; i++ {
				pattern = filepath.Join(pattern, "*")
			}
			pattern = filepath.Join(pattern, name)
			matches, _ := filepath.Glob(pattern)
			for _, match := range matches {
				if info, err := os.Stat(match); err == nil && info.IsDir() {
					dirs = append(dirs, match)
				}
			}
		}
		if len(dirs) == 1 {
			return dirs[0], nil
		}
		if len(dirs) > 1 {
			return "", fmt.Errorf("multiple matches for %q:\n• %s\nUse the full path or configure a shortcut", name, strings.Join(dirs, "\n• "))
		}
	}

	// 5. Return as-is
	return name, nil
}

// GetSession returns the active session for a chat, or nil if none.
func (m *Manager) GetSession(platform, chatID string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[sessionKey(platform, chatID)]
}

// HasSession returns true if there is an active session for the chat.
func (m *Manager) HasSession(platform, chatID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.sessions[sessionKey(platform, chatID)]
	return ok
}

// CloseSession removes an active agent session.
func (m *Manager) CloseSession(platform, chatID string) {
	key := sessionKey(platform, chatID)
	m.mu.Lock()
	delete(m.sessions, key)
	m.mu.Unlock()

	m.logger.Info("agent session closed", "platform", platform, "chat_id", chatID)
}

// Execute runs a message through the agent CLI and returns the output.
// On timeout, it automatically retries with --continue up to MaxRetries times,
// accumulating partial output across attempts.
// When onProgress is non-nil and the agent is Claude, stdout is streamed as
// NDJSON events so tool-use progress can be forwarded to the user in real time.
func (m *Manager) Execute(ctx context.Context, sess *Session, message string, media []string, onProgress func(string)) (string, error) {
	sess.Touch()
	timeout := time.Duration(m.config.Timeout) * time.Second
	maxRetries := m.config.MaxRetries
	streaming := onProgress != nil && sess.Agent == AgentClaude

	var allOutput []string

	for attempt := 0; attempt <= maxRetries; attempt++ {
		var attemptCtx context.Context
		var cancel context.CancelFunc
		var idle *time.Timer

		if streaming {
			// Idle-based timeout: resets every time Claude sends output.
			attemptCtx, cancel = context.WithCancel(ctx)
			idle = time.AfterFunc(timeout, cancel)
		} else {
			attemptCtx, cancel = context.WithTimeout(ctx, timeout)
		}

		args := buildArgs(sess, message, media, m.config.GetCLISettings, m.config.PlanDelegate)
		if streaming {
			for i, arg := range args {
				if arg == "--output-format" && i+1 < len(args) {
					args[i+1] = "stream-json"
					break
				}
			}
			args = append(args, "--verbose")
		}
		bin := agentInfo[sess.Agent]

		m.logger.Debug("executing agent",
			"agent", sess.Agent, "dir", sess.Dir,
			"attempt", attempt, "msg_count", sess.MsgCount,
			"streaming", streaming,
		)

		cmd := exec.CommandContext(attemptCtx, bin, args...)
		cmd.Dir = sess.Dir

		var stderr bytes.Buffer
		cmd.Stderr = &limitedWriter{w: &stderr, n: maxOutputBytes}

		var partial string
		var err error

		if streaming {
			var pipe io.ReadCloser
			pipe, err = cmd.StdoutPipe()
			if err != nil {
				idle.Stop()
				cancel()
				return "", fmt.Errorf("stdout pipe: %w", err)
			}
			if err = cmd.Start(); err != nil {
				idle.Stop()
				cancel()
				return "", fmt.Errorf("start agent: %w", err)
			}
			heartbeat := func() { idle.Reset(timeout) }
			partial = m.readStreamEvents(pipe, onProgress, heartbeat, DefaultTemplates)
			err = cmd.Wait()
		} else {
			var stdout bytes.Buffer
			cmd.Stdout = &limitedWriter{w: &stdout, n: maxOutputBytes}
			err = cmd.Run()
			partial = strings.TrimSpace(StripANSI(stdout.String()))
		}
		// Save timeout state BEFORE cancel — cancel() always sets Err() non-nil
		timedOut := attemptCtx.Err() != nil
		cancel()
		if idle != nil {
			idle.Stop()
		}

		// Always increment so next attempt/call uses --continue
		sess.mu.Lock()
		sess.MsgCount++
		sess.mu.Unlock()
		if partial != "" {
			allOutput = append(allOutput, partial)
		}

		if err == nil {
			return strings.Join(allOutput, "\n"), nil
		}

		// Parent context canceled (e.g. shutdown) — don't retry
		if ctx.Err() != nil {
			combined := strings.Join(allOutput, "\n")
			return combined, fmt.Errorf("agent %s: %s", sess.Agent, ctx.Err())
		}

		// Non-timeout error — don't retry
		if !timedOut {
			errMsg := strings.TrimSpace(stderr.String())
			if errMsg == "" {
				errMsg = err.Error()
			}
			combined := strings.Join(allOutput, "\n")
			if combined != "" {
				return combined, fmt.Errorf("agent %s: %s", sess.Agent, errMsg)
			}
			return "", fmt.Errorf("agent %s: %s", sess.Agent, errMsg)
		}

		// Internal timeout — auto-retry with continue
		if attempt < maxRetries {
			m.logger.Info("agent timed out, auto-retrying",
				"agent", sess.Agent, "attempt", attempt+1, "max_retries", maxRetries,
			)
			message = "continue"
			media = nil
			continue
		}
	}

	// All retries exhausted — return as output (not error) so user sees
	// a clean message without "Agent error:" prefix
	combined := strings.Join(allOutput, "\n")
	timeoutNotice := fmt.Sprintf("\n\n⏱ Agent timed out after %d attempts (%v each). Send a message to continue, or :quit to end session.", maxRetries+1, timeout)
	if combined != "" {
		return combined + timeoutNotice, nil
	}
	return strings.TrimPrefix(timeoutNotice, "\n\n"), nil
}

// GetMsgCount returns the session message count (thread-safe).
func (s *Session) GetMsgCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.MsgCount
}

// Touch updates the session's last activity timestamp.
func (s *Session) Touch() {
	s.mu.Lock()
	s.LastActivity = time.Now()
	s.mu.Unlock()
}

// GetLastActivity returns the session's last activity timestamp (thread-safe).
func (s *Session) GetLastActivity() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.LastActivity
}

// idleCleanupLoop periodically sweeps idle sessions.
func (m *Manager) idleCleanupLoop() {
	interval := time.Duration(m.config.SessionTimeout) * time.Second / 6
	if interval < time.Minute {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.sweepIdleSessions()
		case <-m.done:
			return
		}
	}
}

// sweepIdleSessions closes sessions that have been idle longer than SessionTimeout.
func (m *Manager) sweepIdleSessions() {
	timeout := time.Duration(m.config.SessionTimeout) * time.Second
	now := time.Now()

	m.mu.Lock()
	var expired []*Session
	for key, sess := range m.sessions {
		sess.mu.Lock()
		idle := now.Sub(sess.LastActivity)
		sess.mu.Unlock()
		if idle > timeout {
			expired = append(expired, sess)
			delete(m.sessions, key)
		}
	}
	m.mu.Unlock()

	for _, sess := range expired {
		m.logger.Info("agent session expired (idle timeout)",
			"platform", sess.Platform,
			"chat_id", sess.ChatID,
			"agent", sess.Agent,
			"idle", now.Sub(sess.LastActivity).Truncate(time.Second).String(),
		)
		if m.config.OnSessionClose != nil {
			m.config.OnSessionClose(sess.Platform, sess.ChatID,
				fmt.Sprintf("⏱ Agent session (%s in %s) closed — idle for more than %s.",
					sess.Agent, sess.Dir, timeout.Truncate(time.Second)))
		}
	}
}

// planDelegatePrompt instructs Claude to plan first, then delegate to subagents.
const planDelegatePrompt = `Decide whether a task needs planning:

This workflow is ONLY for code/engineering tasks that modify the codebase.

DO plan (default for any non-trivial change):
- Multi-file changes, refactors, new features, architecture changes
- Tasks that require codebase investigation or analysis (e.g. "find X and fix it", "deduplicate", "optimize")
- Anything where you need to read/explore before knowing the full scope
- Any change where listing affected files upfront would help the user decide

SKIP plan (just do it directly):
- Non-coding requests (travel, writing, general questions, conversation)
- Truly trivial changes: typo fixes, single-line edits where scope is already obvious
- Explanations, code lookups, questions about the code
- User explicitly says "just do it" or "no need to plan"

When in doubt, plan. It is always better to show a plan and have the user say "just do it" than to make changes without agreement.

For tasks that need planning, follow this workflow:

PHASE 1 — PLAN (do this now, then STOP):
1. Analyze the codebase to understand what needs to change. Use subagents to explore in parallel when multiple areas need investigation; do it yourself for smaller scopes.
2. Produce a plan with:
   - Summary of what you found (the problem/opportunity)
   - Numbered list of changes, each with the file path and what will change
   - Any trade-offs or alternatives you considered
3. Output the plan and STOP. Do NOT write, edit, or create any files yet. End your response asking for confirmation with a (y/n) hint, in the same language the user is using.

CRITICAL: Do not touch any code during Phase 1. No edits, no writes, no file creation. Planning means research and presenting a proposal only.

PHASE 2 — USER CONFIRMATION:
Based on the user's intent (in whatever language they use):
- Confirms → proceed to Phase 3.
- Confirms with modifications → adjust accordingly and proceed.
- Declines with feedback → revise the plan, output the updated plan, and ask for confirmation again.
- Wants to stop/cancel → acknowledge and end. Do not continue.

PHASE 3 — IMPLEMENT (only after user confirms):
1. Implement the confirmed plan. Use subagents for independent steps that can run in parallel; do sequential/dependent steps yourself.
2. After implementation, build/test to confirm everything works.
3. Give a brief summary of what was done.

When to use subagents: independent work that can run in parallel (e.g. editing unrelated files, exploring separate packages, running different checks).
When NOT to use subagents: sequential/dependent changes, small scope (1-2 files), or when order matters.`

// buildArgs constructs CLI arguments for the given agent and message.
func buildArgs(sess *Session, message string, media []string, getCLI CLISettings, planDelegate bool) []string {
	// Prepend file references so the agent can read them
	if len(media) > 0 {
		var parts []string
		parts = append(parts, "User sent files (use Read tool to view them):")
		for _, path := range media {
			parts = append(parts, "  "+path)
		}
		if message != "" {
			parts = append(parts, "", message)
		}
		message = strings.Join(parts, "\n")
	}

	count := sess.GetMsgCount()
	switch sess.Agent {
	case AgentClaude:
		args := []string{"-p", message, "--output-format", "text", "--dangerously-skip-permissions"}
		if count > 0 {
			args = append(args, "--continue")
		}
		if planDelegate && count == 0 {
			args = append(args, "--append-system-prompt", planDelegatePrompt)
		}
		// Apply current model/effort from /model and /effort commands
		if getCLI != nil {
			model, effort := getCLI()
			if model != "" {
				args = append(args, "--model", model)
			}
			if effort != "" {
				args = append(args, "--effort", effort)
			}
		}
		return args
	case AgentCodex:
		return []string{"exec", "--", message}
	case AgentGemini:
		return []string{"-p", "--", message}
	default:
		return []string{"--", message}
	}
}

// StripANSI removes ANSI escape sequences from a string.
func StripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

// limitedWriter wraps a writer and stops after n bytes, preventing OOM.
type limitedWriter struct {
	w io.Writer
	n int64
}

func (lw *limitedWriter) Write(p []byte) (int, error) {
	if lw.n <= 0 {
		return len(p), nil // discard excess silently
	}
	if int64(len(p)) > lw.n {
		p = p[:lw.n]
	}
	n, err := lw.w.Write(p)
	lw.n -= int64(n)
	return n, err
}

// --- Claude stream-json event parsing ---

// streamEvent represents a parsed event from Claude's stream-json output.
type streamEvent struct {
	Type    string         `json:"type"`
	Message *streamMessage `json:"message,omitempty"`
	Result  string         `json:"result,omitempty"`
}

type streamMessage struct {
	Content []contentBlock `json:"content"`
}

type contentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

// DefaultTemplates provides English fallback progress messages.
// Keys use placeholders: {file}, {pattern}, {command}, {description}, {elapsed}.
var DefaultTemplates = map[string]string{
	"read_file":     "Let me take a look at {file}",
	"edit_file":     "I found something to fix in {file}, updating it now",
	"write_file":    "Creating a new file: {file}",
	"search_files":  "Looking for files matching {pattern}",
	"search_code":   "Searching the codebase for '{pattern}'",
	"run_command":   "Running a command: {command}",
	"run_described": "Hold on, I need to {description}",
	"generic":       "Hold on, I'm analyzing something",
	"still_working": "Still working on it, been {elapsed} so far",
}

// readStreamEvents reads Claude's stream-json output, sends progress notifications
// for tool-use events, and returns the final result text.
// When heartbeat is non-nil it is called on every parsed event so the caller
// can reset an idle timer.
func (m *Manager) readStreamEvents(r io.Reader, onProgress func(string), heartbeat func(), templates map[string]string) string {
	scanner := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, maxOutputBytes)

	var result string
	var textContent strings.Builder
	var lastNotify time.Time
	var lastMsg string
	const notifyInterval = 30 * time.Second

	for scanner.Scan() {
		var evt streamEvent
		if err := json.Unmarshal(scanner.Bytes(), &evt); err != nil {
			continue
		}
		if heartbeat != nil {
			heartbeat()
		}

		switch evt.Type {
		case "assistant":
			if evt.Message == nil {
				continue
			}
			for _, block := range evt.Message.Content {
				switch block.Type {
				case "tool_use":
					if time.Since(lastNotify) >= notifyInterval {
						msg := formatToolUse(block.Name, block.Input, templates)
						if msg != lastMsg {
							onProgress(msg)
							lastNotify = time.Now()
							lastMsg = msg
						}
					}
				case "text":
					textContent.WriteString(block.Text)
				}
			}
		case "result":
			result = evt.Result
		}
	}

	if result != "" {
		return strings.TrimSpace(result)
	}
	return strings.TrimSpace(textContent.String())
}

// tpl returns a template value, falling back to DefaultTemplates.
func tpl(templates map[string]string, key string) string {
	if v, ok := templates[key]; ok && v != "" {
		return v
	}
	return DefaultTemplates[key]
}

// formatToolUse formats a tool-use event using the given templates.
func formatToolUse(name string, input json.RawMessage, templates map[string]string) string {
	switch name {
	case "Read":
		var p struct {
			FilePath string `json:"file_path"`
		}
		_ = json.Unmarshal(input, &p)
		if p.FilePath != "" {
			return strings.ReplaceAll(tpl(templates, "read_file"), "{file}", filepath.Base(p.FilePath))
		}
	case "Edit":
		var p struct {
			FilePath string `json:"file_path"`
		}
		_ = json.Unmarshal(input, &p)
		if p.FilePath != "" {
			return strings.ReplaceAll(tpl(templates, "edit_file"), "{file}", filepath.Base(p.FilePath))
		}
	case "Write":
		var p struct {
			FilePath string `json:"file_path"`
		}
		_ = json.Unmarshal(input, &p)
		if p.FilePath != "" {
			return strings.ReplaceAll(tpl(templates, "write_file"), "{file}", filepath.Base(p.FilePath))
		}
	case "Bash":
		var p struct {
			Command     string `json:"command"`
			Description string `json:"description"`
		}
		_ = json.Unmarshal(input, &p)
		if p.Description != "" {
			desc := p.Description
			if len(desc) > 80 {
				desc = desc[:80] + "..."
			}
			return strings.ReplaceAll(tpl(templates, "run_described"), "{description}", strings.ToLower(desc))
		}
		if p.Command != "" {
			cmd := p.Command
			if len(cmd) > 50 {
				cmd = cmd[:50] + "..."
			}
			return strings.ReplaceAll(tpl(templates, "run_command"), "{command}", cmd)
		}
	case "Glob":
		var p struct {
			Pattern string `json:"pattern"`
		}
		_ = json.Unmarshal(input, &p)
		if p.Pattern != "" {
			return strings.ReplaceAll(tpl(templates, "search_files"), "{pattern}", p.Pattern)
		}
	case "Grep":
		var p struct {
			Pattern string `json:"pattern"`
		}
		_ = json.Unmarshal(input, &p)
		if p.Pattern != "" {
			return strings.ReplaceAll(tpl(templates, "search_code"), "{pattern}", p.Pattern)
		}
	}
	return tpl(templates, "generic")
}

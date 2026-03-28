// Package agent manages coding agent sessions (Claude Code, Codex)
// that users can control through chat commands.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/kusandriadi/allm-go"
	"github.com/kusandriadi/allm-go/provider"
)

// Supported agent types.
const (
	AgentClaude = "claude"
	AgentCodex  = "codex"
)

// CLISettings returns the current effort for Claude CLI.
// This is called on each execution to pick up runtime changes from /effort.
type CLISettings func() (effort string)

// NotifyFunc sends a message to a specific platform+chat (for idle timeout notifications).
type NotifyFunc func(platform, chatID, message string)

// Config holds agent manager settings.
type Config struct {
	Main                string            // main/primary agent type
	Timeout             int               // execution timeout in seconds
	MaxRetries          int               // auto-retry on timeout (0 = no retry)
	SessionTimeout      int               // idle session timeout in seconds (0 = disabled, default 21600 = 6h)
	AllowedDirs         []string          // directories users may target (empty = user home only)
	Shortcuts           map[string]string // directory shortcuts, e.g. "myproject": "~/code/myproject"
	DiscoverDepth       int               // auto-discover search depth (default 3)
	GetCLISettings      CLISettings       // optional: returns current effort for Claude CLI
	PlanDelegate        bool              // plan first, then delegate to subagents
	OnSessionClose      NotifyFunc        // optional: called when a session is auto-closed
	CLIPath             string            // path to claude binary (default: "claude")
	PlanModel       string            // model for planning phase (overrides default during plan)
	ImplModel string            // model for implementation phase (overrides default during impl)
}

// Session represents an active agent session tied to a chat.
type Session struct {
	mu           sync.Mutex
	Agent        string // agent type: claude, codex
	Dir          string // working directory (resolved absolute path)
	Platform     string
	ChatID       string
	UserID       string
	MsgCount     int                         // tracks messages sent (for --continue)
	LastActivity time.Time                   // last Execute() call (for idle timeout)
	cli          *provider.ClaudeCLIProvider // Claude CLI provider (nil for non-Claude agents)
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
		return nil, fmt.Errorf("unknown agent %q (supported: claude, codex)", agent)
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

	// Create Claude CLI provider for Claude agent sessions
	if agent == AgentClaude {
		cliPath := m.config.CLIPath
		if cliPath == "" {
			cliPath = "claude"
		}
		sess.cli = provider.ClaudeCLI(
			provider.WithCLIPath(cliPath),
			provider.WithCLIWorkDir(absDir),
			provider.WithCLISessionPersist(true),
		)
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

// Execute runs a message through the agent and returns the output.
// For Claude agents, uses allm-go ClaudeCLIProvider. For Codex, uses direct exec.
// On timeout, it automatically retries with --continue up to MaxRetries times.
func (m *Manager) Execute(ctx context.Context, sess *Session, message string, media []string, onProgress func(string)) (string, error) {
	sess.Touch()
	timeout := time.Duration(m.config.Timeout) * time.Second
	maxRetries := m.config.MaxRetries

	if sess.Agent == AgentClaude && sess.cli != nil {
		return m.executeClaude(ctx, sess, message, media, onProgress, timeout, maxRetries)
	}
	return m.executeCodex(ctx, sess, message, timeout)
}

// executeClaude runs a message through Claude CLI via allm-go provider.
func (m *Manager) executeClaude(ctx context.Context, sess *Session, message string, media []string, onProgress func(string), timeout time.Duration, maxRetries int) (string, error) {
	streaming := onProgress != nil

	var allOutput []string

	for attempt := 0; attempt <= maxRetries; attempt++ {
		var attemptCtx context.Context
		var cancel context.CancelFunc
		var idle *time.Timer

		if streaming {
			attemptCtx, cancel = context.WithCancel(ctx)
			idle = time.AfterFunc(timeout, cancel)
		} else {
			attemptCtx, cancel = context.WithTimeout(ctx, timeout)
		}

		// Configure provider for this attempt
		count := sess.GetMsgCount()
		sess.cli.SetContinue(count > 0)

		if m.config.PlanDelegate && count == 0 {
			sess.cli.SetAppendPrompt(planDelegatePrompt)
		} else {
			sess.cli.SetAppendPrompt("")
		}

		// Build request with phase-aware model selection
		req := m.buildRequest(sess, message, media, count)

		m.logger.Debug("executing agent",
			"agent", sess.Agent, "dir", sess.Dir,
			"attempt", attempt, "msg_count", count,
			"streaming", streaming,
		)

		var partial string
		var err error

		if streaming {
			partial, err = m.streamClaude(attemptCtx, sess, req, onProgress, idle, timeout)
		} else {
			var resp *allm.Response
			resp, err = sess.cli.Complete(attemptCtx, req)
			if err == nil {
				partial = strings.TrimSpace(resp.Content)
			}
		}

		timedOut := attemptCtx.Err() != nil
		cancel()
		if idle != nil {
			idle.Stop()
		}

		sess.mu.Lock()
		sess.MsgCount++
		sess.mu.Unlock()
		if partial != "" {
			allOutput = append(allOutput, partial)
		}

		if err == nil {
			return strings.Join(allOutput, "\n"), nil
		}

		if ctx.Err() != nil {
			combined := strings.Join(allOutput, "\n")
			return combined, fmt.Errorf("agent %s: %s", sess.Agent, ctx.Err())
		}

		if !timedOut {
			combined := strings.Join(allOutput, "\n")
			if combined != "" {
				return combined, fmt.Errorf("agent %s: %s", sess.Agent, err)
			}
			return "", fmt.Errorf("agent %s: %s", sess.Agent, err)
		}

		if attempt < maxRetries {
			m.logger.Info("agent timed out, auto-retrying",
				"agent", sess.Agent, "attempt", attempt+1, "max_retries", maxRetries,
			)
			message = "continue"
			media = nil
			continue
		}
	}

	combined := strings.Join(allOutput, "\n")
	timeoutNotice := fmt.Sprintf("\n\n⏱ Agent timed out after %d attempts (%v each). Send a message to continue, or :quit to end session.", maxRetries+1, timeout)
	if combined != "" {
		return combined + timeoutNotice, nil
	}
	return strings.TrimPrefix(timeoutNotice, "\n\n"), nil
}

// buildRequest constructs an allm.Request for the Claude CLI provider.
// count is the current message count: 0 = planning phase, >0 = implementation phase.
func (m *Manager) buildRequest(sess *Session, message string, media []string, count int) *allm.Request {
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

	req := &allm.Request{
		Messages: []allm.Message{
			{Role: allm.RoleUser, Content: message},
		},
	}

	// Phase-aware model selection:
	// planning phase (count == 0) → PlanModel
	// implementation phase (count > 0) → ImplModel
	if m.config.PlanDelegate && count == 0 && m.config.PlanModel != "" {
		req.Model = m.config.PlanModel
	} else if m.config.ImplModel != "" {
		req.Model = m.config.ImplModel
	}

	// Effort from /effort command
	if m.config.GetCLISettings != nil {
		if effort := m.config.GetCLISettings(); effort != "" {
			req.Effort = effort
		}
	}

	return req
}

// streamClaude reads streaming output from Claude CLI via allm-go.
func (m *Manager) streamClaude(ctx context.Context, sess *Session, req *allm.Request, onProgress func(string), idle *time.Timer, timeout time.Duration) (string, error) {
	ch := sess.cli.Stream(ctx, req)

	var textContent strings.Builder
	var lastNotify time.Time
	var lastMsg string
	const notifyInterval = 30 * time.Second

	for chunk := range ch {
		if chunk.Error != nil {
			return textContent.String(), chunk.Error
		}
		if chunk.Done {
			break
		}

		// Reset idle timer on every chunk
		if idle != nil {
			idle.Reset(timeout)
		}

		if chunk.ToolUse != nil && onProgress != nil {
			if time.Since(lastNotify) >= notifyInterval {
				msg := formatToolUse(chunk.ToolUse.Name, chunk.ToolUse.Input, DefaultTemplates)
				if msg != lastMsg {
					onProgress(msg)
					lastNotify = time.Now()
					lastMsg = msg
				}
			}
		}

		if chunk.Content != "" {
			textContent.WriteString(chunk.Content)
		}
	}

	return strings.TrimSpace(textContent.String()), nil
}

// executeCodex runs a message through Codex via direct exec.
func (m *Manager) executeCodex(ctx context.Context, sess *Session, message string, timeout time.Duration) (string, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{"exec", "--", message}
	bin := agentInfo[sess.Agent]

	cmd := exec.CommandContext(attemptCtx, bin, args...)
	cmd.Dir = sess.Dir

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("agent %s: %w", sess.Agent, err)
	}
	return strings.TrimSpace(StripANSI(string(out))), nil
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
const planDelegatePrompt = `First, decide whether this task needs planning or can be handled directly.

SKIP PLANNING — respond directly when:
- Questions or conversations: "what does X do?", "explain this", "how does this work?"
- Information lookups: "find where X is defined", "show me the config", "what version are we on?"
- Non-coding requests: general knowledge, writing, brainstorming, advice
- Trivial single-file changes where the scope is already obvious: typo fix, rename a variable, update a string
- The user explicitly says "just do it", "no plan needed", "langsung aja"
- Debugging help: "why is this failing?", "what's wrong with this code?"
- Code review or feedback: "review this PR", "is this approach good?"

For these, just answer or do the work immediately. No plan needed.

DO PLAN — when the task modifies the codebase AND:
- Touches multiple files or packages
- Requires investigation before knowing the full scope (e.g. "find all usages of X and update them")
- Involves new features, refactors, architecture changes, or dependency updates
- Could have trade-offs or alternative approaches worth discussing
- The impact is unclear without reading the code first

When genuinely unsure, lean toward planning — it's better to show a quick plan than to make unwanted changes.

For tasks that need planning, follow this workflow:

PHASE 1 — PLAN (do this now, then STOP):
1. Investigate the codebase to understand scope. Use subagents to explore in parallel when multiple areas need investigation; do it yourself for smaller scopes.
2. Present a concise plan:
   - What you found (the problem/opportunity)
   - Numbered list of changes with file paths
   - Trade-offs or alternatives (if any)
3. STOP. Do NOT write, edit, or create any files. Ask for confirmation with a (y/n) hint, in the same language the user is using.

CRITICAL: No code changes during Phase 1. Planning = research + proposal only.

PHASE 2 — USER CONFIRMATION:
Based on the user's response (in whatever language they use):
- Confirms → proceed to Phase 3.
- Confirms with modifications → adjust and proceed.
- Declines with feedback → revise the plan and ask again.
- Wants to stop/cancel → acknowledge and end.

PHASE 3 — IMPLEMENT (only after user confirms):
1. Implement the confirmed plan. Use subagents for independent steps that can run in parallel; do sequential/dependent steps yourself.
2. After implementation, build/test to confirm everything works.
3. Give a brief summary of what was done.

When to use subagents: independent work that can run in parallel (e.g. editing unrelated files, exploring separate packages, running different checks).
When NOT to use subagents: sequential/dependent changes, small scope (1-2 files), or when order matters.`

// StripANSI removes ANSI escape sequences from a string.
func StripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
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

// Package agent manages coding agent sessions (Claude Code, Codex, Gemini CLI)
// that users can control through chat commands.
package agent

import (
	"bytes"
	"context"
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

// Config holds agent manager settings.
type Config struct {
	Main       string   // main/primary agent type
	Timeout    int      // execution timeout in seconds
	AllowedDirs []string // directories users may target (empty = user home only)
}

// Session represents an active agent session tied to a chat.
type Session struct {
	mu       sync.Mutex
	Agent    string // agent type: claude, codex, gemini
	Dir      string // working directory (resolved absolute path)
	Platform string
	ChatID   string
	UserID   string
	MsgCount int // tracks messages sent (for --continue)
}

// Manager manages agent sessions across chats.
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session // key: "platform:chatID"
	config   Config
	logger   *slog.Logger
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
		cfg.Timeout = 120
	}
	return &Manager{
		sessions: make(map[string]*Session),
		config:   cfg,
		logger:   logger,
	}
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
		Agent:    agent,
		Dir:      absDir,
		Platform: platform,
		ChatID:   chatID,
		UserID:   userID,
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
// Each call is a one-shot CLI invocation (no persistent process).
func (m *Manager) Execute(ctx context.Context, sess *Session, message string) (string, error) {
	timeout := time.Duration(m.config.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := buildArgs(sess, message)
	bin := agentInfo[sess.Agent]

	m.logger.Debug("executing agent",
		"agent", sess.Agent, "dir", sess.Dir,
		"msg_count", sess.MsgCount,
	)

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = sess.Dir

	// Limit stdout/stderr capture to prevent OOM
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitedWriter{w: &stdout, n: maxOutputBytes}
	cmd.Stderr = &limitedWriter{w: &stderr, n: maxOutputBytes}

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return "", fmt.Errorf("agent %s: %s", sess.Agent, errMsg)
	}

	// Thread-safe MsgCount increment
	sess.mu.Lock()
	sess.MsgCount++
	sess.mu.Unlock()

	output := StripANSI(stdout.String())
	output = strings.TrimSpace(output)

	return output, nil
}

// GetMsgCount returns the session message count (thread-safe).
func (s *Session) GetMsgCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.MsgCount
}

// buildArgs constructs CLI arguments for the given agent and message.
func buildArgs(sess *Session, message string) []string {
	count := sess.GetMsgCount()
	switch sess.Agent {
	case AgentClaude:
		args := []string{"-p", message, "--output-format", "text"}
		if count > 0 {
			args = append(args, "--continue")
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

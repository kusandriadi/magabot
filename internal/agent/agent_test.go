package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNewManager(t *testing.T) {
	m := NewManager(Config{}, nil)
	if m.config.Main != AgentClaude {
		t.Errorf("default agent = %q, want %q", m.config.Main, AgentClaude)
	}
	if m.config.Timeout != 120 {
		t.Errorf("default timeout = %d, want 120", m.config.Timeout)
	}
}

func TestNewManagerCustom(t *testing.T) {
	m := NewManager(Config{Main: "codex", Timeout: 60}, nil)
	if m.config.Main != "codex" {
		t.Errorf("default = %q, want codex", m.config.Main)
	}
	if m.config.Timeout != 60 {
		t.Errorf("timeout = %d, want 60", m.config.Timeout)
	}
}

func TestSessionCRUD(t *testing.T) {
	dir := t.TempDir()
	home, _ := os.UserHomeDir()
	m := NewManager(Config{AllowedDirs: []string{os.TempDir(), home}}, nil)

	// No session initially
	if m.HasSession("telegram", "123") {
		t.Error("expected no session")
	}
	if s := m.GetSession("telegram", "123"); s != nil {
		t.Error("expected nil session")
	}

	// NewSession requires a valid binary
	sess, err := m.NewSession("telegram", "123", "user1", "claude", dir)
	if err != nil {
		if _, lookErr := exec.LookPath("claude"); lookErr != nil {
			t.Skip("claude binary not in PATH, skipping session create test")
		}
		t.Fatalf("NewSession: %v", err)
	}

	if !m.HasSession("telegram", "123") {
		t.Error("expected session to exist")
	}
	if got := m.GetSession("telegram", "123"); got != sess {
		t.Error("GetSession returned different session")
	}

	// Close session
	m.CloseSession("telegram", "123")
	if m.HasSession("telegram", "123") {
		t.Error("expected session to be removed")
	}
}

func TestNewSessionInvalidAgent(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(Config{AllowedDirs: []string{os.TempDir()}}, nil)

	_, err := m.NewSession("telegram", "123", "user1", "invalid", dir)
	if err == nil {
		t.Error("expected error for invalid agent")
	}
}

func TestNewSessionInvalidDir(t *testing.T) {
	m := NewManager(Config{AllowedDirs: []string{os.TempDir()}}, nil)

	_, err := m.NewSession("telegram", "123", "user1", "claude", "/nonexistent/path/xyz")
	if err == nil {
		t.Error("expected error for nonexistent dir")
	}
}

func TestNewSessionNotADir(t *testing.T) {
	f, err := os.CreateTemp("", "agent-test")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	defer os.Remove(f.Name())

	m := NewManager(Config{AllowedDirs: []string{os.TempDir()}}, nil)

	_, err = m.NewSession("telegram", "123", "user1", "claude", f.Name())
	if err == nil {
		t.Error("expected error for non-directory path")
	}
}

func TestNewSessionDisallowedDir(t *testing.T) {
	safeOnly := filepath.Join(t.TempDir(), "safe-only")
	m := NewManager(Config{AllowedDirs: []string{safeOnly}}, nil)

	// os.TempDir() itself is not under safeOnly
	_, err := m.NewSession("telegram", "123", "user1", "claude", os.TempDir())
	if err == nil {
		t.Error("expected error for disallowed directory")
	}
}

func TestValidateDir(t *testing.T) {
	home, _ := os.UserHomeDir()
	m := NewManager(Config{}, nil)

	// Home dir should be allowed by default
	if err := m.validateDir(home); err != nil {
		t.Errorf("home dir should be allowed: %v", err)
	}

	// Subdir of home should be allowed
	sub := filepath.Join(home, "projects")
	if err := m.validateDir(sub); err != nil {
		t.Errorf("home subdir should be allowed: %v", err)
	}

	// Root should NOT be allowed
	root := "/"
	if runtime.GOOS == "windows" {
		root = filepath.VolumeName(home) + string(filepath.Separator)
	}
	if err := m.validateDir(root); err == nil {
		t.Error("root should not be allowed")
	}
}

func TestValidAgent(t *testing.T) {
	if !ValidAgent("claude") {
		t.Error("claude should be valid")
	}
	if !ValidAgent("codex") {
		t.Error("codex should be valid")
	}
	if !ValidAgent("gemini") {
		t.Error("gemini should be valid")
	}
	if ValidAgent("unknown") {
		t.Error("unknown should not be valid")
	}
}

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain text", "hello world", "hello world"},
		{"color code", "\x1b[31mred\x1b[0m", "red"},
		{"bold", "\x1b[1mbold\x1b[0m", "bold"},
		{"multiple codes", "\x1b[1;32mgreen bold\x1b[0m normal", "green bold normal"},
		{"cursor movement", "\x1b[2Amoved", "moved"},
		{"osc sequence", "\x1b]0;title\x07text", "text"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripANSI(tt.input)
			if got != tt.want {
				t.Errorf("StripANSI(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildArgs(t *testing.T) {
	tests := []struct {
		agent   string
		count   int
		message string
		wantLen int
	}{
		{AgentClaude, 0, "hello", 4},  // -p hello --output-format text
		{AgentClaude, 1, "hello", 5},  // + --continue
		{AgentCodex, 0, "hello", 3},   // exec -- hello
		{AgentGemini, 0, "hello", 3},  // -p -- hello
	}

	for _, tt := range tests {
		t.Run(tt.agent, func(t *testing.T) {
			sess := &Session{Agent: tt.agent, MsgCount: tt.count}
			args := buildArgs(sess, tt.message)
			if len(args) != tt.wantLen {
				t.Errorf("buildArgs(%s, count=%d) len = %d, want %d: %v",
					tt.agent, tt.count, len(args), tt.wantLen, args)
			}
		})
	}
}

func TestBuildArgsFlagInjection(t *testing.T) {
	// Codex and Gemini use -- separator to prevent flag injection
	sess := &Session{Agent: AgentCodex}
	args := buildArgs(sess, "--malicious-flag")
	// Should be: exec -- --malicious-flag
	if len(args) < 2 || args[1] != "--" {
		t.Errorf("codex args should have -- separator: %v", args)
	}

	sess = &Session{Agent: AgentGemini}
	args = buildArgs(sess, "--malicious-flag")
	// Should be: -p -- --malicious-flag
	if len(args) < 2 || args[1] != "--" {
		t.Errorf("gemini args should have -- separator: %v", args)
	}
}

func TestExecuteCancelledContext(t *testing.T) {
	home, _ := os.UserHomeDir()
	m := NewManager(Config{Timeout: 10, AllowedDirs: []string{home}}, nil)
	dir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	sess := &Session{Agent: AgentClaude, Dir: dir}
	_, err := m.Execute(ctx, sess, "test")
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestSessionKey(t *testing.T) {
	tests := []struct {
		platform, chatID, want string
	}{
		{"telegram", "123", "telegram:123"},
		{"slack", "C0001", "slack:C0001"},
		{"whatsapp", "+1234", "whatsapp:+1234"},
	}

	for _, tt := range tests {
		got := sessionKey(tt.platform, tt.chatID)
		if got != tt.want {
			t.Errorf("sessionKey(%q, %q) = %q, want %q", tt.platform, tt.chatID, got, tt.want)
		}
	}
}

func TestLimitedWriter(t *testing.T) {
	var buf []byte
	w := &limitedWriter{w: writerFunc(func(p []byte) (int, error) {
		buf = append(buf, p...)
		return len(p), nil
	}), n: 5}

	_, _ = w.Write([]byte("hello world")) // 11 bytes, limit 5
	if string(buf) != "hello" {
		t.Errorf("limitedWriter wrote %q, want %q", string(buf), "hello")
	}

	// Further writes should be discarded
	_, _ = w.Write([]byte("more"))
	if string(buf) != "hello" {
		t.Errorf("limitedWriter should discard after limit, got %q", string(buf))
	}
}

func TestGetMsgCount(t *testing.T) {
	s := &Session{MsgCount: 5}
	if got := s.GetMsgCount(); got != 5 {
		t.Errorf("GetMsgCount() = %d, want 5", got)
	}
}

// writerFunc is a helper that implements io.Writer via a function.
type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

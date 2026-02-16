package bot

import (
	"os"
	"strings"
	"testing"

	"github.com/kusa/magabot/internal/config"
)

// newTestConfig creates a Config for testing with a temp file for saves.
func newTestConfig(t *testing.T) *config.Config {
	t.Helper()
	tmpDir := t.TempDir()
	cfgPath := tmpDir + "/config.yaml"

	// Write a minimal valid config
	data := []byte(`
bot:
  name: testbot
access:
  mode: allowlist
  global_admins:
    - admin1
platforms:
  telegram:
    enabled: true
    token: test
    admins:
      - admin1
    allowed_users:
      - admin1
      - user1
    allowed_chats: []
    allow_groups: true
    allow_dms: true
`)
	if err := os.WriteFile(cfgPath, data, 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("failed to load test config: %v", err)
	}
	return cfg
}

func TestHandleCommand_NoArgs(t *testing.T) {
	cfg := newTestConfig(t)
	h := NewAdminHandler(cfg, t.TempDir())

	response, restart, err := h.HandleCommand("telegram", "admin1", "chat1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if restart {
		t.Error("expected no restart for help")
	}
	if !strings.Contains(response, "Config Commands") {
		t.Errorf("expected help text, got: %s", response)
	}
}

func TestHandleCommand_Status(t *testing.T) {
	cfg := newTestConfig(t)
	h := NewAdminHandler(cfg, t.TempDir())

	response, restart, err := h.HandleCommand("telegram", "admin1", "chat1", []string{"status"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if restart {
		t.Error("expected no restart for status")
	}
	if !strings.Contains(response, "Config Status") {
		t.Errorf("expected status info, got: %s", response)
	}
	if !strings.Contains(response, "allowlist") {
		t.Errorf("expected mode in status, got: %s", response)
	}
}

func TestHandleCommand_Help(t *testing.T) {
	cfg := newTestConfig(t)
	h := NewAdminHandler(cfg, t.TempDir())

	response, restart, err := h.HandleCommand("telegram", "admin1", "chat1", []string{"help"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if restart {
		t.Error("expected no restart for help")
	}
	if !strings.Contains(response, "Config Commands") {
		t.Errorf("expected help text, got: %s", response)
	}
}

func TestHandleCommand_UnknownCommand(t *testing.T) {
	cfg := newTestConfig(t)
	h := NewAdminHandler(cfg, t.TempDir())

	response, restart, err := h.HandleCommand("telegram", "admin1", "chat1", []string{"foobar"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if restart {
		t.Error("expected no restart for unknown command")
	}
	if !strings.Contains(response, "Unknown command") {
		t.Errorf("expected unknown command error, got: %s", response)
	}
}

func TestHandleCommand_AllowUser(t *testing.T) {
	cfg := newTestConfig(t)
	h := NewAdminHandler(cfg, t.TempDir())

	response, _, err := h.HandleCommand("telegram", "admin1", "chat1", []string{"allow", "user", "newuser"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(response, "Allowed user") {
		t.Errorf("expected allow confirmation, got: %s", response)
	}
}

func TestHandleCommand_RemoveUser(t *testing.T) {
	cfg := newTestConfig(t)
	h := NewAdminHandler(cfg, t.TempDir())

	response, _, err := h.HandleCommand("telegram", "admin1", "chat1", []string{"remove", "user", "user1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(response, "Removed user") {
		t.Errorf("expected remove confirmation, got: %s", response)
	}
}

func TestHandleCommand_Mode(t *testing.T) {
	cfg := newTestConfig(t)
	h := NewAdminHandler(cfg, t.TempDir())

	response, _, err := h.HandleCommand("telegram", "admin1", "chat1", []string{"mode", "open"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(response, "Access mode set to") {
		t.Errorf("expected mode change confirmation, got: %s", response)
	}
}

func TestHandleCommand_AdminAdd(t *testing.T) {
	cfg := newTestConfig(t)
	h := NewAdminHandler(cfg, t.TempDir())

	response, _, err := h.HandleCommand("telegram", "admin1", "chat1", []string{"admin", "global", "add", "admin2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(response, "Added global admin") {
		t.Errorf("expected admin add confirmation, got: %s", response)
	}
}

func TestHandleCommand_AdminRemove(t *testing.T) {
	cfg := newTestConfig(t)
	h := NewAdminHandler(cfg, t.TempDir())

	// First add another admin so we can remove one
	h.HandleCommand("telegram", "admin1", "chat1", []string{"admin", "global", "add", "admin2"})

	response, _, err := h.HandleCommand("telegram", "admin1", "chat1", []string{"admin", "global", "rm", "admin2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(response, "Removed global admin") {
		t.Errorf("expected admin remove confirmation, got: %s", response)
	}
}

func TestHandleCommand_NonAdmin(t *testing.T) {
	cfg := newTestConfig(t)
	h := NewAdminHandler(cfg, t.TempDir())

	// Non-admin trying to change mode
	response, _, err := h.HandleCommand("telegram", "user1", "chat1", []string{"mode", "open"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(response, "Only global admins") {
		t.Errorf("expected permission denied, got: %s", response)
	}
}

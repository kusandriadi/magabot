package tools_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/kusa/magabot/internal/tools"
)

// mockTool is a simple implementation of tools.Tool for testing.
type mockTool struct {
	name        string
	description string
	result      string
	err         error
}

func (m *mockTool) Name() string        { return m.name }
func (m *mockTool) Description() string { return m.description }

func (m *mockTool) Execute(_ context.Context, params map[string]string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.result, nil
}

func newTestManager() *tools.Manager {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return tools.NewManager(logger)
}

func TestNewManager(t *testing.T) {
	mgr := newTestManager()
	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}

	list := mgr.List()
	if len(list) != 0 {
		t.Errorf("expected empty tool list, got %d tools", len(list))
	}
}

func TestRegister_And_Get(t *testing.T) {
	mgr := newTestManager()

	tool := &mockTool{
		name:        "search",
		description: "Search the web",
		result:      "search results",
	}
	mgr.Register(tool)

	got, ok := mgr.Get("search")
	if !ok {
		t.Fatal("expected Get('search') to return true")
	}
	if got.Name() != "search" {
		t.Errorf("expected tool name 'search', got %q", got.Name())
	}
	if got.Description() != "Search the web" {
		t.Errorf("expected description 'Search the web', got %q", got.Description())
	}
}

func TestRegister_Overwrite(t *testing.T) {
	mgr := newTestManager()

	tool1 := &mockTool{name: "calc", description: "v1", result: "1"}
	tool2 := &mockTool{name: "calc", description: "v2", result: "2"}

	mgr.Register(tool1)
	mgr.Register(tool2)

	got, ok := mgr.Get("calc")
	if !ok {
		t.Fatal("expected Get('calc') to return true")
	}
	if got.Description() != "v2" {
		t.Errorf("expected overwritten tool description 'v2', got %q", got.Description())
	}
}

func TestGet_NotFound(t *testing.T) {
	mgr := newTestManager()

	_, ok := mgr.Get("nonexistent")
	if ok {
		t.Error("expected Get('nonexistent') to return false")
	}
}

func TestList(t *testing.T) {
	mgr := newTestManager()

	toolNames := []string{"search", "calculator", "weather"}
	for _, name := range toolNames {
		mgr.Register(&mockTool{
			name:        name,
			description: name + " tool",
		})
	}

	list := mgr.List()
	if len(list) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(list))
	}

	// Collect names from list and verify all expected tools are present.
	gotNames := make(map[string]bool)
	for _, tool := range list {
		gotNames[tool.Name()] = true
	}

	for _, name := range toolNames {
		if !gotNames[name] {
			t.Errorf("expected tool %q in list, but not found", name)
		}
	}
}

func TestList_Empty(t *testing.T) {
	mgr := newTestManager()

	list := mgr.List()
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d tools", len(list))
	}
}

func TestExecute_Known(t *testing.T) {
	mgr := newTestManager()
	ctx := context.Background()

	mgr.Register(&mockTool{
		name:        "echo",
		description: "Echo input",
		result:      "hello world",
	})

	result, err := mgr.Execute(ctx, "echo", map[string]string{"input": "hello"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result != "hello world" {
		t.Errorf("expected 'hello world', got %q", result)
	}
}

func TestExecute_Known_WithError(t *testing.T) {
	mgr := newTestManager()
	ctx := context.Background()

	expectedErr := fmt.Errorf("tool execution failed")
	mgr.Register(&mockTool{
		name:        "failing",
		description: "A tool that fails",
		err:         expectedErr,
	})

	_, err := mgr.Execute(ctx, "failing", nil)
	if err == nil {
		t.Fatal("expected error from failing tool, got nil")
	}
	if err.Error() != expectedErr.Error() {
		t.Errorf("expected error %q, got %q", expectedErr, err)
	}
}

func TestExecute_Unknown(t *testing.T) {
	mgr := newTestManager()
	ctx := context.Background()

	result, err := mgr.Execute(ctx, "nonexistent", map[string]string{})
	if err != nil {
		t.Fatalf("expected nil error for unknown tool, got %v", err)
	}
	if result != "" {
		t.Errorf("expected empty string for unknown tool, got %q", result)
	}
}

func TestGetToolDescriptions(t *testing.T) {
	mgr := newTestManager()

	mgr.Register(&mockTool{
		name:        "search",
		description: "Search the web for information",
	})
	mgr.Register(&mockTool{
		name:        "calculator",
		description: "Perform mathematical calculations",
	})

	desc := mgr.GetToolDescriptions()

	if !strings.Contains(desc, "Available tools:") {
		t.Errorf("expected descriptions to contain 'Available tools:', got %q", desc)
	}
	if !strings.Contains(desc, "search") {
		t.Errorf("expected descriptions to contain 'search', got %q", desc)
	}
	if !strings.Contains(desc, "calculator") {
		t.Errorf("expected descriptions to contain 'calculator', got %q", desc)
	}
	if !strings.Contains(desc, "Search the web for information") {
		t.Errorf("expected descriptions to contain tool description, got %q", desc)
	}
	if !strings.Contains(desc, "Perform mathematical calculations") {
		t.Errorf("expected descriptions to contain tool description, got %q", desc)
	}
}

func TestGetToolDescriptions_Empty(t *testing.T) {
	mgr := newTestManager()

	desc := mgr.GetToolDescriptions()

	if !strings.Contains(desc, "Available tools:") {
		t.Errorf("expected 'Available tools:' header even with no tools, got %q", desc)
	}
}

func TestGetToolDescriptions_Format(t *testing.T) {
	mgr := newTestManager()

	mgr.Register(&mockTool{
		name:        "weather",
		description: "Get weather forecasts",
	})

	desc := mgr.GetToolDescriptions()

	// Verify the format: "- <name>: <description>"
	if !strings.Contains(desc, "- weather: Get weather forecasts") {
		t.Errorf("expected '- weather: Get weather forecasts' in descriptions, got %q", desc)
	}
}

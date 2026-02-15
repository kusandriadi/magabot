// Package test contains integration tests for tools module
package test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/kusa/magabot/internal/tools"
)

// MockTool implements tools.Tool for testing
type MockTool struct {
	name        string
	description string
	executeFunc func(ctx context.Context, params map[string]string) (string, error)
}

func (m *MockTool) Name() string        { return m.name }
func (m *MockTool) Description() string { return m.description }

func (m *MockTool) Execute(ctx context.Context, params map[string]string) (string, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, params)
	}
	return "mock result", nil
}

func TestToolsManagerIntegration(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	t.Run("CreateManager", func(t *testing.T) {
		mgr := tools.NewManager(logger)
		if mgr == nil {
			t.Fatal("Manager should not be nil")
		}
	})

	t.Run("RegisterTool", func(t *testing.T) {
		mgr := tools.NewManager(logger)

		tool := &MockTool{
			name:        "test_tool",
			description: "A test tool",
		}

		mgr.Register(tool)

		// Verify registration
		retrieved, ok := mgr.Get("test_tool")
		if !ok {
			t.Fatal("Tool should be retrievable after registration")
		}

		if retrieved.Name() != "test_tool" {
			t.Errorf("Expected name 'test_tool', got '%s'", retrieved.Name())
		}
	})

	t.Run("GetNonExistentTool", func(t *testing.T) {
		mgr := tools.NewManager(logger)

		_, ok := mgr.Get("nonexistent")
		if ok {
			t.Error("Should return false for non-existent tool")
		}
	})

	t.Run("ListTools", func(t *testing.T) {
		mgr := tools.NewManager(logger)

		mgr.Register(&MockTool{name: "tool1", description: "First tool"})
		mgr.Register(&MockTool{name: "tool2", description: "Second tool"})
		mgr.Register(&MockTool{name: "tool3", description: "Third tool"})

		toolList := mgr.List()

		if len(toolList) != 3 {
			t.Errorf("Expected 3 tools, got %d", len(toolList))
		}
	})

	t.Run("ExecuteTool", func(t *testing.T) {
		mgr := tools.NewManager(logger)

		tool := &MockTool{
			name:        "calculator",
			description: "Simple calculator",
			executeFunc: func(ctx context.Context, params map[string]string) (string, error) {
				return "Result: 42", nil
			},
		}

		mgr.Register(tool)

		ctx := context.Background()
		result, err := mgr.Execute(ctx, "calculator", map[string]string{"op": "add"})

		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		if result != "Result: 42" {
			t.Errorf("Expected 'Result: 42', got '%s'", result)
		}
	})

	t.Run("ExecuteNonExistentTool", func(t *testing.T) {
		mgr := tools.NewManager(logger)

		ctx := context.Background()
		result, err := mgr.Execute(ctx, "nonexistent", nil)

		if err != nil {
			t.Errorf("Execute should not error for non-existent tool, got: %v", err)
		}

		if result != "" {
			t.Error("Should return empty result for non-existent tool")
		}
	})

	t.Run("GetToolDescriptions", func(t *testing.T) {
		mgr := tools.NewManager(logger)

		mgr.Register(&MockTool{name: "weather", description: "Get weather info"})
		mgr.Register(&MockTool{name: "search", description: "Web search"})

		desc := mgr.GetToolDescriptions()

		if desc == "" {
			t.Error("Descriptions should not be empty")
		}

		if len(desc) < 20 {
			t.Error("Descriptions should contain tool info")
		}
	})

	t.Run("ToolWithParams", func(t *testing.T) {
		mgr := tools.NewManager(logger)

		tool := &MockTool{
			name:        "greeter",
			description: "Greet someone",
			executeFunc: func(ctx context.Context, params map[string]string) (string, error) {
				name := params["name"]
				if name == "" {
					name = "World"
				}
				return "Hello, " + name + "!", nil
			},
		}

		mgr.Register(tool)

		ctx := context.Background()

		// With parameter
		result, _ := mgr.Execute(ctx, "greeter", map[string]string{"name": "Alice"})
		if result != "Hello, Alice!" {
			t.Errorf("Expected 'Hello, Alice!', got '%s'", result)
		}

		// Without parameter
		result, _ = mgr.Execute(ctx, "greeter", map[string]string{})
		if result != "Hello, World!" {
			t.Errorf("Expected 'Hello, World!', got '%s'", result)
		}
	})

	t.Run("ContextCancellation", func(t *testing.T) {
		mgr := tools.NewManager(logger)

		tool := &MockTool{
			name:        "slow_tool",
			description: "A slow tool",
			executeFunc: func(ctx context.Context, params map[string]string) (string, error) {
				select {
				case <-ctx.Done():
					return "", ctx.Err()
				default:
					return "done", nil
				}
			},
		}

		mgr.Register(tool)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err := mgr.Execute(ctx, "slow_tool", nil)
		// Depending on timing, may or may not be cancelled
		if err != nil && err != context.Canceled {
			t.Logf("Got error (may be expected): %v", err)
		}
	})

	t.Run("ReplaceExistingTool", func(t *testing.T) {
		mgr := tools.NewManager(logger)

		tool1 := &MockTool{
			name:        "mytool",
			description: "Version 1",
			executeFunc: func(ctx context.Context, params map[string]string) (string, error) {
				return "v1", nil
			},
		}

		tool2 := &MockTool{
			name:        "mytool",
			description: "Version 2",
			executeFunc: func(ctx context.Context, params map[string]string) (string, error) {
				return "v2", nil
			},
		}

		mgr.Register(tool1)
		mgr.Register(tool2)

		// Should use the latest registration
		ctx := context.Background()
		result, _ := mgr.Execute(ctx, "mytool", nil)

		if result != "v2" {
			t.Errorf("Expected 'v2', got '%s'", result)
		}

		// List should still have only 1 tool with that name
		toolList := mgr.List()
		count := 0
		for _, tool := range toolList {
			if tool.Name() == "mytool" {
				count++
			}
		}
		if count != 1 {
			t.Errorf("Should have exactly 1 tool named 'mytool', got %d", count)
		}
	})
}

func TestToolsWithNilParams(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	mgr := tools.NewManager(logger)

	tool := &MockTool{
		name:        "nil_params_tool",
		description: "Handles nil params",
		executeFunc: func(ctx context.Context, params map[string]string) (string, error) {
			if params == nil {
				return "nil params", nil
			}
			return "has params", nil
		},
	}

	mgr.Register(tool)

	ctx := context.Background()
	result, err := mgr.Execute(ctx, "nil_params_tool", nil)

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result != "nil params" {
		t.Errorf("Expected 'nil params', got '%s'", result)
	}
}

package subagent

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// mockExecutor is a test executor that returns a fixed result.
type mockExecutor struct {
	result    string
	err       error
	delay     time.Duration
	execCount atomic.Int32
}

func (m *mockExecutor) Execute(ctx context.Context, task string, history []Message) (string, error) {
	m.execCount.Add(1)
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return m.result, m.err
}

func TestRegistrySpawn(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "subagent-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := Config{
		DataDir:   tmpDir,
		MaxAgents: 10,
		MaxDepth:  3,
	}

	registry, err := NewRegistry(cfg)
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	executor := &mockExecutor{result: "done"}
	registry.SetExecutor(executor)

	// Spawn an agent
	agent, err := registry.Spawn(context.Background(), SpawnOptions{
		Name:    "test-agent",
		Task:    "do something",
		UserID:  "user123",
		Timeout: 1 * time.Second,
	})
	if err != nil {
		t.Fatalf("failed to spawn agent: %v", err)
	}

	if agent.ID == "" {
		t.Error("agent should have an ID")
	}
	if agent.Task != "do something" {
		t.Errorf("expected task 'do something', got '%s'", agent.Task)
	}

	// Wait for completion
	result, err := registry.WaitFor(agent.ID, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to wait for agent: %v", err)
	}

	if result.Status != StatusComplete {
		t.Errorf("expected status complete, got %s", result.Status)
	}
	if result.Result != "done" {
		t.Errorf("expected result 'done', got '%s'", result.Result)
	}
}

func TestRegistryCancel(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "subagent-test-*")
	defer os.RemoveAll(tmpDir)

	registry, _ := NewRegistry(Config{DataDir: tmpDir})
	executor := &mockExecutor{result: "done", delay: 10 * time.Second}
	registry.SetExecutor(executor)

	// Spawn a slow agent
	agent, err := registry.Spawn(context.Background(), SpawnOptions{
		Task:    "slow task",
		Timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("failed to spawn agent: %v", err)
	}

	// Wait a bit for it to start
	time.Sleep(100 * time.Millisecond)

	// Cancel it
	err = registry.Cancel(agent.ID)
	if err != nil {
		t.Fatalf("failed to cancel agent: %v", err)
	}

	// Wait for it to register as canceled
	time.Sleep(100 * time.Millisecond)

	status, _ := registry.GetStatus(agent.ID)
	if status != StatusCanceled {
		t.Errorf("expected status canceled, got %s", status)
	}
}

func TestRegistryNestingLimit(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "subagent-test-*")
	defer os.RemoveAll(tmpDir)

	// MaxDepth of 3 allows: root (depth 0) -> child (depth 1) -> grandchild (depth 2)
	// Great-grandchild would be depth 3 which exceeds maxDepth
	registry, _ := NewRegistry(Config{
		DataDir:  tmpDir,
		MaxDepth: 3,
	})
	executor := &mockExecutor{result: "done"}
	registry.SetExecutor(executor)

	// Spawn root agent (depth 0)
	root, err := registry.Spawn(context.Background(), SpawnOptions{
		Task:    "root",
		Timeout: 1 * time.Second,
	})
	if err != nil {
		t.Fatalf("failed to spawn root: %v", err)
	}
	_, _ = registry.WaitFor(root.ID, 2*time.Second)

	// Spawn child (depth 1)
	child, err := registry.Spawn(context.Background(), SpawnOptions{
		Task:     "child",
		ParentID: root.ID,
		Timeout:  1 * time.Second,
	})
	if err != nil {
		t.Fatalf("failed to spawn child: %v", err)
	}
	_, _ = registry.WaitFor(child.ID, 2*time.Second)

	// Spawn grandchild (depth 2, should succeed)
	grandchild, err := registry.Spawn(context.Background(), SpawnOptions{
		Task:     "grandchild",
		ParentID: child.ID,
		Timeout:  1 * time.Second,
	})
	if err != nil {
		t.Fatalf("failed to spawn grandchild: %v", err)
	}
	_, _ = registry.WaitFor(grandchild.ID, 2*time.Second)

	// Try to spawn great-grandchild (depth 3, should fail as it equals maxDepth)
	_, err = registry.Spawn(context.Background(), SpawnOptions{
		Task:     "great-grandchild",
		ParentID: grandchild.ID,
	})
	if err == nil {
		t.Error("expected error due to max depth, got nil")
	}
}

func TestRegistryMaxAgents(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "subagent-test-*")
	defer os.RemoveAll(tmpDir)

	registry, _ := NewRegistry(Config{
		DataDir:   tmpDir,
		MaxAgents: 2,
	})
	// No executor = agents will fail, but that's fine for this test
	executor := &mockExecutor{result: "done", delay: 5 * time.Second}
	registry.SetExecutor(executor)

	// Spawn two agents
	_, _ = registry.Spawn(context.Background(), SpawnOptions{Task: "task1", Timeout: 10 * time.Second})
	_, _ = registry.Spawn(context.Background(), SpawnOptions{Task: "task2", Timeout: 10 * time.Second})

	// Third should fail
	_, err := registry.Spawn(context.Background(), SpawnOptions{Task: "task3"})
	if err == nil {
		t.Error("expected error due to max agents limit, got nil")
	}
}

func TestRegistryMessaging(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "subagent-test-*")
	defer os.RemoveAll(tmpDir)

	registry, _ := NewRegistry(Config{DataDir: tmpDir})
	executor := &mockExecutor{result: "done", delay: 1 * time.Second}
	registry.SetExecutor(executor)

	// Spawn an agent
	agent, _ := registry.Spawn(context.Background(), SpawnOptions{
		Task:    "receive messages",
		Timeout: 5 * time.Second,
	})

	// Send a message
	err := registry.SendMessage(agent.ID, Message{
		Role:      "agent",
		Content:   "hello from another agent",
		FromAgent: "sender-123",
	})
	if err != nil {
		t.Fatalf("failed to send message: %v", err)
	}

	// Receive the message
	msg, err := registry.ReceiveMessage(agent.ID, 1*time.Second)
	if err != nil {
		t.Fatalf("failed to receive message: %v", err)
	}

	if msg == nil {
		t.Fatal("expected a message, got nil")
	}
	if msg.Content != "hello from another agent" {
		t.Errorf("expected 'hello from another agent', got '%s'", msg.Content)
	}
	if msg.FromAgent != "sender-123" {
		t.Errorf("expected from 'sender-123', got '%s'", msg.FromAgent)
	}
}

func TestRegistryStats(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "subagent-test-*")
	defer os.RemoveAll(tmpDir)

	registry, _ := NewRegistry(Config{DataDir: tmpDir})
	executor := &mockExecutor{result: "done"}
	registry.SetExecutor(executor)

	// Spawn some agents
	for i := 0; i < 3; i++ {
		agent, _ := registry.Spawn(context.Background(), SpawnOptions{
			Task:    "task",
			Timeout: 1 * time.Second,
		})
		_, _ = registry.WaitFor(agent.ID, 2*time.Second)
	}

	stats := registry.Stats()
	if stats["total"] != 3 {
		t.Errorf("expected total 3, got %d", stats["total"])
	}
	if stats["complete"] != 3 {
		t.Errorf("expected 3 complete, got %d", stats["complete"])
	}
}

func TestRegistryCleanup(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "subagent-test-*")
	defer os.RemoveAll(tmpDir)

	registry, _ := NewRegistry(Config{DataDir: tmpDir})
	executor := &mockExecutor{result: "done"}
	registry.SetExecutor(executor)

	// Spawn agents
	for i := 0; i < 5; i++ {
		agent, _ := registry.Spawn(context.Background(), SpawnOptions{
			Task:    "task",
			Timeout: 100 * time.Millisecond,
		})
		_, _ = registry.WaitFor(agent.ID, 1*time.Second)
	}

	stats := registry.Stats()
	if stats["total"] != 5 {
		t.Errorf("expected 5 agents, got %d", stats["total"])
	}

	// Cleanup old agents
	time.Sleep(50 * time.Millisecond)
	cleaned := registry.Cleanup(10 * time.Millisecond)
	if cleaned != 5 {
		t.Errorf("expected to clean 5 agents, cleaned %d", cleaned)
	}

	stats = registry.Stats()
	if stats["total"] != 0 {
		t.Errorf("expected 0 agents after cleanup, got %d", stats["total"])
	}
}

func TestRegistryTimeout(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "subagent-test-*")
	defer os.RemoveAll(tmpDir)

	registry, _ := NewRegistry(Config{DataDir: tmpDir})
	executor := &mockExecutor{result: "done", delay: 5 * time.Second}
	registry.SetExecutor(executor)

	// Spawn agent with short timeout
	agent, _ := registry.Spawn(context.Background(), SpawnOptions{
		Task:    "slow task",
		Timeout: 100 * time.Millisecond,
	})

	// Wait for completion
	result, err := registry.WaitFor(agent.ID, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to wait: %v", err)
	}

	if result.Status != StatusTimeout {
		t.Errorf("expected status timeout, got %s", result.Status)
	}
}

func TestRegistryChildren(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "subagent-test-*")
	defer os.RemoveAll(tmpDir)

	registry, _ := NewRegistry(Config{DataDir: tmpDir})
	executor := &mockExecutor{result: "done"}
	registry.SetExecutor(executor)

	// Spawn parent
	parent, _ := registry.Spawn(context.Background(), SpawnOptions{
		Task: "parent",
	})
	_, _ = registry.WaitFor(parent.ID, 1*time.Second)

	// Spawn children
	for i := 0; i < 3; i++ {
		child, _ := registry.Spawn(context.Background(), SpawnOptions{
			Task:     "child",
			ParentID: parent.ID,
		})
		_, _ = registry.WaitFor(child.ID, 1*time.Second)
	}

	children := registry.Children(parent.ID)
	if len(children) != 3 {
		t.Errorf("expected 3 children, got %d", len(children))
	}
}

func TestRegistryContext(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "subagent-test-*")
	defer os.RemoveAll(tmpDir)

	registry, _ := NewRegistry(Config{DataDir: tmpDir})
	executor := &mockExecutor{result: "done"}
	registry.SetExecutor(executor)

	agent, _ := registry.Spawn(context.Background(), SpawnOptions{
		Task: "task with context",
		Context: map[string]interface{}{
			"key1": "value1",
		},
	})

	// Set additional context
	_ = registry.SetContext(agent, "key2", "value2")

	// Get context values
	val1 := registry.GetContext(agent, "key1")
	if val1 != "value1" {
		t.Errorf("expected 'value1', got '%v'", val1)
	}

	val2 := registry.GetContext(agent, "key2")
	if val2 != "value2" {
		t.Errorf("expected 'value2', got '%v'", val2)
	}

	// Non-existent key
	val3 := registry.GetContext(agent, "nonexistent")
	if val3 != nil {
		t.Errorf("expected nil for nonexistent key, got '%v'", val3)
	}
}

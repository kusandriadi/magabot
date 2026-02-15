package plugin

import (
	"context"
	"os"
	"testing"
	"time"
)

// testPlugin is a mock plugin for testing.
type testPlugin struct {
	meta      Metadata
	initErr   error
	startErr  error
	stopErr   error
	initCount int
	startCount int
	stopCount  int
}

func (p *testPlugin) Metadata() Metadata {
	return p.meta
}

func (p *testPlugin) Init(ctx Context) error {
	p.initCount++
	return p.initErr
}

func (p *testPlugin) Start(ctx context.Context) error {
	p.startCount++
	return p.startErr
}

func (p *testPlugin) Stop(ctx context.Context) error {
	p.stopCount++
	return p.stopErr
}

func TestManagerRegister(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "plugin-test-*")
	defer os.RemoveAll(tmpDir)

	mgr := NewManager(Config{DataDir: tmpDir})

	plugin := &testPlugin{
		meta: Metadata{
			ID:      "test-plugin",
			Name:    "Test Plugin",
			Version: "1.0.0",
		},
	}

	err := mgr.Register(plugin)
	if err != nil {
		t.Fatalf("failed to register plugin: %v", err)
	}

	// Verify registration
	reg := mgr.Get("test-plugin")
	if reg == nil {
		t.Fatal("plugin should be registered")
	}
	if reg.State != StateUnloaded {
		t.Errorf("expected state unloaded, got %s", reg.State)
	}

	// Double registration should fail
	err = mgr.Register(plugin)
	if err == nil {
		t.Error("expected error for duplicate registration")
	}
}

func TestManagerLifecycle(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "plugin-test-*")
	defer os.RemoveAll(tmpDir)

	mgr := NewManager(Config{DataDir: tmpDir})

	plugin := &testPlugin{
		meta: Metadata{
			ID:       "lifecycle-plugin",
			Name:     "Lifecycle Plugin",
			Version:  "1.0.0",
			Priority: PriorityNormal,
		},
	}

	mgr.Register(plugin)

	// Init
	err := mgr.Init("lifecycle-plugin")
	if err != nil {
		t.Fatalf("failed to init plugin: %v", err)
	}
	if plugin.initCount != 1 {
		t.Errorf("expected init count 1, got %d", plugin.initCount)
	}

	reg := mgr.Get("lifecycle-plugin")
	if reg.State != StateInitialized {
		t.Errorf("expected state initialized, got %s", reg.State)
	}

	// Start
	err = mgr.Start("lifecycle-plugin")
	if err != nil {
		t.Fatalf("failed to start plugin: %v", err)
	}
	if plugin.startCount != 1 {
		t.Errorf("expected start count 1, got %d", plugin.startCount)
	}
	if reg.State != StateStarted {
		t.Errorf("expected state started, got %s", reg.State)
	}

	// Stop
	err = mgr.Stop("lifecycle-plugin")
	if err != nil {
		t.Fatalf("failed to stop plugin: %v", err)
	}
	if plugin.stopCount != 1 {
		t.Errorf("expected stop count 1, got %d", plugin.stopCount)
	}
	if reg.State != StateStopped {
		t.Errorf("expected state stopped, got %s", reg.State)
	}
}

func TestManagerStartAll(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "plugin-test-*")
	defer os.RemoveAll(tmpDir)

	mgr := NewManager(Config{DataDir: tmpDir})

	// Register plugins with different priorities
	plugins := []*testPlugin{
		{meta: Metadata{ID: "p1", Name: "Plugin 1", Priority: PriorityHigh}},
		{meta: Metadata{ID: "p2", Name: "Plugin 2", Priority: PriorityLow}},
		{meta: Metadata{ID: "p3", Name: "Plugin 3", Priority: PriorityNormal}},
	}

	for _, p := range plugins {
		mgr.Register(p)
	}

	// Start all
	err := mgr.StartAll()
	if err != nil {
		t.Fatalf("failed to start all: %v", err)
	}

	// All should be started
	for _, p := range plugins {
		reg := mgr.Get(p.meta.ID)
		if reg.State != StateStarted {
			t.Errorf("plugin %s should be started, got %s", p.meta.ID, reg.State)
		}
	}
}

func TestManagerStopAll(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "plugin-test-*")
	defer os.RemoveAll(tmpDir)

	mgr := NewManager(Config{DataDir: tmpDir})

	plugins := []*testPlugin{
		{meta: Metadata{ID: "p1", Name: "Plugin 1", Priority: PriorityHigh}},
		{meta: Metadata{ID: "p2", Name: "Plugin 2", Priority: PriorityLow}},
	}

	for _, p := range plugins {
		mgr.Register(p)
	}

	mgr.StartAll()
	mgr.StopAll()

	// All should be stopped
	for _, p := range plugins {
		reg := mgr.Get(p.meta.ID)
		if reg.State != StateStopped {
			t.Errorf("plugin %s should be stopped, got %s", p.meta.ID, reg.State)
		}
	}
}

func TestManagerDependencies(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "plugin-test-*")
	defer os.RemoveAll(tmpDir)

	mgr := NewManager(Config{DataDir: tmpDir})

	// Plugin with dependency
	base := &testPlugin{
		meta: Metadata{ID: "base", Name: "Base Plugin", Priority: PriorityCore},
	}
	dependent := &testPlugin{
		meta: Metadata{
			ID:           "dependent",
			Name:         "Dependent Plugin",
			Priority:     PriorityNormal,
			Dependencies: []string{"base"},
		},
	}

	mgr.Register(base)
	mgr.Register(dependent)

	// Try to init dependent before base
	err := mgr.Init("dependent")
	if err == nil {
		t.Error("expected error for uninitialized dependency")
	}

	// Init base first
	mgr.Init("base")
	
	// Now dependent should work
	err = mgr.Init("dependent")
	if err != nil {
		t.Fatalf("failed to init dependent: %v", err)
	}
}

func TestManagerCommands(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "plugin-test-*")
	defer os.RemoveAll(tmpDir)

	mgr := NewManager(Config{DataDir: tmpDir})

	plugin := &testPlugin{
		meta: Metadata{ID: "cmd-plugin", Name: "Command Plugin"},
	}

	mgr.Register(plugin)

	// Init to get context
	mgr.Init("cmd-plugin")

	// Get registration to access context
	reg := mgr.Get("cmd-plugin")
	ctx := &pluginContext{
		manager:  mgr,
		pluginID: "cmd-plugin",
		config:   reg.Config,
		dataDir:  reg.DataDir,
		logger:   mgr.logger,
	}

	// Register command
	err := ctx.RegisterCommand("test", func(ctx context.Context, cmd *Command) (string, error) {
		return "test response", nil
	})
	if err != nil {
		t.Fatalf("failed to register command: %v", err)
	}

	// Check command exists
	if !mgr.HasCommand("test") {
		t.Error("command should be registered")
	}

	// Execute command
	resp, err := mgr.HandleCommand(context.Background(), &Command{Name: "test"})
	if err != nil {
		t.Fatalf("failed to handle command: %v", err)
	}
	if resp != "test response" {
		t.Errorf("expected 'test response', got '%s'", resp)
	}
}

func TestManagerHooks(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "plugin-test-*")
	defer os.RemoveAll(tmpDir)

	mgr := NewManager(Config{DataDir: tmpDir})

	plugin := &testPlugin{
		meta: Metadata{ID: "hook-plugin", Name: "Hook Plugin"},
	}

	mgr.Register(plugin)
	mgr.Init("hook-plugin")

	reg := mgr.Get("hook-plugin")
	ctx := &pluginContext{
		manager:  mgr,
		pluginID: "hook-plugin",
		config:   reg.Config,
		dataDir:  reg.DataDir,
		logger:   mgr.logger,
	}

	hookCalled := false
	err := ctx.RegisterHook("pre_message", func(ctx context.Context, event string, data interface{}) error {
		hookCalled = true
		return nil
	})
	if err != nil {
		t.Fatalf("failed to register hook: %v", err)
	}

	// Trigger hook
	mgr.TriggerHook(context.Background(), "pre_message", nil)

	if !hookCalled {
		t.Error("hook should have been called")
	}
}

func TestManagerStats(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "plugin-test-*")
	defer os.RemoveAll(tmpDir)

	mgr := NewManager(Config{DataDir: tmpDir})

	// Register some plugins
	for i := 0; i < 3; i++ {
		plugin := &testPlugin{
			meta: Metadata{ID: string(rune('a' + i)), Name: "Plugin"},
		}
		mgr.Register(plugin)
	}

	// Init one
	mgr.Init("a")

	// Start one
	mgr.Start("a")

	stats := mgr.Stats()
	if stats["total"] != 3 {
		t.Errorf("expected total 3, got %d", stats["total"])
	}
	if stats["started"] != 1 {
		t.Errorf("expected started 1, got %d", stats["started"])
	}
	if stats["unloaded"] != 2 {
		t.Errorf("expected unloaded 2, got %d", stats["unloaded"])
	}
}

func TestManagerEvents(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "plugin-test-*")
	defer os.RemoveAll(tmpDir)

	mgr := NewManager(Config{DataDir: tmpDir})

	eventReceived := make(chan string, 1)

	// Register listener directly (normally done through plugin context)
	mgr.mu.Lock()
	mgr.eventListeners["test_event"] = []eventListener{
		{
			pluginID: "test",
			handler: func(event string, data interface{}) {
				eventReceived <- event
			},
		},
	}
	mgr.mu.Unlock()

	// Emit event
	mgr.Emit("test_event", nil)

	// Wait for event
	select {
	case event := <-eventReceived:
		if event != "test_event" {
			t.Errorf("expected 'test_event', got '%s'", event)
		}
	case <-time.After(1 * time.Second):
		t.Error("timeout waiting for event")
	}
}

func TestManagerConfig(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "plugin-test-*")
	defer os.RemoveAll(tmpDir)

	mgr := NewManager(Config{DataDir: tmpDir})

	plugin := &testPlugin{
		meta: Metadata{
			ID:   "config-plugin",
			Name: "Config Plugin",
			ConfigSchema: &ConfigSchema{
				Fields: []ConfigField{
					{Name: "setting1", Type: "string", Default: "default-value"},
					{Name: "setting2", Type: "int", Default: 42},
				},
			},
		},
	}

	mgr.Register(plugin)

	// Check defaults
	reg := mgr.Get("config-plugin")
	if reg.Config["setting1"] != "default-value" {
		t.Errorf("expected default value, got %v", reg.Config["setting1"])
	}

	// Save and load config
	reg.Config["setting1"] = "new-value"
	err := mgr.SaveConfig()
	if err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Modify config
	reg.Config["setting1"] = "temporary"

	// Load should restore
	err = mgr.LoadConfig()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if reg.Config["setting1"] != "new-value" {
		t.Errorf("expected loaded value 'new-value', got %v", reg.Config["setting1"])
	}
}

func TestManagerList(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "plugin-test-*")
	defer os.RemoveAll(tmpDir)

	mgr := NewManager(Config{DataDir: tmpDir})

	// Register plugins
	for i := 0; i < 5; i++ {
		plugin := &testPlugin{
			meta: Metadata{ID: string(rune('a' + i)), Name: "Plugin"},
		}
		mgr.Register(plugin)
	}

	list := mgr.List()
	if len(list) != 5 {
		t.Errorf("expected 5 plugins, got %d", len(list))
	}
}

func TestPluginContextDataDir(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "plugin-test-*")
	defer os.RemoveAll(tmpDir)

	mgr := NewManager(Config{DataDir: tmpDir})

	plugin := &testPlugin{
		meta: Metadata{ID: "data-plugin", Name: "Data Plugin"},
	}

	mgr.Register(plugin)

	reg := mgr.Get("data-plugin")
	expectedDir := tmpDir + "/plugins/data-plugin"

	if reg.DataDir != expectedDir {
		t.Errorf("expected data dir '%s', got '%s'", expectedDir, reg.DataDir)
	}

	// Check directory was created
	if _, err := os.Stat(reg.DataDir); os.IsNotExist(err) {
		t.Error("plugin data directory should exist")
	}
}

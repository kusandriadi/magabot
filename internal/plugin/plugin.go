// Package plugin provides a plugin system for extending Magabot functionality.
// Plugins are Go packages that implement the Plugin interface and can be
// discovered, loaded, and managed at runtime.
package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// State represents the lifecycle state of a plugin.
type State string

const (
	StateUnloaded    State = "unloaded"
	StateLoading     State = "loading"
	StateInitialized State = "initialized"
	StateStarted     State = "started"
	StateStopping    State = "stopping"
	StateStopped     State = "stopped"
	StateError       State = "error"
)

// Priority defines plugin initialization order.
type Priority int

const (
	PriorityCore   Priority = 0 // Core plugins (loaded first)
	PriorityHigh   Priority = 100
	PriorityNormal Priority = 500
	PriorityLow    Priority = 900
	PriorityLast   Priority = 999 // Loaded last
)

// Metadata holds plugin metadata.
type Metadata struct {
	ID           string        `json:"id" yaml:"id"`           // Unique identifier
	Name         string        `json:"name" yaml:"name"`       // Display name
	Version      string        `json:"version" yaml:"version"` // SemVer version
	Description  string        `json:"description" yaml:"description"`
	Author       string        `json:"author" yaml:"author"`
	Homepage     string        `json:"homepage,omitempty" yaml:"homepage,omitempty"`
	License      string        `json:"license,omitempty" yaml:"license,omitempty"`
	Priority     Priority      `json:"priority" yaml:"priority"`
	Dependencies []string      `json:"dependencies,omitempty" yaml:"dependencies,omitempty"` // Other plugin IDs
	Tags         []string      `json:"tags,omitempty" yaml:"tags,omitempty"`
	Capabilities []string      `json:"capabilities,omitempty" yaml:"capabilities,omitempty"` // What the plugin provides
	ConfigSchema *ConfigSchema `json:"config_schema,omitempty" yaml:"config_schema,omitempty"`
}

// ConfigSchema defines the configuration structure for a plugin.
type ConfigSchema struct {
	Fields []ConfigField `json:"fields" yaml:"fields"`
}

// ConfigField defines a single configuration field.
type ConfigField struct {
	Name        string      `json:"name" yaml:"name"`
	Type        string      `json:"type" yaml:"type"` // string, int, bool, float, []string, map
	Required    bool        `json:"required" yaml:"required"`
	Default     interface{} `json:"default,omitempty" yaml:"default,omitempty"`
	Description string      `json:"description,omitempty" yaml:"description,omitempty"`
	Validate    string      `json:"validate,omitempty" yaml:"validate,omitempty"` // Validation pattern/rule
}

// Plugin is the interface that all plugins must implement.
type Plugin interface {
	// Metadata returns the plugin's metadata.
	Metadata() Metadata

	// Init is called when the plugin is loaded. Use for one-time setup.
	// The context provides access to the plugin host services.
	Init(ctx Context) error

	// Start is called when the plugin should begin operation.
	Start(ctx context.Context) error

	// Stop is called when the plugin should cease operation.
	Stop(ctx context.Context) error
}

// Context provides plugins access to host services.
type Context interface {
	// Logger returns a logger for the plugin.
	Logger() *slog.Logger

	// Config returns the plugin's configuration.
	Config() map[string]interface{}

	// SetConfig updates the plugin's configuration.
	SetConfig(key string, value interface{}) error

	// DataDir returns the plugin's data directory.
	DataDir() string

	// SendMessage sends a message to a platform.
	SendMessage(platform, chatID, message string) error

	// RegisterCommand registers a chat command handler.
	RegisterCommand(cmd string, handler CommandHandler) error

	// RegisterHook registers an event hook.
	RegisterHook(event string, handler HookHandler) error

	// GetPlugin returns another loaded plugin by ID.
	GetPlugin(id string) (Plugin, error)

	// Emit emits an event that other plugins can listen to.
	Emit(event string, data interface{})
}

// CommandHandler handles a chat command.
type CommandHandler func(ctx context.Context, cmd *Command) (string, error)

// Command represents a chat command invocation.
type Command struct {
	Name     string
	Args     []string
	RawArgs  string
	Platform string
	ChatID   string
	UserID   string
	IsAdmin  bool
	Message  string
}

// HookHandler handles an event hook.
type HookHandler func(ctx context.Context, event string, data interface{}) error

// Registration holds a loaded plugin and its state.
type Registration struct {
	mu sync.RWMutex

	Plugin    Plugin
	Metadata  Metadata
	State     State
	Error     error
	Config    map[string]interface{}
	DataDir   string
	LoadedAt  time.Time
	StartedAt *time.Time
	StoppedAt *time.Time
}

// Manager handles plugin discovery, loading, and lifecycle.
type Manager struct {
	mu sync.RWMutex

	plugins        map[string]*Registration
	pluginDirs     []string
	dataDir        string
	commands       map[string]commandEntry
	hooks          map[string][]hookEntry
	eventListeners map[string][]eventListener
	messageSender  MessageSender
	logger         *slog.Logger
}

type commandEntry struct {
	pluginID string
	handler  CommandHandler
}

type hookEntry struct {
	pluginID string
	handler  HookHandler
}

type eventListener struct {
	pluginID string
	handler  func(event string, data interface{})
}

// MessageSender is the interface for sending messages to platforms.
type MessageSender interface {
	Send(platform, chatID, message string) error
}

// Config holds manager configuration.
type Config struct {
	PluginDirs    []string
	DataDir       string
	MessageSender MessageSender
	Logger        *slog.Logger
}

// NewManager creates a new plugin manager.
func NewManager(cfg Config) *Manager {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return &Manager{
		plugins:        make(map[string]*Registration),
		pluginDirs:     cfg.PluginDirs,
		dataDir:        cfg.DataDir,
		commands:       make(map[string]commandEntry),
		hooks:          make(map[string][]hookEntry),
		eventListeners: make(map[string][]eventListener),
		messageSender:  cfg.MessageSender,
		logger:         cfg.Logger,
	}
}

// validatePluginID checks that a plugin ID is safe for filesystem operations.
func validatePluginID(id string) error {
	if id == "" {
		return fmt.Errorf("plugin ID cannot be empty")
	}

	// Block path traversal characters
	if strings.Contains(id, "/") || strings.Contains(id, "\\") ||
		strings.Contains(id, "..") || strings.Contains(id, "\x00") {
		return fmt.Errorf("plugin ID contains invalid characters: %s", id)
	}

	// Ensure ID doesn't start with dots (hidden files/directories)
	if strings.HasPrefix(id, ".") {
		return fmt.Errorf("plugin ID cannot start with dot: %s", id)
	}

	// Check for reserved names on Windows
	reserved := []string{"CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4",
		"COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4",
		"LPT5", "LPT6", "LPT7", "LPT8", "LPT9"}
	upperID := strings.ToUpper(id)
	for _, r := range reserved {
		if upperID == r {
			return fmt.Errorf("plugin ID is reserved name: %s", id)
		}
	}

	// Limit length
	if len(id) > 128 {
		return fmt.Errorf("plugin ID too long (max 128 characters): %s", id)
	}

	return nil
}

// Register manually registers a plugin (for built-in plugins).
func (m *Manager) Register(p Plugin) error {
	meta := p.Metadata()

	// Validate plugin ID for path traversal prevention
	if err := validatePluginID(meta.ID); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.plugins[meta.ID]; exists {
		return fmt.Errorf("plugin already registered: %s", meta.ID)
	}

	// Create data directory for plugin (safe after validation)
	pluginDataDir := filepath.Join(m.dataDir, "plugins", meta.ID)
	if err := os.MkdirAll(pluginDataDir, 0700); err != nil {
		return fmt.Errorf("create plugin data dir: %w", err)
	}

	reg := &Registration{
		Plugin:   p,
		Metadata: meta,
		State:    StateUnloaded,
		Config:   make(map[string]interface{}),
		DataDir:  pluginDataDir,
		LoadedAt: time.Now(),
	}

	// Load config defaults
	if meta.ConfigSchema != nil {
		for _, field := range meta.ConfigSchema.Fields {
			if field.Default != nil {
				reg.Config[field.Name] = field.Default
			}
		}
	}

	m.plugins[meta.ID] = reg
	m.logger.Info("registered plugin", "id", meta.ID, "name", meta.Name)

	return nil
}

// Init initializes a registered plugin.
func (m *Manager) Init(id string) error {
	m.mu.Lock()
	reg, ok := m.plugins[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("plugin not found: %s", id)
	}
	m.mu.Unlock()

	reg.mu.Lock()
	defer reg.mu.Unlock()

	if reg.State != StateUnloaded && reg.State != StateError {
		return fmt.Errorf("plugin %s is in state %s, cannot init", id, reg.State)
	}

	reg.State = StateLoading

	// Create plugin context
	ctx := &pluginContext{
		manager:  m,
		pluginID: id,
		config:   reg.Config,
		dataDir:  reg.DataDir,
		logger:   m.logger.With("plugin", id),
	}

	// Check dependencies
	for _, depID := range reg.Metadata.Dependencies {
		dep, exists := m.plugins[depID]
		if !exists {
			reg.State = StateError
			reg.Error = fmt.Errorf("missing dependency: %s", depID)
			return reg.Error
		}
		if dep.State != StateInitialized && dep.State != StateStarted {
			reg.State = StateError
			reg.Error = fmt.Errorf("dependency not initialized: %s", depID)
			return reg.Error
		}
	}

	// Initialize
	if err := reg.Plugin.Init(ctx); err != nil {
		reg.State = StateError
		reg.Error = err
		m.logger.Error("plugin init failed", "id", id, "error", err)
		return err
	}

	reg.State = StateInitialized
	m.logger.Info("plugin initialized", "id", id)

	return nil
}

// Start starts an initialized plugin.
func (m *Manager) Start(id string) error {
	m.mu.RLock()
	reg, ok := m.plugins[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("plugin not found: %s", id)
	}

	reg.mu.Lock()
	defer reg.mu.Unlock()

	if reg.State != StateInitialized {
		return fmt.Errorf("plugin %s is in state %s, cannot start", id, reg.State)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := reg.Plugin.Start(ctx); err != nil {
		reg.State = StateError
		reg.Error = err
		m.logger.Error("plugin start failed", "id", id, "error", err)
		return err
	}

	now := time.Now()
	reg.State = StateStarted
	reg.StartedAt = &now
	m.logger.Info("plugin started", "id", id)

	return nil
}

// Stop stops a running plugin.
func (m *Manager) Stop(id string) error {
	m.mu.RLock()
	reg, ok := m.plugins[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("plugin not found: %s", id)
	}

	reg.mu.Lock()
	defer reg.mu.Unlock()

	if reg.State != StateStarted {
		return fmt.Errorf("plugin %s is in state %s, cannot stop", id, reg.State)
	}

	reg.State = StateStopping

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := reg.Plugin.Stop(ctx); err != nil {
		m.logger.Warn("plugin stop error", "id", id, "error", err)
		// Continue stopping anyway
	}

	now := time.Now()
	reg.State = StateStopped
	reg.StoppedAt = &now
	m.logger.Info("plugin stopped", "id", id)

	// Unregister commands and hooks
	m.unregisterPlugin(id)

	return nil
}

// StartAll starts all registered plugins in priority order.
func (m *Manager) StartAll() error {
	m.mu.RLock()
	plugins := make([]*Registration, 0, len(m.plugins))
	for _, reg := range m.plugins {
		plugins = append(plugins, reg)
	}
	m.mu.RUnlock()

	// Sort by priority (lower = first)
	for i := 0; i < len(plugins)-1; i++ {
		for j := i + 1; j < len(plugins); j++ {
			if plugins[j].Metadata.Priority < plugins[i].Metadata.Priority {
				plugins[i], plugins[j] = plugins[j], plugins[i]
			}
		}
	}

	// Initialize all
	for _, reg := range plugins {
		if reg.State == StateUnloaded || reg.State == StateError {
			if err := m.Init(reg.Metadata.ID); err != nil {
				m.logger.Warn("failed to init plugin", "id", reg.Metadata.ID, "error", err)
			}
		}
	}

	// Start all initialized
	for _, reg := range plugins {
		if reg.State == StateInitialized {
			if err := m.Start(reg.Metadata.ID); err != nil {
				m.logger.Warn("failed to start plugin", "id", reg.Metadata.ID, "error", err)
			}
		}
	}

	return nil
}

// StopAll stops all running plugins in reverse priority order.
func (m *Manager) StopAll() error {
	m.mu.RLock()
	plugins := make([]*Registration, 0, len(m.plugins))
	for _, reg := range m.plugins {
		if reg.State == StateStarted {
			plugins = append(plugins, reg)
		}
	}
	m.mu.RUnlock()

	// Sort by priority (higher = first to stop, reverse order)
	for i := 0; i < len(plugins)-1; i++ {
		for j := i + 1; j < len(plugins); j++ {
			if plugins[j].Metadata.Priority > plugins[i].Metadata.Priority {
				plugins[i], plugins[j] = plugins[j], plugins[i]
			}
		}
	}

	for _, reg := range plugins {
		if err := m.Stop(reg.Metadata.ID); err != nil {
			m.logger.Warn("failed to stop plugin", "id", reg.Metadata.ID, "error", err)
		}
	}

	return nil
}

// Get returns a plugin registration by ID.
func (m *Manager) Get(id string) *Registration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.plugins[id]
}

// List returns all registered plugins.
func (m *Manager) List() []*Registration {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Registration, 0, len(m.plugins))
	for _, reg := range m.plugins {
		result = append(result, reg)
	}
	return result
}

// Stats returns plugin statistics.
func (m *Manager) Stats() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := map[string]int{
		"total":       len(m.plugins),
		"unloaded":    0,
		"initialized": 0,
		"started":     0,
		"stopped":     0,
		"error":       0,
	}

	for _, reg := range m.plugins {
		switch reg.State {
		case StateUnloaded:
			stats["unloaded"]++
		case StateInitialized:
			stats["initialized"]++
		case StateStarted:
			stats["started"]++
		case StateStopped:
			stats["stopped"]++
		case StateError:
			stats["error"]++
		}
	}

	return stats
}

// HandleCommand processes a command through registered handlers.
func (m *Manager) HandleCommand(ctx context.Context, cmd *Command) (string, error) {
	m.mu.RLock()
	entry, ok := m.commands[cmd.Name]
	m.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("unknown command: %s", cmd.Name)
	}

	return entry.handler(ctx, cmd)
}

// HasCommand checks if a command is registered.
func (m *Manager) HasCommand(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.commands[name]
	return ok
}

// TriggerHook triggers an event hook.
func (m *Manager) TriggerHook(ctx context.Context, event string, data interface{}) error {
	m.mu.RLock()
	entries := m.hooks[event]
	m.mu.RUnlock()

	var lastErr error
	for _, entry := range entries {
		if err := entry.handler(ctx, event, data); err != nil {
			m.logger.Warn("hook error", "event", event, "plugin", entry.pluginID, "error", err)
			lastErr = err
		}
	}

	return lastErr
}

// Emit broadcasts an event to all listeners.
func (m *Manager) Emit(event string, data interface{}) {
	m.mu.RLock()
	listeners := m.eventListeners[event]
	m.mu.RUnlock()

	for _, listener := range listeners {
		go listener.handler(event, data)
	}
}

// unregisterPlugin removes all commands and hooks for a plugin.
func (m *Manager) unregisterPlugin(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Remove commands
	for cmd, entry := range m.commands {
		if entry.pluginID == id {
			delete(m.commands, cmd)
		}
	}

	// Remove hooks
	for event, entries := range m.hooks {
		filtered := entries[:0]
		for _, entry := range entries {
			if entry.pluginID != id {
				filtered = append(filtered, entry)
			}
		}
		m.hooks[event] = filtered
	}

	// Remove event listeners
	for event, listeners := range m.eventListeners {
		filtered := listeners[:0]
		for _, listener := range listeners {
			if listener.pluginID != id {
				filtered = append(filtered, listener)
			}
		}
		m.eventListeners[event] = filtered
	}
}

// SaveConfig persists plugin configs to disk.
func (m *Manager) SaveConfig() error {
	m.mu.RLock()
	configs := make(map[string]map[string]interface{})
	for id, reg := range m.plugins {
		reg.mu.RLock()
		configs[id] = reg.Config
		reg.mu.RUnlock()
	}
	m.mu.RUnlock()

	configPath := filepath.Join(m.dataDir, "plugin_configs.json")
	data, err := json.MarshalIndent(configs, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal plugin configs: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	tmpFile := configPath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return fmt.Errorf("write plugin configs: %w", err)
	}

	if err := os.Rename(tmpFile, configPath); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("rename plugin configs: %w", err)
	}

	return nil
}

// LoadConfig loads plugin configs from disk.
func (m *Manager) LoadConfig() error {
	configPath := filepath.Join(m.dataDir, "plugin_configs.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read plugin configs: %w", err)
	}

	var configs map[string]map[string]interface{}
	if err := json.Unmarshal(data, &configs); err != nil {
		return fmt.Errorf("parse plugin configs: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for id, config := range configs {
		if reg, ok := m.plugins[id]; ok {
			reg.mu.Lock()
			for k, v := range config {
				reg.Config[k] = v
			}
			reg.mu.Unlock()
		}
	}

	return nil
}

// pluginContext implements Context for a specific plugin.
type pluginContext struct {
	manager  *Manager
	pluginID string
	config   map[string]interface{}
	dataDir  string
	logger   *slog.Logger
}

func (c *pluginContext) Logger() *slog.Logger {
	return c.logger
}

func (c *pluginContext) Config() map[string]interface{} {
	return c.config
}

func (c *pluginContext) SetConfig(key string, value interface{}) error {
	c.config[key] = value
	return c.manager.SaveConfig()
}

func (c *pluginContext) DataDir() string {
	return c.dataDir
}

func (c *pluginContext) SendMessage(platform, chatID, message string) error {
	if c.manager.messageSender == nil {
		return fmt.Errorf("message sender not configured")
	}
	return c.manager.messageSender.Send(platform, chatID, message)
}

func (c *pluginContext) RegisterCommand(cmd string, handler CommandHandler) error {
	// Validate command name
	if cmd == "" {
		return fmt.Errorf("command name cannot be empty")
	}
	if len(cmd) > 64 {
		return fmt.Errorf("command name too long (max 64 characters)")
	}
	// Only allow alphanumeric, underscores, and hyphens
	for _, r := range cmd {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-') {
			return fmt.Errorf("command name contains invalid character: %c", r)
		}
	}

	c.manager.mu.Lock()
	defer c.manager.mu.Unlock()

	if _, exists := c.manager.commands[cmd]; exists {
		return fmt.Errorf("command already registered: %s", cmd)
	}

	c.manager.commands[cmd] = commandEntry{
		pluginID: c.pluginID,
		handler:  handler,
	}

	c.logger.Debug("registered command", "cmd", cmd)
	return nil
}

func (c *pluginContext) RegisterHook(event string, handler HookHandler) error {
	c.manager.mu.Lock()
	defer c.manager.mu.Unlock()

	c.manager.hooks[event] = append(c.manager.hooks[event], hookEntry{
		pluginID: c.pluginID,
		handler:  handler,
	})

	c.logger.Debug("registered hook", "event", event)
	return nil
}

func (c *pluginContext) GetPlugin(id string) (Plugin, error) {
	reg := c.manager.Get(id)
	if reg == nil {
		return nil, fmt.Errorf("plugin not found: %s", id)
	}
	if reg.State != StateStarted && reg.State != StateInitialized {
		return nil, fmt.Errorf("plugin not available: %s (state: %s)", id, reg.State)
	}
	return reg.Plugin, nil
}

func (c *pluginContext) Emit(event string, data interface{}) {
	c.manager.Emit(event, data)
}

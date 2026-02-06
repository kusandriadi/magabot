// Package tools provides external tool integrations for LLM
package tools

import (
	"context"
	"log/slog"
)

// Tool interface for all tools
type Tool interface {
	Name() string
	Description() string
	Execute(ctx context.Context, params map[string]string) (string, error)
}

// Manager manages all available tools
type Manager struct {
	tools  map[string]Tool
	logger *slog.Logger
}

// NewManager creates a new tool manager
func NewManager(logger *slog.Logger) *Manager {
	return &Manager{
		tools:  make(map[string]Tool),
		logger: logger,
	}
}

// Register registers a tool
func (m *Manager) Register(t Tool) {
	m.tools[t.Name()] = t
	m.logger.Info("registered tool", "name", t.Name())
}

// Get returns a tool by name
func (m *Manager) Get(name string) (Tool, bool) {
	t, ok := m.tools[name]
	return t, ok
}

// List returns all registered tools
func (m *Manager) List() []Tool {
	tools := make([]Tool, 0, len(m.tools))
	for _, t := range m.tools {
		tools = append(tools, t)
	}
	return tools
}

// Execute executes a tool by name
func (m *Manager) Execute(ctx context.Context, name string, params map[string]string) (string, error) {
	tool, ok := m.tools[name]
	if !ok {
		return "", nil
	}
	return tool.Execute(ctx, params)
}

// GetToolDescriptions returns descriptions for LLM context
func (m *Manager) GetToolDescriptions() string {
	desc := "Available tools:\n"
	for _, t := range m.tools {
		desc += "- " + t.Name() + ": " + t.Description() + "\n"
	}
	return desc
}

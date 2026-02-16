// Package subagent provides multi-agent orchestration with isolated sessions.
// Each sub-agent runs in its own goroutine with independent context, allowing
// parallel execution and agent-to-agent communication.
package subagent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// Status represents the lifecycle state of a sub-agent.
type Status string

const (
	StatusPending  Status = "pending"
	StatusRunning  Status = "running"
	StatusComplete Status = "complete"
	StatusFailed   Status = "failed"
	StatusCanceled Status = "canceled"
	StatusTimeout  Status = "timeout"
)

// Agent represents a spawned sub-agent with its own isolated session.
type Agent struct {
	mu sync.RWMutex

	ID          string                 `json:"id"`
	ParentID    string                 `json:"parent_id,omitempty"` // Parent agent ID (empty for root)
	Name        string                 `json:"name,omitempty"`      // Human-readable name
	Task        string                 `json:"task"`                // Task description/prompt
	Status      Status                 `json:"status"`
	Result      string                 `json:"result,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Context     map[string]interface{} `json:"context,omitempty"`  // Shared context data
	Messages    []Message              `json:"messages,omitempty"` // Agent's conversation history
	Metadata    map[string]string      `json:"metadata,omitempty"` // Custom metadata
	Platform    string                 `json:"platform,omitempty"` // Originating platform
	ChatID      string                 `json:"chat_id,omitempty"`  // Originating chat
	UserID      string                 `json:"user_id,omitempty"`  // Originating user
	Priority    int                    `json:"priority"`           // 0=normal, higher=urgent
	Timeout     time.Duration          `json:"timeout"`            // Max execution time
	CreatedAt   time.Time              `json:"created_at"`
	StartedAt   *time.Time             `json:"started_at,omitempty"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`

	// Internal fields (not serialized)
	cancelFunc context.CancelFunc `json:"-"`
	inbox      chan Message       `json:"-"` // For receiving messages from other agents
}

// Message represents a message in an agent's conversation or mailbox.
type Message struct {
	ID        string                 `json:"id"`
	Role      string                 `json:"role"` // user, assistant, system, agent
	Content   string                 `json:"content"`
	FromAgent string                 `json:"from_agent,omitempty"` // Sender agent ID
	ToAgent   string                 `json:"to_agent,omitempty"`   // Target agent ID
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// ExecutorFunc is the function signature for agent task execution.
// It receives the agent context and should return the result or error.
type ExecutorFunc func(ctx context.Context, agent *Agent, executor TaskExecutor) (string, error)

// TaskExecutor provides LLM execution capabilities to sub-agents.
type TaskExecutor interface {
	Execute(ctx context.Context, task string, history []Message) (string, error)
}

// NotifyFunc is called when an agent's status changes.
type NotifyFunc func(agent *Agent)

// Registry manages all sub-agents and their lifecycles.
type Registry struct {
	mu sync.RWMutex

	agents      map[string]*Agent   // All agents by ID
	byParent    map[string][]string // Parent ID -> child agent IDs
	executor    TaskExecutor
	notifyFunc  NotifyFunc
	persistPath string
	counter     atomic.Int64
	logger      *slog.Logger
	maxAgents   int // Max concurrent agents
	maxDepth    int // Max nesting depth
	maxHistory  int // Max messages per agent
}

// Config holds registry configuration.
type Config struct {
	DataDir    string // Directory for persistence
	MaxAgents  int    // Max concurrent agents (default: 50)
	MaxDepth   int    // Max nesting depth (default: 5)
	MaxHistory int    // Max messages per agent (default: 100)
	Logger     *slog.Logger
}

// NewRegistry creates a new sub-agent registry.
func NewRegistry(cfg Config) (*Registry, error) {
	if cfg.MaxAgents <= 0 {
		cfg.MaxAgents = 50
	}
	if cfg.MaxDepth <= 0 {
		cfg.MaxDepth = 5
	}
	if cfg.MaxHistory <= 0 {
		cfg.MaxHistory = 100
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	persistPath := ""
	if cfg.DataDir != "" {
		persistPath = filepath.Join(cfg.DataDir, "subagents.json")
	}

	r := &Registry{
		agents:      make(map[string]*Agent),
		byParent:    make(map[string][]string),
		persistPath: persistPath,
		maxAgents:   cfg.MaxAgents,
		maxDepth:    cfg.MaxDepth,
		maxHistory:  cfg.MaxHistory,
		logger:      cfg.Logger,
	}

	// Load persisted state if available
	if persistPath != "" {
		if err := r.load(); err != nil && !os.IsNotExist(err) {
			r.logger.Warn("failed to load subagent state", "error", err)
		}
	}

	return r, nil
}

// SetExecutor sets the task executor for running agent tasks.
func (r *Registry) SetExecutor(executor TaskExecutor) {
	r.executor = executor
}

// SetNotifyFunc sets the notification callback.
func (r *Registry) SetNotifyFunc(fn NotifyFunc) {
	r.notifyFunc = fn
}

// SpawnOptions configures a new sub-agent.
type SpawnOptions struct {
	Name     string
	Task     string
	ParentID string
	Platform string
	ChatID   string
	UserID   string
	Context  map[string]interface{}
	Metadata map[string]string
	Priority int
	Timeout  time.Duration
}

// MaxTaskLength is the maximum allowed task description length.
const MaxTaskLength = 100000 // 100KB

// MaxNameLength is the maximum allowed agent name length.
const MaxNameLength = 256

// Spawn creates and starts a new sub-agent.
func (r *Registry) Spawn(ctx context.Context, opts SpawnOptions) (*Agent, error) {
	// Validate inputs before acquiring lock
	if len(opts.Task) > MaxTaskLength {
		return nil, fmt.Errorf("task too long (max %d characters)", MaxTaskLength)
	}
	if len(opts.Name) > MaxNameLength {
		return nil, fmt.Errorf("name too long (max %d characters)", MaxNameLength)
	}
	if strings.TrimSpace(opts.Task) == "" {
		return nil, fmt.Errorf("task cannot be empty")
	}

	r.mu.Lock()

	// Check limits
	if len(r.agents) >= r.maxAgents {
		r.mu.Unlock()
		return nil, fmt.Errorf("max agents limit reached (%d)", r.maxAgents)
	}

	// Check nesting depth
	depth := r.getDepth(opts.ParentID)
	if depth >= r.maxDepth {
		r.mu.Unlock()
		return nil, fmt.Errorf("max nesting depth reached (%d)", r.maxDepth)
	}

	// Generate unique ID
	id := fmt.Sprintf("agent-%s-%d", uuid.New().String()[:8], r.counter.Add(1))

	// Default timeout
	if opts.Timeout <= 0 {
		opts.Timeout = 5 * time.Minute
	}

	agent := &Agent{
		ID:        id,
		ParentID:  opts.ParentID,
		Name:      opts.Name,
		Task:      opts.Task,
		Status:    StatusPending,
		Context:   opts.Context,
		Metadata:  opts.Metadata,
		Platform:  opts.Platform,
		ChatID:    opts.ChatID,
		UserID:    opts.UserID,
		Priority:  opts.Priority,
		Timeout:   opts.Timeout,
		Messages:  make([]Message, 0),
		CreatedAt: time.Now(),
		inbox:     make(chan Message, 100),
	}

	// Initialize context if nil
	if agent.Context == nil {
		agent.Context = make(map[string]interface{})
	}
	if agent.Metadata == nil {
		agent.Metadata = make(map[string]string)
	}

	// Register
	r.agents[id] = agent
	if opts.ParentID != "" {
		r.byParent[opts.ParentID] = append(r.byParent[opts.ParentID], id)
	}

	r.mu.Unlock()

	// Start execution in background
	go r.runAgent(ctx, agent)

	r.logger.Info("spawned sub-agent",
		"id", id,
		"parent", opts.ParentID,
		"task", truncate(opts.Task, 100),
	)

	return agent, nil
}

// runAgent executes the agent's task.
func (r *Registry) runAgent(parentCtx context.Context, agent *Agent) {
	// Create timeout context
	ctx, cancel := context.WithTimeout(parentCtx, agent.Timeout)
	defer cancel()

	agent.mu.Lock()
	agent.Status = StatusRunning
	now := time.Now()
	agent.StartedAt = &now
	agent.cancelFunc = cancel
	agent.mu.Unlock()

	r.notify(agent)

	var result string
	var err error

	if r.executor != nil {
		// Get parent context for continuity
		var history []Message
		if agent.ParentID != "" {
			if parent := r.Get(agent.ParentID); parent != nil {
				history = r.GetHistory(parent, 10)
			}
		}

		result, err = r.executor.Execute(ctx, agent.Task, history)
	} else {
		err = fmt.Errorf("no task executor configured")
	}

	// Check if context was canceled
	if ctx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("task timed out after %v", agent.Timeout)
	} else if ctx.Err() == context.Canceled {
		agent.mu.Lock()
		agent.Status = StatusCanceled
		now := time.Now()
		agent.CompletedAt = &now
		agent.mu.Unlock()
		r.notify(agent)
		r.persist()
		return
	}

	// Update final state
	now = time.Now()
	agent.mu.Lock()
	agent.CompletedAt = &now

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			agent.Status = StatusTimeout
		} else {
			agent.Status = StatusFailed
		}
		agent.Error = err.Error()
		r.logger.Warn("sub-agent failed",
			"id", agent.ID,
			"error", err,
		)
	} else {
		agent.Status = StatusComplete
		agent.Result = result
		r.logger.Info("sub-agent completed",
			"id", agent.ID,
			"result_len", len(result),
		)
	}
	agent.mu.Unlock()

	r.notify(agent)
	r.persist()
}

// Get retrieves an agent by ID.
func (r *Registry) Get(id string) *Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.agents[id]
}

// GetStatus returns the current status of an agent.
func (r *Registry) GetStatus(id string) (Status, error) {
	agent := r.Get(id)
	if agent == nil {
		return "", fmt.Errorf("agent not found: %s", id)
	}
	agent.mu.RLock()
	defer agent.mu.RUnlock()
	return agent.Status, nil
}

// Cancel stops a running agent.
func (r *Registry) Cancel(id string) error {
	agent := r.Get(id)
	if agent == nil {
		return fmt.Errorf("agent not found: %s", id)
	}

	agent.mu.Lock()

	if agent.Status != StatusRunning && agent.Status != StatusPending {
		agent.mu.Unlock()
		return fmt.Errorf("agent is not running (status: %s)", agent.Status)
	}

	if agent.cancelFunc != nil {
		agent.cancelFunc()
	}

	agent.Status = StatusCanceled
	now := time.Now()
	agent.CompletedAt = &now

	// Audit log: agent cancellation
	r.logger.Info("agent canceled",
		"id", id,
		"parent_id", agent.ParentID,
		"user_id", agent.UserID,
		"platform", agent.Platform,
		"task", truncate(agent.Task, 100),
	)

	agent.mu.Unlock()

	r.persist()

	return nil
}

// SendMessage sends a message to an agent's inbox.
func (r *Registry) SendMessage(toAgentID string, msg Message) error {
	agent := r.Get(toAgentID)
	if agent == nil {
		return fmt.Errorf("agent not found: %s", toAgentID)
	}

	msg.ID = uuid.New().String()[:8]
	msg.ToAgent = toAgentID
	msg.Timestamp = time.Now()

	select {
	case agent.inbox <- msg:
		r.logger.Debug("message sent to agent",
			"from", msg.FromAgent,
			"to", toAgentID,
		)
		return nil
	default:
		return fmt.Errorf("agent inbox full")
	}
}

// ReceiveMessage reads a message from an agent's inbox (blocking with timeout).
func (r *Registry) ReceiveMessage(agentID string, timeout time.Duration) (*Message, error) {
	agent := r.Get(agentID)
	if agent == nil {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}

	select {
	case msg := <-agent.inbox:
		return &msg, nil
	case <-time.After(timeout):
		return nil, nil // No message available
	}
}

// AddMessage adds a message to agent's history.
func (r *Registry) AddMessage(agent *Agent, role, content string) {
	agent.mu.Lock()
	defer agent.mu.Unlock()

	agent.Messages = append(agent.Messages, Message{
		ID:        uuid.New().String()[:8],
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	})

	// Trim history if too long
	if len(agent.Messages) > r.maxHistory {
		agent.Messages = agent.Messages[len(agent.Messages)-r.maxHistory:]
	}
}

// GetHistory returns recent messages from an agent.
func (r *Registry) GetHistory(agent *Agent, limit int) []Message {
	agent.mu.RLock()
	defer agent.mu.RUnlock()

	if limit <= 0 || limit > len(agent.Messages) {
		limit = len(agent.Messages)
	}

	start := len(agent.Messages) - limit
	if start < 0 {
		start = 0
	}

	// Return a copy
	src := agent.Messages[start:]
	dst := make([]Message, len(src))
	copy(dst, src)
	return dst
}

// MaxContextKeys is the maximum number of context entries per agent.
const MaxContextKeys = 100

// MaxContextKeyLength is the maximum length of a context key.
const MaxContextKeyLength = 256

// SetContext sets a context value for an agent.
func (r *Registry) SetContext(agent *Agent, key string, value interface{}) error {
	// Validate key
	if key == "" {
		return fmt.Errorf("context key cannot be empty")
	}
	if len(key) > MaxContextKeyLength {
		return fmt.Errorf("context key too long (max %d characters)", MaxContextKeyLength)
	}

	agent.mu.Lock()
	defer agent.mu.Unlock()

	if agent.Context == nil {
		agent.Context = make(map[string]interface{})
	}

	// Check if adding new key would exceed limit
	if _, exists := agent.Context[key]; !exists && len(agent.Context) >= MaxContextKeys {
		return fmt.Errorf("max context entries reached (%d)", MaxContextKeys)
	}

	agent.Context[key] = value
	return nil
}

// GetContext gets a context value from an agent.
func (r *Registry) GetContext(agent *Agent, key string) interface{} {
	agent.mu.RLock()
	defer agent.mu.RUnlock()

	if agent.Context == nil {
		return nil
	}
	return agent.Context[key]
}

// List returns all agents, optionally filtered.
func (r *Registry) List(filter ListFilter) []*Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Agent, 0)
	for _, agent := range r.agents {
		if filter.Status != "" && agent.Status != filter.Status {
			continue
		}
		if filter.ParentID != "" && agent.ParentID != filter.ParentID {
			continue
		}
		if filter.UserID != "" && agent.UserID != filter.UserID {
			continue
		}
		if !filter.IncludeComplete && (agent.Status == StatusComplete || agent.Status == StatusFailed || agent.Status == StatusCanceled) {
			continue
		}
		result = append(result, agent)
	}

	return result
}

// ListFilter specifies agent list filtering options.
type ListFilter struct {
	Status          Status
	ParentID        string
	UserID          string
	IncludeComplete bool
}

// Children returns all child agents of a parent.
func (r *Registry) Children(parentID string) []*Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	childIDs := r.byParent[parentID]
	children := make([]*Agent, 0, len(childIDs))

	for _, id := range childIDs {
		if agent, ok := r.agents[id]; ok {
			children = append(children, agent)
		}
	}

	return children
}

// WaitFor blocks until an agent completes or timeout.
func (r *Registry) WaitFor(id string, timeout time.Duration) (*Agent, error) {
	deadline := time.Now().Add(timeout)

	for {
		agent := r.Get(id)
		if agent == nil {
			return nil, fmt.Errorf("agent not found: %s", id)
		}

		agent.mu.RLock()
		status := agent.Status
		agent.mu.RUnlock()

		switch status {
		case StatusComplete, StatusFailed, StatusCanceled, StatusTimeout:
			return agent, nil
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timeout waiting for agent %s", id)
		}

		time.Sleep(100 * time.Millisecond)
	}
}

// Cleanup removes completed agents older than the specified duration.
func (r *Registry) Cleanup(olderThan time.Duration) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	cutoff := time.Now().Add(-olderThan)
	count := 0

	for id, agent := range r.agents {
		agent.mu.RLock()
		isComplete := agent.Status == StatusComplete || agent.Status == StatusFailed || agent.Status == StatusCanceled || agent.Status == StatusTimeout
		completedAt := agent.CompletedAt
		agent.mu.RUnlock()

		if isComplete && completedAt != nil && completedAt.Before(cutoff) {
			delete(r.agents, id)
			// Clean up parent references
			if agent.ParentID != "" {
				children := r.byParent[agent.ParentID]
				for i, childID := range children {
					if childID == id {
						r.byParent[agent.ParentID] = append(children[:i], children[i+1:]...)
						break
					}
				}
			}
			count++
		}
	}

	if count > 0 {
		r.logger.Info("cleaned up completed agents", "count", count)
	}

	return count
}

// Stats returns registry statistics.
func (r *Registry) Stats() map[string]int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := map[string]int{
		"total":    len(r.agents),
		"pending":  0,
		"running":  0,
		"complete": 0,
		"failed":   0,
		"canceled": 0,
		"timeout":  0,
	}

	for _, agent := range r.agents {
		agent.mu.RLock()
		switch agent.Status {
		case StatusPending:
			stats["pending"]++
		case StatusRunning:
			stats["running"]++
		case StatusComplete:
			stats["complete"]++
		case StatusFailed:
			stats["failed"]++
		case StatusCanceled:
			stats["canceled"]++
		case StatusTimeout:
			stats["timeout"]++
		}
		agent.mu.RUnlock()
	}

	return stats
}

// getDepth calculates the nesting depth of an agent (must hold mu).
func (r *Registry) getDepth(parentID string) int {
	if parentID == "" {
		return 0
	}

	depth := 0
	currentID := parentID

	for currentID != "" && depth < r.maxDepth+1 {
		if agent, ok := r.agents[currentID]; ok {
			currentID = agent.ParentID
			depth++
		} else {
			break
		}
	}

	return depth
}

// notify calls the notification function if set.
func (r *Registry) notify(agent *Agent) {
	if r.notifyFunc != nil {
		r.notifyFunc(agent)
	}
}

// persist saves the current state to disk.
func (r *Registry) persist() {
	if r.persistPath == "" {
		return
	}

	r.mu.RLock()
	agents := make([]*Agent, 0, len(r.agents))
	for _, agent := range r.agents {
		// Create a deep copy for serialization (avoid copying mutex and map references)
		agent.mu.RLock()
		var ctxCopy map[string]interface{}
		if agent.Context != nil {
			ctxCopy = make(map[string]interface{}, len(agent.Context))
			for k, v := range agent.Context {
				ctxCopy[k] = v
			}
		}
		var metaCopy map[string]string
		if agent.Metadata != nil {
			metaCopy = make(map[string]string, len(agent.Metadata))
			for k, v := range agent.Metadata {
				metaCopy[k] = v
			}
		}
		msgsCopy := make([]Message, len(agent.Messages))
		copy(msgsCopy, agent.Messages)
		agentCopy := &Agent{
			ID:          agent.ID,
			ParentID:    agent.ParentID,
			Name:        agent.Name,
			Task:        agent.Task,
			Status:      agent.Status,
			Result:      agent.Result,
			Error:       agent.Error,
			Context:     ctxCopy,
			Messages:    msgsCopy,
			Metadata:    metaCopy,
			Platform:    agent.Platform,
			ChatID:      agent.ChatID,
			UserID:      agent.UserID,
			Priority:    agent.Priority,
			Timeout:     agent.Timeout,
			CreatedAt:   agent.CreatedAt,
			StartedAt:   agent.StartedAt,
			CompletedAt: agent.CompletedAt,
		}
		agent.mu.RUnlock()
		agents = append(agents, agentCopy)
	}
	r.mu.RUnlock()

	data, err := json.MarshalIndent(agents, "", "  ")
	if err != nil {
		r.logger.Error("failed to marshal agents", "error", err)
		return
	}

	if err := os.MkdirAll(filepath.Dir(r.persistPath), 0700); err != nil {
		r.logger.Error("failed to create data dir", "error", err)
		return
	}

	tmpFile := r.persistPath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		r.logger.Error("failed to write agents", "error", err)
		return
	}

	if err := os.Rename(tmpFile, r.persistPath); err != nil {
		os.Remove(tmpFile)
		r.logger.Error("failed to rename agents file", "error", err)
	}
}

// load reads the persisted state from disk.
func (r *Registry) load() error {
	data, err := os.ReadFile(r.persistPath)
	if err != nil {
		return err
	}

	var agents []*Agent
	if err := json.Unmarshal(data, &agents); err != nil {
		return fmt.Errorf("parse agents: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, agent := range agents {
		// Reset internal state
		agent.inbox = make(chan Message, 100)
		agent.cancelFunc = nil

		// Mark incomplete agents as failed (they didn't survive restart)
		if agent.Status == StatusPending || agent.Status == StatusRunning {
			agent.Status = StatusFailed
			agent.Error = "interrupted by restart"
			now := time.Now()
			agent.CompletedAt = &now
		}

		r.agents[agent.ID] = agent
		if agent.ParentID != "" {
			r.byParent[agent.ParentID] = append(r.byParent[agent.ParentID], agent.ID)
		}
	}

	r.logger.Info("loaded persisted agents", "count", len(agents))
	return nil
}

// truncate shortens a string for logging.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

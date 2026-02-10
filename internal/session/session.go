// Package session provides multi-session management
package session

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kusa/magabot/internal/util"
)

// Status represents session status
type Status string

const (
	StatusPending  Status = "pending"
	StatusRunning  Status = "running"
	StatusComplete Status = "complete"
	StatusFailed   Status = "failed"
	StatusCanceled Status = "canceled"
)

// Session represents a chat session or sub-task
type Session struct {
	ID          string                 `json:"id"`
	ParentID    string                 `json:"parent_id,omitempty"` // For sub-sessions
	Type        string                 `json:"type"`                // main, sub, background
	UserID      string                 `json:"user_id"`
	Platform    string                 `json:"platform"`
	ChatID      string                 `json:"chat_id"`
	Task        string                 `json:"task,omitempty"`       // For sub-sessions
	Status      Status                 `json:"status"`
	Result      string                 `json:"result,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Context     map[string]interface{} `json:"context,omitempty"`
	Messages    []Message              `json:"messages,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
	cancelFunc  context.CancelFunc     // internal: cancels the sub-session goroutine
}

// Message represents a chat message
type Message struct {
	Role      string    `json:"role"` // user, assistant, system
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// TaskFunc is the function signature for background tasks
type TaskFunc func(ctx context.Context, session *Session) (result string, err error)

// NotifyFunc is called when a sub-session completes
type NotifyFunc func(platform, chatID, message string) error

// Manager manages multiple sessions
type Manager struct {
	mu         sync.RWMutex
	sessions   map[string]*Session
	notify     NotifyFunc
	taskRunner TaskRunner
	maxHistory int // Max messages to keep per session
	subCounter atomic.Int64 // monotonic counter for unique sub-session IDs
	logger     *slog.Logger
}

// TaskRunner executes tasks (usually LLM calls)
type TaskRunner interface {
	Execute(ctx context.Context, task string, sessionContext []Message) (string, error)
}

// NewManager creates a new session manager
func NewManager(notify NotifyFunc, maxHistory int, logger *slog.Logger) *Manager {
	if maxHistory <= 0 {
		maxHistory = 50
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &Manager{
		sessions:   make(map[string]*Session),
		notify:     notify,
		maxHistory: maxHistory,
		logger:     logger,
	}
}

// SetTaskRunner sets the task runner (LLM)
func (m *Manager) SetTaskRunner(runner TaskRunner) {
	m.taskRunner = runner
}

// GetOrCreate gets an existing session or creates a new one
func (m *Manager) GetOrCreate(platform, chatID, userID string) *Session {
	key := fmt.Sprintf("%s:%s", platform, chatID)
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if session, ok := m.sessions[key]; ok {
		return session
	}
	
	session := &Session{
		ID:        key,
		Type:      "main",
		UserID:    userID,
		Platform:  platform,
		ChatID:    chatID,
		Status:    StatusRunning,
		Messages:  make([]Message, 0),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	
	m.sessions[key] = session
	return session
}

// Get retrieves a session by ID
func (m *Manager) Get(id string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[id]
}

// AddMessage adds a message to session history
func (m *Manager) AddMessage(session *Session, role, content string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	session.Messages = append(session.Messages, Message{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	})
	session.UpdatedAt = time.Now()
	
	// Trim history if too long
	if len(session.Messages) > m.maxHistory {
		session.Messages = session.Messages[len(session.Messages)-m.maxHistory:]
	}
}

// GetHistory returns recent messages for context
func (m *Manager) GetHistory(session *Session, limit int) []Message {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if limit <= 0 || limit > len(session.Messages) {
		limit = len(session.Messages)
	}
	
	start := len(session.Messages) - limit
	if start < 0 {
		start = 0
	}
	
	return session.Messages[start:]
}

// Spawn creates a sub-session for background task
func (m *Manager) Spawn(parent *Session, task string) (*Session, error) {
	subID := fmt.Sprintf("%s:sub:%d", parent.ID, m.subCounter.Add(1))
	
	sub := &Session{
		ID:        subID,
		ParentID:  parent.ID,
		Type:      "sub",
		UserID:    parent.UserID,
		Platform:  parent.Platform,
		ChatID:    parent.ChatID,
		Task:      task,
		Status:    StatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	
	m.mu.Lock()
	m.sessions[subID] = sub
	m.mu.Unlock()
	
	// Run task in background
	go m.runSubSession(sub)
	
	return sub, nil
}

// runSubSession executes a sub-session task
func (m *Manager) runSubSession(session *Session) {
	m.logger.Info("starting sub-session", "id", session.ID, "task", session.Task)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	m.mu.Lock()
	session.Status = StatusRunning
	session.cancelFunc = cancel
	m.mu.Unlock()
	
	var result string
	var err error
	
	if m.taskRunner != nil {
		// Get parent context
		parent := m.Get(session.ParentID)
		var context []Message
		if parent != nil {
			context = m.GetHistory(parent, 10)
		}
		
		result, err = m.taskRunner.Execute(ctx, session.Task, context)
	} else {
		err = fmt.Errorf("no task runner configured")
	}
	
	now := time.Now()
	
	m.mu.Lock()
	session.CompletedAt = &now
	session.UpdatedAt = now
	
	if err != nil {
		session.Status = StatusFailed
		session.Error = err.Error()
		m.logger.Warn("sub-session failed", "id", session.ID, "error", err)
	} else {
		session.Status = StatusComplete
		session.Result = result
		m.logger.Info("sub-session completed", "id", session.ID)
	}
	m.mu.Unlock()
	
	// Notify parent
	m.notifyCompletion(session)
}

// notifyCompletion sends notification when sub-session completes
func (m *Manager) notifyCompletion(session *Session) {
	if m.notify == nil {
		return
	}
	
	var message string
	if session.Status == StatusComplete {
		message = fmt.Sprintf("✅ *Task Complete*\n\n📋 %s\n\n%s", 
			util.Truncate(session.Task, 100), 
			util.Truncate(session.Result, 1000))
	} else {
		message = fmt.Sprintf("❌ *Task Failed*\n\n📋 %s\n\n⚠️ %s",
			util.Truncate(session.Task, 100),
			session.Error)
	}
	
	if err := m.notify(session.Platform, session.ChatID, message); err != nil {
		m.logger.Error("failed to notify", "error", err)
	}
}

// List returns all sessions, optionally filtered
func (m *Manager) List(userID string, includeComplete bool) []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	result := make([]*Session, 0)
	for _, session := range m.sessions {
		if userID != "" && session.UserID != userID {
			continue
		}
		if !includeComplete && (session.Status == StatusComplete || session.Status == StatusFailed) {
			continue
		}
		result = append(result, session)
	}
	
	return result
}

// ListSubSessions returns sub-sessions for a parent
func (m *Manager) ListSubSessions(parentID string) []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	result := make([]*Session, 0)
	for _, session := range m.sessions {
		if session.ParentID == parentID {
			result = append(result, session)
		}
	}
	
	return result
}

// Cancel cancels a running session
func (m *Manager) Cancel(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found")
	}

	if session.Status != StatusRunning && session.Status != StatusPending {
		return fmt.Errorf("session not running")
	}

	// Cancel the context to stop the goroutine
	if session.cancelFunc != nil {
		session.cancelFunc()
	}

	session.Status = StatusCanceled
	now := time.Now()
	session.CompletedAt = &now
	session.UpdatedAt = now

	return nil
}

// Clear removes completed sessions older than duration
func (m *Manager) Clear(olderThan time.Duration) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	cutoff := time.Now().Add(-olderThan)
	count := 0
	
	for id, session := range m.sessions {
		if session.Status == StatusComplete || session.Status == StatusFailed || session.Status == StatusCanceled {
			if session.CompletedAt != nil && session.CompletedAt.Before(cutoff) {
				delete(m.sessions, id)
				count++
			}
		}
	}
	
	return count
}

// SetContext sets session context (for passing data between messages)
func (m *Manager) SetContext(session *Session, key string, value interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if session.Context == nil {
		session.Context = make(map[string]interface{})
	}
	session.Context[key] = value
}

// GetContext gets a context value
func (m *Manager) GetContext(session *Session, key string) interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if session.Context == nil {
		return nil
	}
	return session.Context[key]
}

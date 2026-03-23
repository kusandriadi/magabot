package bot

import (
	"fmt"
	"sync"
	"time"
)

// ActionFunc is called when a pending action is confirmed.
type ActionFunc func() (string, error)

// PendingAction represents an action waiting for user confirmation.
type PendingAction struct {
	UserID    string
	OnConfirm ActionFunc
	CreatedAt time.Time
	Timeout   time.Duration
}

// ConfirmationManager tracks one pending action per chat.
type ConfirmationManager struct {
	mu      sync.Mutex
	pending map[string]*PendingAction // key: "platform:chatID"
}

// NewConfirmationManager creates a new manager.
func NewConfirmationManager() *ConfirmationManager {
	return &ConfirmationManager{
		pending: make(map[string]*PendingAction),
	}
}

// Request registers a pending action for a chat.
// Returns the prompt message to send to the user.
func (m *ConfirmationManager) Request(platform, chatID, userID, description string, timeout time.Duration, onConfirm ActionFunc) string {
	key := platform + ":" + chatID

	m.mu.Lock()
	defer m.mu.Unlock()

	m.pending[key] = &PendingAction{
		UserID:    userID,
		OnConfirm: onConfirm,
		CreatedAt: time.Now(),
		Timeout:   timeout,
	}

	return fmt.Sprintf("%s\n\n_Reply *y* or *n*_", description)
}

// Confirm attempts to confirm the pending action.
func (m *ConfirmationManager) Confirm(platform, chatID, userID string) (string, bool) {
	key := platform + ":" + chatID

	m.mu.Lock()
	action, ok := m.pending[key]
	if !ok {
		m.mu.Unlock()
		return "", false
	}

	if time.Since(action.CreatedAt) > action.Timeout {
		delete(m.pending, key)
		m.mu.Unlock()
		return "⏰ Action expired. Please try again.", true
	}

	if action.UserID != userID {
		m.mu.Unlock()
		return "🔒 Only the original requester can confirm.", true
	}

	delete(m.pending, key)
	m.mu.Unlock()

	result, err := action.OnConfirm()
	if err != nil {
		return fmt.Sprintf("❌ %v", err), true
	}
	return result, true
}

// Cancel cancels the pending action.
func (m *ConfirmationManager) Cancel(platform, chatID, userID string) (string, bool) {
	key := platform + ":" + chatID

	m.mu.Lock()
	defer m.mu.Unlock()

	action, ok := m.pending[key]
	if !ok {
		return "", false
	}

	if action.UserID != userID {
		return "🔒 Only the original requester can cancel.", true
	}

	delete(m.pending, key)
	return "❌ Canceled.", true
}

// HasPending returns true if the chat has a non-expired pending action.
func (m *ConfirmationManager) HasPending(platform, chatID string) bool {
	key := platform + ":" + chatID

	m.mu.Lock()
	defer m.mu.Unlock()

	action, ok := m.pending[key]
	if !ok {
		return false
	}
	if time.Since(action.CreatedAt) > action.Timeout {
		delete(m.pending, key)
		return false
	}
	return true
}

// Cleanup removes expired actions.
func (m *ConfirmationManager) Cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for key, action := range m.pending {
		if time.Since(action.CreatedAt) > action.Timeout {
			delete(m.pending, key)
		}
	}
}

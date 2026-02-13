// Package security - Session management with timeout (A07 fix)
package security

import (
	"sync"
	"time"
)

const (
	// SessionTimeout is the maximum lifetime of a session
	SessionTimeout = 24 * time.Hour
	// IdleTimeout is the maximum time between requests
	IdleTimeout = 4 * time.Hour
	// MaxFailedAttempts before lockout
	MaxFailedAttempts = 5
	// LockoutDuration after max failed attempts
	LockoutDuration = 15 * time.Minute
)

var (
	ErrSessionExpired = errNew("session expired")
	ErrSessionIdle    = errNew("session idle timeout")
	ErrAccountLocked  = errNew("account temporarily locked")
)

func errNew(msg string) error {
	return &securityError{msg: msg}
}

type securityError struct {
	msg string
}

func (e *securityError) Error() string {
	return e.msg
}

// Session represents a user session with timeout tracking
type Session struct {
	UserID    string
	Platform  string
	CreatedAt time.Time
	ExpiresAt time.Time
	LastSeen  time.Time
}

// IsValid checks if the session is still valid
func (s *Session) IsValid() bool {
	now := time.Now()
	
	// Check absolute expiry
	if now.After(s.ExpiresAt) {
		return false
	}
	
	// Check idle timeout
	if now.Sub(s.LastSeen) > IdleTimeout {
		return false
	}
	
	return true
}

// Touch updates the last seen time
func (s *Session) Touch() {
	s.LastSeen = time.Now()
}

// SessionManager manages user sessions
type SessionManager struct {
	sessions map[string]*Session // key: platform:userID
	mu       sync.RWMutex
	done     chan struct{}
}

// NewSessionManager creates a new session manager
func NewSessionManager() *SessionManager {
	sm := &SessionManager{
		sessions: make(map[string]*Session),
		done:     make(chan struct{}),
	}

	// Start cleanup goroutine
	go sm.cleanupLoop()

	return sm
}

// Stop signals the cleanup goroutine to exit
func (sm *SessionManager) Stop() {
	close(sm.done)
}

func (sm *SessionManager) sessionKey(platform, userID string) string {
	return platform + ":" + userID
}

// GetOrCreate gets an existing session or creates a new one
func (sm *SessionManager) GetOrCreate(platform, userID string) *Session {
	key := sm.sessionKey(platform, userID)
	
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	if sess, ok := sm.sessions[key]; ok && sess.IsValid() {
		sess.Touch()
		return sess
	}
	
	// Create new session
	now := time.Now()
	sess := &Session{
		UserID:    userID,
		Platform:  platform,
		CreatedAt: now,
		ExpiresAt: now.Add(SessionTimeout),
		LastSeen:  now,
	}
	sm.sessions[key] = sess
	
	return sess
}

// Validate checks if a user has a valid session
func (sm *SessionManager) Validate(platform, userID string) error {
	key := sm.sessionKey(platform, userID)
	
	sm.mu.RLock()
	sess, ok := sm.sessions[key]
	sm.mu.RUnlock()
	
	if !ok {
		return nil // No session yet, will be created on next GetOrCreate
	}
	
	now := time.Now()
	
	if now.After(sess.ExpiresAt) {
		sm.Invalidate(platform, userID)
		return ErrSessionExpired
	}
	
	if now.Sub(sess.LastSeen) > IdleTimeout {
		sm.Invalidate(platform, userID)
		return ErrSessionIdle
	}
	
	return nil
}

// Invalidate removes a session
func (sm *SessionManager) Invalidate(platform, userID string) {
	key := sm.sessionKey(platform, userID)
	
	sm.mu.Lock()
	delete(sm.sessions, key)
	sm.mu.Unlock()
}

// cleanupLoop removes expired sessions periodically
func (sm *SessionManager) cleanupLoop() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sm.cleanup()
		case <-sm.done:
			return
		}
	}
}

func (sm *SessionManager) cleanup() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	for key, sess := range sm.sessions {
		if !sess.IsValid() {
			delete(sm.sessions, key)
		}
	}
}

// AuthAttempts tracks failed authentication attempts
type AuthAttempts struct {
	attempts map[string][]time.Time // key -> timestamps of failures
	mu       sync.Mutex
}

// NewAuthAttempts creates a new auth attempts tracker
func NewAuthAttempts() *AuthAttempts {
	return &AuthAttempts{
		attempts: make(map[string][]time.Time),
	}
}

// RecordFailure records a failed authentication attempt
func (a *AuthAttempts) RecordFailure(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	now := time.Now()
	cutoff := now.Add(-LockoutDuration)
	
	// Clean old entries
	var recent []time.Time
	for _, t := range a.attempts[key] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	
	a.attempts[key] = append(recent, now)
}

// IsLocked checks if an account is locked due to too many failures
func (a *AuthAttempts) IsLocked(key string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	now := time.Now()
	cutoff := now.Add(-LockoutDuration)
	
	var recentCount int
	for _, t := range a.attempts[key] {
		if t.After(cutoff) {
			recentCount++
		}
	}
	
	return recentCount >= MaxFailedAttempts
}

// ClearFailures clears failed attempts for a key (after successful auth)
func (a *AuthAttempts) ClearFailures(key string) {
	a.mu.Lock()
	delete(a.attempts, key)
	a.mu.Unlock()
}

// Cleanup removes old entries
func (a *AuthAttempts) Cleanup() {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	cutoff := time.Now().Add(-LockoutDuration * 2)
	
	for key, times := range a.attempts {
		var recent []time.Time
		for _, t := range times {
			if t.After(cutoff) {
				recent = append(recent, t)
			}
		}
		if len(recent) == 0 {
			delete(a.attempts, key)
		} else {
			a.attempts[key] = recent
		}
	}
}

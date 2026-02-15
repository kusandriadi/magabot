// Package security - Security event logging (A09 fix)
package security

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SecurityEventType defines the type of security event
type SecurityEventType string

const (
	EventAuthSuccess     SecurityEventType = "auth_success"
	EventAuthFailure     SecurityEventType = "auth_failure"
	EventAuthLockout     SecurityEventType = "auth_lockout"
	EventSessionCreated  SecurityEventType = "session_created"
	EventSessionExpired  SecurityEventType = "session_expired"
	EventSessionInvalid  SecurityEventType = "session_invalid"
	EventAdminAction     SecurityEventType = "admin_action"
	EventConfigChange    SecurityEventType = "config_change"
	EventRateLimited     SecurityEventType = "rate_limited"
	EventAccessDenied    SecurityEventType = "access_denied"
	EventEncryptError    SecurityEventType = "encrypt_error"
	EventDecryptError    SecurityEventType = "decrypt_error"
	EventSSRFBlocked     SecurityEventType = "ssrf_blocked"
	EventInputSanitized  SecurityEventType = "input_sanitized"
	EventSuspiciousInput SecurityEventType = "suspicious_input"
)

// SecurityEvent represents a security-related event
type SecurityEvent struct {
	Timestamp   time.Time         `json:"timestamp"`
	EventType   SecurityEventType `json:"event_type"`
	Platform    string            `json:"platform,omitempty"`
	UserID      string            `json:"user_id,omitempty"` // hashed for privacy
	IP          string            `json:"ip,omitempty"`
	Success     bool              `json:"success"`
	Details     string            `json:"details,omitempty"`
	Severity    string            `json:"severity"` // info, warning, critical
	RequestID   string            `json:"request_id,omitempty"`
}

// AuditLogger logs security events
type AuditLogger struct {
	writer    io.Writer
	mu        sync.Mutex
	logPath   string
	maxSizeMB int
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(logDir string) (*AuditLogger, error) {
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}
	
	logPath := filepath.Join(logDir, "security.log")
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open security log: %w", err)
	}
	
	return &AuditLogger{
		writer:    file,
		logPath:   logPath,
		maxSizeMB: 50, // 50MB max before rotation
	}, nil
}

// Log writes a security event to the audit log
func (a *AuditLogger) Log(event SecurityEvent) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	
	if event.Severity == "" {
		event.Severity = a.inferSeverity(event.EventType)
	}
	
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}
	
	a.mu.Lock()
	defer a.mu.Unlock()
	
	// Check for rotation
	a.rotateIfNeeded()
	
	_, err = fmt.Fprintf(a.writer, "%s\n", data)
	return err
}

func (a *AuditLogger) inferSeverity(eventType SecurityEventType) string {
	switch eventType {
	case EventAuthLockout, EventSSRFBlocked, EventSuspiciousInput:
		return "critical"
	case EventAuthFailure, EventAccessDenied, EventRateLimited:
		return "warning"
	default:
		return "info"
	}
}

func (a *AuditLogger) rotateIfNeeded() {
	info, err := os.Stat(a.logPath)
	if err != nil {
		return
	}

	sizeMB := info.Size() / (1024 * 1024)
	if sizeMB < int64(a.maxSizeMB) {
		return
	}

	// Rotate: rename first, then open new file, then close old writer.
	// This avoids a window where a.writer points to a closed file.
	timestamp := time.Now().Format("20060102-150405")
	rotatedPath := fmt.Sprintf("%s.%s", a.logPath, timestamp)
	if err := os.Rename(a.logPath, rotatedPath); err != nil {
		return // keep writing to current file
	}

	// Open new file
	file, err := os.OpenFile(a.logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		// Undo rotation so the old path is restored
		_ = os.Rename(rotatedPath, a.logPath)
		return
	}

	// Close old writer after new one is ready
	oldWriter := a.writer
	a.writer = file
	if closer, ok := oldWriter.(io.Closer); ok {
		closer.Close()
	}
}

// Close closes the audit logger
func (a *AuditLogger) Close() error {
	if closer, ok := a.writer.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// Helper functions for common events

// LogAuthSuccess logs a successful authentication
func (a *AuditLogger) LogAuthSuccess(platform, userID string) {
	a.Log(SecurityEvent{
		EventType: EventAuthSuccess,
		Platform:  platform,
		UserID:    HashUserID(platform, userID),
		Success:   true,
	})
}

// LogAuthFailure logs a failed authentication attempt
func (a *AuditLogger) LogAuthFailure(platform, userID, reason string) {
	a.Log(SecurityEvent{
		EventType: EventAuthFailure,
		Platform:  platform,
		UserID:    HashUserID(platform, userID),
		Success:   false,
		Details:   reason,
	})
}

// LogAuthLockout logs an account lockout
func (a *AuditLogger) LogAuthLockout(platform, userID string) {
	a.Log(SecurityEvent{
		EventType: EventAuthLockout,
		Platform:  platform,
		UserID:    HashUserID(platform, userID),
		Success:   false,
		Details:   "account locked due to repeated failures",
		Severity:  "critical",
	})
}

// LogRateLimited logs a rate limit event
func (a *AuditLogger) LogRateLimited(platform, userID string) {
	a.Log(SecurityEvent{
		EventType: EventRateLimited,
		Platform:  platform,
		UserID:    HashUserID(platform, userID),
		Success:   false,
	})
}

// LogSSRFBlocked logs a blocked SSRF attempt
func (a *AuditLogger) LogSSRFBlocked(platform, userID, url string) {
	a.Log(SecurityEvent{
		EventType: EventSSRFBlocked,
		Platform:  platform,
		UserID:    HashUserID(platform, userID),
		Success:   false,
		Details:   fmt.Sprintf("blocked URL: %s", url),
		Severity:  "critical",
	})
}

// LogAdminAction logs an admin action
func (a *AuditLogger) LogAdminAction(platform, userID, action string) {
	a.Log(SecurityEvent{
		EventType: EventAdminAction,
		Platform:  platform,
		UserID:    HashUserID(platform, userID),
		Success:   true,
		Details:   action,
	})
}

// LogConfigChange logs a configuration change
func (a *AuditLogger) LogConfigChange(platform, userID, change string) {
	a.Log(SecurityEvent{
		EventType: EventConfigChange,
		Platform:  platform,
		UserID:    HashUserID(platform, userID),
		Success:   true,
		Details:   change,
	})
}

// LogAccessDenied logs an access denied event
func (a *AuditLogger) LogAccessDenied(platform, userID, resource string) {
	a.Log(SecurityEvent{
		EventType: EventAccessDenied,
		Platform:  platform,
		UserID:    HashUserID(platform, userID),
		Success:   false,
		Details:   fmt.Sprintf("denied access to: %s", resource),
	})
}

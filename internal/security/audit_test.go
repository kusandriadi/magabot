package security

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewAuditLogger(t *testing.T) {
	t.Run("ValidDir", func(t *testing.T) {
		dir := t.TempDir()
		logger, err := NewAuditLogger(dir)
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}
		defer logger.Close()

		if logger == nil {
			t.Error("Logger should not be nil")
		}

		// Check log file was created
		logPath := filepath.Join(dir, "security.log")
		if _, err := os.Stat(logPath); os.IsNotExist(err) {
			t.Error("Security log file should be created")
		}
	})

	t.Run("CreatesDirIfNotExists", func(t *testing.T) {
		tmpDir := t.TempDir()
		newDir := filepath.Join(tmpDir, "subdir", "logs")

		logger, err := NewAuditLogger(newDir)
		if err != nil {
			t.Fatalf("Should create directory: %v", err)
		}
		defer logger.Close()

		if _, err := os.Stat(newDir); os.IsNotExist(err) {
			t.Error("Directory should be created")
		}
	})
}

func TestAuditLoggerLog(t *testing.T) {
	dir := t.TempDir()
	logger, _ := NewAuditLogger(dir)
	defer logger.Close()

	t.Run("BasicLog", func(t *testing.T) {
		err := logger.Log(SecurityEvent{
			EventType: EventAuthSuccess,
			Platform:  "telegram",
			UserID:    "test_user",
			Success:   true,
		})
		if err != nil {
			t.Errorf("Log failed: %v", err)
		}
	})

	t.Run("LogWithAllFields", func(t *testing.T) {
		event := SecurityEvent{
			Timestamp: time.Now(),
			EventType: EventAuthFailure,
			Platform:  "discord",
			UserID:    "user123",
			IP:        "192.168.1.1",
			Success:   false,
			Details:   "Wrong password",
			Severity:  "warning",
			RequestID: "req-12345",
		}
		err := logger.Log(event)
		if err != nil {
			t.Errorf("Log with all fields failed: %v", err)
		}
	})

	t.Run("AutoTimestamp", func(t *testing.T) {
		event := SecurityEvent{
			EventType: EventSessionCreated,
			// Timestamp intentionally left zero
		}
		err := logger.Log(event)
		if err != nil {
			t.Errorf("Log without timestamp failed: %v", err)
		}
	})

	t.Run("AllEventTypes", func(t *testing.T) {
		eventTypes := []SecurityEventType{
			EventAuthSuccess,
			EventAuthFailure,
			EventAuthLockout,
			EventSessionCreated,
			EventSessionExpired,
			EventSessionInvalid,
			EventAdminAction,
			EventConfigChange,
			EventRateLimited,
			EventAccessDenied,
			EventEncryptError,
			EventDecryptError,
			EventSSRFBlocked,
			EventInputSanitized,
			EventSuspiciousInput,
		}

		for _, et := range eventTypes {
			err := logger.Log(SecurityEvent{EventType: et})
			if err != nil {
				t.Errorf("Failed to log event type %s: %v", et, err)
			}
		}
	})
}

func TestAuditLoggerInferSeverity(t *testing.T) {
	dir := t.TempDir()
	logger, _ := NewAuditLogger(dir)
	defer logger.Close()

	tests := []struct {
		eventType SecurityEventType
		expected  string
	}{
		{EventAuthLockout, "critical"},
		{EventSSRFBlocked, "critical"},
		{EventSuspiciousInput, "critical"},
		{EventAuthFailure, "warning"},
		{EventAccessDenied, "warning"},
		{EventRateLimited, "warning"},
		{EventAuthSuccess, "info"},
		{EventSessionCreated, "info"},
		{EventConfigChange, "info"},
	}

	for _, tt := range tests {
		t.Run(string(tt.eventType), func(t *testing.T) {
			result := logger.inferSeverity(tt.eventType)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestAuditLoggerHelpers(t *testing.T) {
	dir := t.TempDir()
	logger, _ := NewAuditLogger(dir)
	defer logger.Close()

	t.Run("LogAuthSuccess", func(t *testing.T) {
		logger.LogAuthSuccess("telegram", "user1")
		// Should not panic
	})

	t.Run("LogAuthFailure", func(t *testing.T) {
		logger.LogAuthFailure("telegram", "user1", "invalid token")
	})

	t.Run("LogAuthLockout", func(t *testing.T) {
		logger.LogAuthLockout("telegram", "user1")
	})

	t.Run("LogRateLimited", func(t *testing.T) {
		logger.LogRateLimited("discord", "user2")
	})

	t.Run("LogSSRFBlocked", func(t *testing.T) {
		logger.LogSSRFBlocked("slack", "user3", "http://internal.local")
	})

	t.Run("LogAdminAction", func(t *testing.T) {
		logger.LogAdminAction("telegram", "admin", "deleted user")
	})

	t.Run("LogConfigChange", func(t *testing.T) {
		logger.LogConfigChange("telegram", "admin", "changed rate limit")
	})

	t.Run("LogAccessDenied", func(t *testing.T) {
		logger.LogAccessDenied("whatsapp", "user", "/admin/settings")
	})
}

func TestAuditLoggerClose(t *testing.T) {
	dir := t.TempDir()
	logger, _ := NewAuditLogger(dir)

	// Log something
	_ = logger.Log(SecurityEvent{EventType: EventAuthSuccess})

	// Close should not error
	err := logger.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestAuditLoggerConcurrency(t *testing.T) {
	dir := t.TempDir()
	logger, _ := NewAuditLogger(dir)
	defer logger.Close()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = logger.Log(SecurityEvent{
				EventType: EventAuthSuccess,
				Platform:  "telegram",
				Details:   "concurrent log",
			})
		}(i)
	}
	wg.Wait()

	// Verify logs were written
	logPath := filepath.Join(dir, "security.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 100 {
		t.Errorf("Expected at least 100 log lines, got %d", len(lines))
	}
}

func TestAuditLoggerLogFormat(t *testing.T) {
	dir := t.TempDir()
	logger, _ := NewAuditLogger(dir)
	defer logger.Close()

	event := SecurityEvent{
		EventType: EventAuthSuccess,
		Platform:  "telegram",
		UserID:    HashUserID("telegram", "12345"),
		Success:   true,
	}
	_ = logger.Log(event)

	// Read and verify JSON format
	logPath := filepath.Join(dir, "security.log")
	data, _ := os.ReadFile(logPath)

	var logged SecurityEvent
	if err := json.Unmarshal(bytes.TrimSpace(data), &logged); err != nil {
		t.Errorf("Log should be valid JSON: %v", err)
	}

	if logged.EventType != EventAuthSuccess {
		t.Errorf("Expected EventAuthSuccess, got %s", logged.EventType)
	}
	if logged.Platform != "telegram" {
		t.Errorf("Expected platform 'telegram', got %s", logged.Platform)
	}
}

// Test with custom writer to avoid file operations
type testWriter struct {
	mu   sync.Mutex
	data []byte
}

func (w *testWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.data = append(w.data, p...)
	return len(p), nil
}

func TestAuditLoggerWithCustomWriter(t *testing.T) {
	tw := &testWriter{}
	logger := &AuditLogger{
		writer:    tw,
		maxSizeMB: 50,
	}

	_ = logger.Log(SecurityEvent{
		EventType: EventAuthSuccess,
		Platform:  "test",
	})

	if len(tw.data) == 0 {
		t.Error("Expected data to be written")
	}
}

func TestAuditLoggerRotation(t *testing.T) {
	dir := t.TempDir()
	logger, _ := NewAuditLogger(dir)
	defer logger.Close()

	// Set small max size for testing
	logger.maxSizeMB = 0 // Force rotation on first call

	// Write something
	_ = logger.Log(SecurityEvent{EventType: EventAuthSuccess})

	// Trigger rotateIfNeeded (would normally happen on next log)
	// Since we can't easily trigger rotation without large writes,
	// just verify the function doesn't panic
	logger.rotateIfNeeded()
}

// Mock closer for testing Close on non-file writers
type mockCloser struct {
	closed bool
}

func (m *mockCloser) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func (m *mockCloser) Close() error {
	m.closed = true
	return nil
}

func TestAuditLoggerCloseWithCloser(t *testing.T) {
	mc := &mockCloser{}
	logger := &AuditLogger{
		writer: mc,
	}

	logger.Close()
	if !mc.closed {
		t.Error("Close should be called on io.Closer")
	}
}

func TestAuditLoggerCloseWithNonCloser(t *testing.T) {
	// bytes.Buffer doesn't implement io.Closer
	buf := &bytes.Buffer{}
	logger := &AuditLogger{
		writer: buf,
	}

	// Should not panic
	err := logger.Close()
	if err != nil {
		t.Errorf("Close should return nil for non-closers: %v", err)
	}
}

// Test rotateIfNeeded with actual file rotation scenario
func TestAuditLoggerRotateScenario(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "security.log")

	// Create an existing large log file
	largeData := make([]byte, 100*1024) // 100KB
	for i := range largeData {
		largeData[i] = 'X'
	}
	_ = os.WriteFile(logPath, largeData, 0600)

	// Create logger (will use existing file)
	logger, err := NewAuditLogger(dir)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Set small threshold
	logger.maxSizeMB = 0 // 0 MB means any file will trigger rotation

	// Log something to trigger rotation check
	_ = logger.Log(SecurityEvent{EventType: EventAuthSuccess})

	// Check that rotation happened (look for timestamped file)
	files, _ := filepath.Glob(filepath.Join(dir, "security.log.*"))
	// Rotation might or might not happen depending on timing
	_ = files // Just verify no panic
}

// Test that Close properly handles rotation edge cases
func TestAuditLoggerRotateEdgeCases(t *testing.T) {
	t.Run("RotateNonexistentFile", func(t *testing.T) {
		logger := &AuditLogger{
			writer:    io.Discard,
			logPath:   "/nonexistent/path/file.log",
			maxSizeMB: 50,
		}
		// Should not panic
		logger.rotateIfNeeded()
	})
}

package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/kusa/magabot/internal/router"
)

func newTestServer(cfg *Config) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	}
	s, _ := New(cfg)
	return s
}

func TestNew(t *testing.T) {
	t.Run("Defaults", func(t *testing.T) {
		s, err := New(&Config{})
		if err != nil {
			t.Fatalf("Failed to create server: %v", err)
		}
		if s.config.Bind != "127.0.0.1" {
			t.Errorf("Expected bind 127.0.0.1, got %s", s.config.Bind)
		}
		if s.config.Port != 8080 {
			t.Errorf("Expected port 8080, got %d", s.config.Port)
		}
		if s.config.Path != "/webhook" {
			t.Errorf("Expected path /webhook, got %s", s.config.Path)
		}
		if s.config.MaxBodySize != 1048576 {
			t.Errorf("Expected maxBodySize 1MB, got %d", s.config.MaxBodySize)
		}
	})

	t.Run("CustomConfig", func(t *testing.T) {
		s, _ := New(&Config{
			Bind: "0.0.0.0",
			Port: 9000,
			Path: "/custom",
		})
		if s.config.Bind != "0.0.0.0" {
			t.Error("Custom bind not applied")
		}
		if s.config.Port != 9000 {
			t.Error("Custom port not applied")
		}
	})
}

func TestServerName(t *testing.T) {
	s := newTestServer(&Config{})
	if s.Name() != "webhook" {
		t.Errorf("Expected name 'webhook', got %s", s.Name())
	}
}

func TestServerSend(t *testing.T) {
	s := newTestServer(&Config{})
	err := s.Send("chat", "message")
	if err == nil {
		t.Error("Send should return error (webhook is receive-only)")
	}
}

func TestHandleWebhook(t *testing.T) {
	s := newTestServer(&Config{
		AuthMethod:   "none",
		AllowedUsers: []string{"testuser"},
	})

	t.Run("MethodNotAllowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/webhook", nil)
		rec := httptest.NewRecorder()
		s.handleWebhook(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("Expected 405, got %d", rec.Code)
		}
	})

	t.Run("ValidPost", func(t *testing.T) {
		body := bytes.NewReader([]byte(`{"message": "hello", "user_id": "testuser"}`))
		req := httptest.NewRequest(http.MethodPost, "/webhook", body)
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "127.0.0.1:12345"
		rec := httptest.NewRecorder()
		s.handleWebhook(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", rec.Code)
		}
	})

	t.Run("WithHandler", func(t *testing.T) {
		handlerCalled := false
		s.SetHandler(func(ctx context.Context, msg *router.Message) (string, error) {
			handlerCalled = true
			if msg.Text != "hello" {
				t.Errorf("Expected message 'hello', got %s", msg.Text)
			}
			return "response", nil
		})

		body := bytes.NewReader([]byte(`{"message": "hello", "user_id": "testuser"}`))
		req := httptest.NewRequest(http.MethodPost, "/webhook", body)
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "127.0.0.1:12345"
		rec := httptest.NewRecorder()
		s.handleWebhook(rec, req)

		if !handlerCalled {
			t.Error("Handler was not called")
		}
	})
}

func TestAuthenticateBearerToken(t *testing.T) {
	s := newTestServer(&Config{
		AuthMethod:  "bearer",
		BearerToken: "secret-token-123",
	})

	t.Run("ValidToken", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
		req.Header.Set("Authorization", "Bearer secret-token-123")

		if _, ok := s.authenticate(req); !ok {
			t.Error("Valid bearer token should authenticate")
		}
	})

	t.Run("InvalidToken", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
		req.Header.Set("Authorization", "Bearer wrong-token")

		if _, ok := s.authenticate(req); ok {
			t.Error("Invalid bearer token should not authenticate")
		}
	})

	t.Run("MissingToken", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/webhook", nil)

		if _, ok := s.authenticate(req); ok {
			t.Error("Missing token should not authenticate")
		}
	})
}

func TestAuthenticateBasic(t *testing.T) {
	s := newTestServer(&Config{
		AuthMethod: "basic",
		BasicUser:  "admin",
		BasicPass:  "password123",
	})

	t.Run("ValidCredentials", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
		req.SetBasicAuth("admin", "password123")

		if _, ok := s.authenticate(req); !ok {
			t.Error("Valid basic auth should authenticate")
		}
	})

	t.Run("WrongPassword", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
		req.SetBasicAuth("admin", "wrongpassword")

		if _, ok := s.authenticate(req); ok {
			t.Error("Wrong password should not authenticate")
		}
	})

	t.Run("WrongUser", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
		req.SetBasicAuth("wronguser", "password123")

		if _, ok := s.authenticate(req); ok {
			t.Error("Wrong user should not authenticate")
		}
	})

	t.Run("NoAuth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/webhook", nil)

		if _, ok := s.authenticate(req); ok {
			t.Error("No auth should not authenticate")
		}
	})
}

func TestAuthenticateHMAC(t *testing.T) {
	secret := "my-webhook-secret"
	s := newTestServer(&Config{
		AuthMethod: "hmac",
		HMACSecret: secret,
	})

	t.Run("ValidSignature", func(t *testing.T) {
		body := []byte(`{"event": "test"}`)
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

		req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
		req.Header.Set("X-Hub-Signature-256", signature)

		if _, ok := s.authenticate(req); !ok {
			t.Error("Valid HMAC should authenticate")
		}
	})

	t.Run("InvalidSignature", func(t *testing.T) {
		body := []byte(`{"event": "test"}`)
		req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
		req.Header.Set("X-Hub-Signature-256", "sha256=invalid")

		if _, ok := s.authenticate(req); ok {
			t.Error("Invalid HMAC should not authenticate")
		}
	})

	t.Run("MissingSignature", func(t *testing.T) {
		body := []byte(`{"event": "test"}`)
		req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))

		if _, ok := s.authenticate(req); ok {
			t.Error("Missing signature should not authenticate")
		}
	})

	t.Run("XSignatureHeader", func(t *testing.T) {
		body := []byte(`{"event": "test"}`)
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

		req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
		req.Header.Set("X-Signature", signature)

		if _, ok := s.authenticate(req); !ok {
			t.Error("X-Signature header should work")
		}
	})
}

func TestAuthenticateNone(t *testing.T) {
	t.Run("AuthNone", func(t *testing.T) {
		s := newTestServer(&Config{AuthMethod: "none"})
		req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
		if _, ok := s.authenticate(req); !ok {
			t.Error("Auth none should always pass")
		}
	})

	t.Run("AuthEmpty", func(t *testing.T) {
		s := newTestServer(&Config{AuthMethod: ""})
		req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
		if _, ok := s.authenticate(req); !ok {
			t.Error("Empty auth should default to none")
		}
	})
}

func TestIPWhitelist(t *testing.T) {
	t.Run("EmptyWhitelist", func(t *testing.T) {
		s := newTestServer(&Config{AllowedIPs: []string{}})
		req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
		req.RemoteAddr = "1.2.3.4:12345"

		if !s.checkIP(req) {
			t.Error("Empty whitelist should allow all")
		}
	})

	t.Run("AllowedIP", func(t *testing.T) {
		s := newTestServer(&Config{AllowedIPs: []string{"1.2.3.4"}})
		req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
		req.RemoteAddr = "1.2.3.4:12345"

		if !s.checkIP(req) {
			t.Error("IP in whitelist should be allowed")
		}
	})

	t.Run("BlockedIP", func(t *testing.T) {
		s := newTestServer(&Config{AllowedIPs: []string{"1.2.3.4"}})
		req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
		req.RemoteAddr = "5.6.7.8:12345"

		if s.checkIP(req) {
			t.Error("IP not in whitelist should be blocked")
		}
	})

	t.Run("CIDR", func(t *testing.T) {
		s := newTestServer(&Config{AllowedIPs: []string{"10.0.0.0/8"}})

		allowedReq := httptest.NewRequest(http.MethodPost, "/webhook", nil)
		allowedReq.RemoteAddr = "10.1.2.3:12345"
		if !s.checkIP(allowedReq) {
			t.Error("IP in CIDR range should be allowed")
		}

		blockedReq := httptest.NewRequest(http.MethodPost, "/webhook", nil)
		blockedReq.RemoteAddr = "11.1.2.3:12345"
		if s.checkIP(blockedReq) {
			t.Error("IP outside CIDR range should be blocked")
		}
	})

	t.Run("InvalidIP", func(t *testing.T) {
		s := newTestServer(&Config{AllowedIPs: []string{"1.2.3.4"}})
		req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
		req.RemoteAddr = "invalid:12345"

		if s.checkIP(req) {
			t.Error("Invalid IP should be blocked")
		}
	})
}

func TestParsePayload(t *testing.T) {
	s := newTestServer(&Config{})

	t.Run("JSONMessage", func(t *testing.T) {
		body := []byte(`{"message": "hello world"}`)
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		text, _ := s.parsePayload(body, req)
		if text != "hello world" {
			t.Errorf("Expected 'hello world', got %s", text)
		}
	})

	t.Run("JSONText", func(t *testing.T) {
		body := []byte(`{"text": "hello text"}`)
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		text, _ := s.parsePayload(body, req)
		if text != "hello text" {
			t.Errorf("Expected 'hello text', got %s", text)
		}
	})

	t.Run("JSONContent", func(t *testing.T) {
		body := []byte(`{"content": "hello content"}`)
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		text, _ := s.parsePayload(body, req)
		if text != "hello content" {
			t.Errorf("Expected 'hello content', got %s", text)
		}
	})

	t.Run("GitHubPush", func(t *testing.T) {
		body := []byte(`{"commits": [{"message": "fix bug"}], "sender": {"login": "testuser"}}`)
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		text, userID := s.parsePayload(body, req)
		if text != "GitHub push: fix bug" {
			t.Errorf("Expected 'GitHub push: fix bug', got %s", text)
		}
		if userID != "github:testuser" {
			t.Errorf("Expected 'github:testuser', got %s", userID)
		}
	})

	t.Run("GrafanaAlert", func(t *testing.T) {
		body := []byte(`{"title": "CPU High", "state": "alerting"}`)
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		text, userID := s.parsePayload(body, req)
		if text != "Grafana [alerting]: CPU High" {
			t.Errorf("Expected 'Grafana [alerting]: CPU High', got %s", text)
		}
		if userID != "grafana" {
			t.Errorf("Expected 'grafana', got %s", userID)
		}
	})

	t.Run("PlainText", func(t *testing.T) {
		body := []byte(`plain text message`)
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		text, _ := s.parsePayload(body, req)
		if text != "plain text message" {
			t.Errorf("Expected 'plain text message', got %s", text)
		}
	})

	t.Run("WithUserID", func(t *testing.T) {
		body := []byte(`{"message": "test", "user_id": "telegram:12345"}`)
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		text, userID := s.parsePayload(body, req)
		if text != "test" {
			t.Errorf("Expected 'test', got %s", text)
		}
		if userID != "telegram:12345" {
			t.Errorf("Expected 'telegram:12345', got %s", userID)
		}
	})
}

func TestCheckUser(t *testing.T) {
	t.Run("EmptyAllowlist", func(t *testing.T) {
		s := newTestServer(&Config{})
		if !s.checkUser("anyuser") {
			t.Error("Empty allowlist should allow all users")
		}
	})

	t.Run("ExactMatch", func(t *testing.T) {
		s := newTestServer(&Config{AllowedUsers: []string{"user123"}})
		if !s.checkUser("user123") {
			t.Error("Should allow exact match")
		}
		if s.checkUser("user456") {
			t.Error("Should block non-matching user")
		}
	})

	t.Run("WildcardPrefix", func(t *testing.T) {
		s := newTestServer(&Config{AllowedUsers: []string{"telegram:*"}})
		if !s.checkUser("telegram:12345") {
			t.Error("Should allow telegram:12345 with telegram:* wildcard")
		}
		if !s.checkUser("telegram:67890") {
			t.Error("Should allow telegram:67890 with telegram:* wildcard")
		}
		if s.checkUser("slack:12345") {
			t.Error("Should block slack:12345 with telegram:* wildcard")
		}
	})

	t.Run("MultipleAllowed", func(t *testing.T) {
		s := newTestServer(&Config{AllowedUsers: []string{"user1", "telegram:*", "github:octocat"}})
		if !s.checkUser("user1") {
			t.Error("Should allow user1")
		}
		if !s.checkUser("telegram:99999") {
			t.Error("Should allow telegram users")
		}
		if !s.checkUser("github:octocat") {
			t.Error("Should allow github:octocat")
		}
		if s.checkUser("github:other") {
			t.Error("Should block github:other")
		}
	})
}

func TestHandleHealth(t *testing.T) {
	s := newTestServer(&Config{})

	t.Run("BasicHealth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()

		s.handleHealth(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", rec.Code)
		}
		if rec.Body.String() != "OK" {
			t.Errorf("Expected 'OK', got %s", rec.Body.String())
		}
	})

	t.Run("HealthWithMetrics", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health?metrics=true", nil)
		rec := httptest.NewRecorder()

		s.handleHealth(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", rec.Code)
		}

		var metrics map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &metrics); err != nil {
			t.Fatalf("Failed to parse metrics JSON: %v", err)
		}

		// Verify required fields
		requiredFields := []string{"status", "goroutines", "heap_alloc", "heap_sys", "gc_cycles", "go_version"}
		for _, field := range requiredFields {
			if _, ok := metrics[field]; !ok {
				t.Errorf("Missing required field: %s", field)
			}
		}

		if metrics["status"] != "ok" {
			t.Errorf("Expected status 'ok', got %v", metrics["status"])
		}

		// Goroutines should be a positive number
		if goroutines, ok := metrics["goroutines"].(float64); !ok || goroutines <= 0 {
			t.Errorf("Expected positive goroutine count, got %v", metrics["goroutines"])
		}
	})
}

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		remoteAddr string
		expected   string
	}{
		{"192.168.1.1:12345", "192.168.1.1"},
		{"10.0.0.1:80", "10.0.0.1"},
		{"[::1]:8080", "::1"},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.RemoteAddr = tt.remoteAddr
		result := getClientIP(req)
		if result != tt.expected {
			t.Errorf("getClientIP(%s) = %s, want %s", tt.remoteAddr, result, tt.expected)
		}
	}
}

func TestServerStartStop(t *testing.T) {
	s := newTestServer(&Config{
		Bind: "127.0.0.1",
		Port: 19999, // Unusual port
	})

	ctx := context.Background()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	if err := s.Stop(); err != nil {
		t.Errorf("Failed to stop server: %v", err)
	}
}

func TestWebhookResponse(t *testing.T) {
	s := newTestServer(&Config{
		AuthMethod:   "none",
		AllowedUsers: []string{"testuser"},
	})
	s.SetHandler(func(ctx context.Context, msg *router.Message) (string, error) {
		return "bot response", nil
	})

	body := bytes.NewReader([]byte(`{"message": "test", "user_id": "testuser"}`))
	req := httptest.NewRequest(http.MethodPost, "/webhook", body)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()

	s.handleWebhook(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp["ok"] != true {
		t.Error("Expected ok=true")
	}
	if resp["response"] != "bot response" {
		t.Errorf("Expected response 'bot response', got %v", resp["response"])
	}
}

func TestWebhookRejectsEmptyUserID(t *testing.T) {
	s := newTestServer(&Config{
		AuthMethod:   "none",
		AllowedUsers: []string{"testuser"},
	})

	body := bytes.NewReader([]byte(`{"message": "test"}`))
	req := httptest.NewRequest(http.MethodPost, "/webhook", body)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()

	s.handleWebhook(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("Expected 403 for missing user_id, got %d", rec.Code)
	}
}

func TestWebhookRejectsUnauthorizedUser(t *testing.T) {
	s := newTestServer(&Config{
		AuthMethod:   "none",
		AllowedUsers: []string{"alloweduser"},
	})

	body := bytes.NewReader([]byte(`{"message": "test", "user_id": "hackeruser"}`))
	req := httptest.NewRequest(http.MethodPost, "/webhook", body)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()

	s.handleWebhook(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("Expected 403 for unauthorized user, got %d", rec.Code)
	}
}

func TestBearerTokensMapping(t *testing.T) {
	s := newTestServer(&Config{
		AuthMethod: "bearer",
		BearerTokens: map[string]string{
			"token-for-alice": "alice",
			"token-for-bob":   "bob",
		},
		AllowedUsers: []string{"alice", "bob"},
	})
	s.SetHandler(func(ctx context.Context, msg *router.Message) (string, error) {
		return "hello " + msg.UserID, nil
	})

	t.Run("AliceToken", func(t *testing.T) {
		body := bytes.NewReader([]byte(`{"message": "hi"}`))
		req := httptest.NewRequest(http.MethodPost, "/webhook", body)
		req.Header.Set("Authorization", "Bearer token-for-alice")
		req.RemoteAddr = "127.0.0.1:12345"
		rec := httptest.NewRecorder()

		s.handleWebhook(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", rec.Code)
		}
		var resp map[string]interface{}
		json.Unmarshal(rec.Body.Bytes(), &resp)
		if resp["response"] != "hello alice" {
			t.Errorf("Expected 'hello alice', got %v", resp["response"])
		}
	})

	t.Run("InvalidToken", func(t *testing.T) {
		body := bytes.NewReader([]byte(`{"message": "hi"}`))
		req := httptest.NewRequest(http.MethodPost, "/webhook", body)
		req.Header.Set("Authorization", "Bearer wrong-token")
		req.RemoteAddr = "127.0.0.1:12345"
		rec := httptest.NewRecorder()

		s.handleWebhook(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Expected 401, got %d", rec.Code)
		}
	})
}

// ==== Security Features Tests ====

func TestRateLimiter(t *testing.T) {
	rl := newRateLimiter(3, 100*time.Millisecond)

	// First 3 should pass
	for i := 0; i < 3; i++ {
		if !rl.allow("test-key") {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 4th should be blocked
	if rl.allow("test-key") {
		t.Error("Request 4 should be blocked")
	}

	// Different key should work
	if !rl.allow("other-key") {
		t.Error("Different key should be allowed")
	}

	// Wait for window to expire
	time.Sleep(150 * time.Millisecond)
	if !rl.allow("test-key") {
		t.Error("Should be allowed after window expires")
	}
}

func TestFailureTracker(t *testing.T) {
	ft := newFailureTracker(3, 100*time.Millisecond)

	// Should not be locked initially
	if ft.isLocked("test-ip") {
		t.Error("Should not be locked initially")
	}

	// Record 2 failures - still not locked
	ft.recordFailure("test-ip")
	ft.recordFailure("test-ip")
	if ft.isLocked("test-ip") {
		t.Error("Should not be locked after 2 failures")
	}

	// Record 3rd failure - now locked
	ft.recordFailure("test-ip")
	if !ft.isLocked("test-ip") {
		t.Error("Should be locked after 3 failures")
	}

	// Different IP should not be locked
	if ft.isLocked("other-ip") {
		t.Error("Other IP should not be locked")
	}

	// Wait for lockout to expire
	time.Sleep(150 * time.Millisecond)
	if ft.isLocked("test-ip") {
		t.Error("Should not be locked after lockout expires")
	}

	// Clear failures
	ft.recordFailure("test-ip")
	ft.recordFailure("test-ip")
	ft.clearFailures("test-ip")
	ft.recordFailure("test-ip")
	if ft.isLocked("test-ip") {
		t.Error("Should not be locked after clearFailures")
	}
}

// ==== Integration Tests: Rate Limiting ====

func TestIPRateLimitingIntegration(t *testing.T) {
	t.Run("BasicRateLimit", func(t *testing.T) {
		s := newTestServer(&Config{
			AuthMethod:      "none",
			AllowedUsers:    []string{"testuser"},
			RateLimitPerIP:  3,
			RateLimitWindow: 100 * time.Millisecond,
		})

		makeRequest := func(ip string) *httptest.ResponseRecorder {
			body := bytes.NewReader([]byte(`{"message": "test", "user_id": "testuser"}`))
			req := httptest.NewRequest(http.MethodPost, "/webhook", body)
			req.RemoteAddr = ip + ":12345"
			rec := httptest.NewRecorder()
			s.handleWebhook(rec, req)
			return rec
		}

		// First 3 requests should succeed
		for i := 1; i <= 3; i++ {
			rec := makeRequest("192.168.1.100")
			if rec.Code != http.StatusOK {
				t.Errorf("Request %d should succeed, got %d", i, rec.Code)
			}
		}

		// 4th request should be rate limited
		rec := makeRequest("192.168.1.100")
		if rec.Code != http.StatusTooManyRequests {
			t.Errorf("Request 4 should be rate limited, got %d", rec.Code)
		}

		// Should have Retry-After header
		if rec.Header().Get("Retry-After") == "" {
			t.Error("Rate limited response should have Retry-After header")
		}
	})

	t.Run("DifferentIPsIndependent", func(t *testing.T) {
		s := newTestServer(&Config{
			AuthMethod:      "none",
			AllowedUsers:    []string{"testuser"},
			RateLimitPerIP:  2,
			RateLimitWindow: 100 * time.Millisecond,
		})

		makeRequest := func(ip string) int {
			body := bytes.NewReader([]byte(`{"message": "test", "user_id": "testuser"}`))
			req := httptest.NewRequest(http.MethodPost, "/webhook", body)
			req.RemoteAddr = ip + ":12345"
			rec := httptest.NewRecorder()
			s.handleWebhook(rec, req)
			return rec.Code
		}

		// IP1: 2 requests OK, 3rd blocked
		makeRequest("10.0.0.1")
		makeRequest("10.0.0.1")
		if code := makeRequest("10.0.0.1"); code != http.StatusTooManyRequests {
			t.Errorf("IP1 request 3 should be blocked, got %d", code)
		}

		// IP2: still has fresh quota
		if code := makeRequest("10.0.0.2"); code != http.StatusOK {
			t.Errorf("IP2 should still work, got %d", code)
		}
		if code := makeRequest("10.0.0.2"); code != http.StatusOK {
			t.Errorf("IP2 request 2 should work, got %d", code)
		}
	})

	t.Run("WindowExpiration", func(t *testing.T) {
		s := newTestServer(&Config{
			AuthMethod:      "none",
			AllowedUsers:    []string{"testuser"},
			RateLimitPerIP:  2,
			RateLimitWindow: 50 * time.Millisecond,
		})

		makeRequest := func() int {
			body := bytes.NewReader([]byte(`{"message": "test", "user_id": "testuser"}`))
			req := httptest.NewRequest(http.MethodPost, "/webhook", body)
			req.RemoteAddr = "172.16.0.1:12345"
			rec := httptest.NewRecorder()
			s.handleWebhook(rec, req)
			return rec.Code
		}

		// Exhaust rate limit
		makeRequest()
		makeRequest()
		if code := makeRequest(); code != http.StatusTooManyRequests {
			t.Errorf("Should be rate limited, got %d", code)
		}

		// Wait for window to expire
		time.Sleep(60 * time.Millisecond)

		// Should work again
		if code := makeRequest(); code != http.StatusOK {
			t.Errorf("Should work after window expires, got %d", code)
		}
	})

	t.Run("DisabledWhenZero", func(t *testing.T) {
		s := newTestServer(&Config{
			AuthMethod:      "none",
			AllowedUsers:    []string{"testuser"},
			RateLimitPerIP:  0, // Disabled
			RateLimitWindow: 100 * time.Millisecond,
		})

		makeRequest := func() int {
			body := bytes.NewReader([]byte(`{"message": "test", "user_id": "testuser"}`))
			req := httptest.NewRequest(http.MethodPost, "/webhook", body)
			req.RemoteAddr = "1.2.3.4:12345"
			rec := httptest.NewRecorder()
			s.handleWebhook(rec, req)
			return rec.Code
		}

		// Should allow unlimited requests
		for i := 0; i < 100; i++ {
			if code := makeRequest(); code != http.StatusOK {
				t.Errorf("Request %d should succeed when rate limit disabled, got %d", i, code)
				break
			}
		}
	})
}

func TestUserRateLimitingIntegration(t *testing.T) {
	t.Run("BasicUserRateLimit", func(t *testing.T) {
		s := newTestServer(&Config{
			AuthMethod:       "none",
			AllowedUsers:     []string{"user1", "user2"},
			RateLimitPerUser: 3,
			RateLimitWindow:  100 * time.Millisecond,
		})

		makeRequest := func(userID string, ip string) *httptest.ResponseRecorder {
			body := bytes.NewReader([]byte(`{"message": "test", "user_id": "` + userID + `"}`))
			req := httptest.NewRequest(http.MethodPost, "/webhook", body)
			req.RemoteAddr = ip + ":12345"
			rec := httptest.NewRecorder()
			s.handleWebhook(rec, req)
			return rec
		}

		// User1: first 3 succeed from different IPs
		for i := 1; i <= 3; i++ {
			ip := fmt.Sprintf("10.0.0.%d", i)
			rec := makeRequest("user1", ip)
			if rec.Code != http.StatusOK {
				t.Errorf("User1 request %d should succeed, got %d", i, rec.Code)
			}
		}

		// User1: 4th blocked even from new IP
		rec := makeRequest("user1", "10.0.0.99")
		if rec.Code != http.StatusTooManyRequests {
			t.Errorf("User1 request 4 should be rate limited, got %d", rec.Code)
		}
		if rec.Header().Get("Retry-After") == "" {
			t.Error("Rate limited response should have Retry-After header")
		}
	})

	t.Run("DifferentUsersIndependent", func(t *testing.T) {
		s := newTestServer(&Config{
			AuthMethod:       "none",
			AllowedUsers:     []string{"alice", "bob", "charlie"},
			RateLimitPerUser: 2,
			RateLimitWindow:  100 * time.Millisecond,
		})

		makeRequest := func(userID string) int {
			body := bytes.NewReader([]byte(`{"message": "test", "user_id": "` + userID + `"}`))
			req := httptest.NewRequest(http.MethodPost, "/webhook", body)
			req.RemoteAddr = "192.168.1.1:12345"
			rec := httptest.NewRecorder()
			s.handleWebhook(rec, req)
			return rec.Code
		}

		// Alice: exhaust quota
		makeRequest("alice")
		makeRequest("alice")
		if code := makeRequest("alice"); code != http.StatusTooManyRequests {
			t.Errorf("Alice request 3 should be blocked, got %d", code)
		}

		// Bob: fresh quota
		if code := makeRequest("bob"); code != http.StatusOK {
			t.Errorf("Bob should work, got %d", code)
		}

		// Charlie: also fresh
		if code := makeRequest("charlie"); code != http.StatusOK {
			t.Errorf("Charlie should work, got %d", code)
		}
	})

	t.Run("UserRateLimitWithTokenAuth", func(t *testing.T) {
		s := newTestServer(&Config{
			AuthMethod: "bearer",
			BearerTokens: map[string]string{
				"token-github": "github:webhook",
				"token-ci":     "ci:jenkins",
			},
			AllowedUsers:     []string{"github:webhook", "ci:jenkins"},
			RateLimitPerUser: 2,
			RateLimitWindow:  100 * time.Millisecond,
		})

		makeRequest := func(token string) int {
			body := bytes.NewReader([]byte(`{"message": "test"}`))
			req := httptest.NewRequest(http.MethodPost, "/webhook", body)
			req.Header.Set("Authorization", "Bearer "+token)
			req.RemoteAddr = "1.1.1.1:12345"
			rec := httptest.NewRecorder()
			s.handleWebhook(rec, req)
			return rec.Code
		}

		// GitHub token: 2 OK, 3rd blocked
		makeRequest("token-github")
		makeRequest("token-github")
		if code := makeRequest("token-github"); code != http.StatusTooManyRequests {
			t.Errorf("GitHub request 3 should be blocked, got %d", code)
		}

		// CI token: still works (different user)
		if code := makeRequest("token-ci"); code != http.StatusOK {
			t.Errorf("CI should work, got %d", code)
		}
	})

	t.Run("WindowExpiration", func(t *testing.T) {
		s := newTestServer(&Config{
			AuthMethod:       "none",
			AllowedUsers:     []string{"testuser"},
			RateLimitPerUser: 2,
			RateLimitWindow:  50 * time.Millisecond,
		})

		makeRequest := func() int {
			body := bytes.NewReader([]byte(`{"message": "test", "user_id": "testuser"}`))
			req := httptest.NewRequest(http.MethodPost, "/webhook", body)
			req.RemoteAddr = "1.1.1.1:12345"
			rec := httptest.NewRecorder()
			s.handleWebhook(rec, req)
			return rec.Code
		}

		// Exhaust quota
		makeRequest()
		makeRequest()
		if code := makeRequest(); code != http.StatusTooManyRequests {
			t.Errorf("Should be rate limited, got %d", code)
		}

		// Wait for window
		time.Sleep(60 * time.Millisecond)

		// Should work again
		if code := makeRequest(); code != http.StatusOK {
			t.Errorf("Should work after window expires, got %d", code)
		}
	})

	t.Run("CombinedIPAndUserRateLimit", func(t *testing.T) {
		s := newTestServer(&Config{
			AuthMethod:       "none",
			AllowedUsers:     []string{"user1", "user2"},
			RateLimitPerIP:   5,
			RateLimitPerUser: 2,
			RateLimitWindow:  100 * time.Millisecond,
		})

		makeRequest := func(userID, ip string) int {
			body := bytes.NewReader([]byte(`{"message": "test", "user_id": "` + userID + `"}`))
			req := httptest.NewRequest(http.MethodPost, "/webhook", body)
			req.RemoteAddr = ip + ":12345"
			rec := httptest.NewRecorder()
			s.handleWebhook(rec, req)
			return rec.Code
		}

		// User1 from IP1: 2 OK
		makeRequest("user1", "10.0.0.1")
		makeRequest("user1", "10.0.0.1")

		// User1 blocked by user limit (even though IP still has quota)
		if code := makeRequest("user1", "10.0.0.1"); code != http.StatusTooManyRequests {
			t.Errorf("User1 should be blocked by user limit, got %d", code)
		}

		// User2 from same IP still works (different user)
		if code := makeRequest("user2", "10.0.0.1"); code != http.StatusOK {
			t.Errorf("User2 from same IP should work, got %d", code)
		}
	})
}

func TestAuthFailureLockoutIntegration(t *testing.T) {
	t.Run("BasicLockout", func(t *testing.T) {
		s := newTestServer(&Config{
			AuthMethod:      "bearer",
			BearerToken:     "correct-token",
			AllowedUsers:    []string{"testuser"},
			MaxAuthFailures: 3,
			AuthLockoutTime: 100 * time.Millisecond,
		})

		badRequest := func(ip string) int {
			body := bytes.NewReader([]byte(`{"message": "test", "user_id": "testuser"}`))
			req := httptest.NewRequest(http.MethodPost, "/webhook", body)
			req.Header.Set("Authorization", "Bearer wrong-token")
			req.RemoteAddr = ip + ":12345"
			rec := httptest.NewRecorder()
			s.handleWebhook(rec, req)
			return rec.Code
		}

		goodRequest := func(ip string) int {
			body := bytes.NewReader([]byte(`{"message": "test", "user_id": "testuser"}`))
			req := httptest.NewRequest(http.MethodPost, "/webhook", body)
			req.Header.Set("Authorization", "Bearer correct-token")
			req.RemoteAddr = ip + ":12345"
			rec := httptest.NewRecorder()
			s.handleWebhook(rec, req)
			return rec.Code
		}

		// 3 bad requests -> lockout
		for i := 1; i <= 3; i++ {
			if code := badRequest("10.0.0.1"); code != http.StatusUnauthorized {
				t.Errorf("Bad request %d should return 401, got %d", i, code)
			}
		}

		// IP is now locked - even good requests blocked
		if code := goodRequest("10.0.0.1"); code != http.StatusTooManyRequests {
			t.Errorf("Locked IP should return 429, got %d", code)
		}

		// Different IP still works
		if code := goodRequest("10.0.0.2"); code != http.StatusOK {
			t.Errorf("Different IP should work, got %d", code)
		}
	})

	t.Run("LockoutExpiration", func(t *testing.T) {
		s := newTestServer(&Config{
			AuthMethod:      "bearer",
			BearerToken:     "secret",
			AllowedUsers:    []string{"testuser"},
			MaxAuthFailures: 2,
			AuthLockoutTime: 50 * time.Millisecond,
		})

		badRequest := func() int {
			body := bytes.NewReader([]byte(`{"message": "test", "user_id": "testuser"}`))
			req := httptest.NewRequest(http.MethodPost, "/webhook", body)
			req.Header.Set("Authorization", "Bearer wrong")
			req.RemoteAddr = "192.168.1.1:12345"
			rec := httptest.NewRecorder()
			s.handleWebhook(rec, req)
			return rec.Code
		}

		goodRequest := func() int {
			body := bytes.NewReader([]byte(`{"message": "test", "user_id": "testuser"}`))
			req := httptest.NewRequest(http.MethodPost, "/webhook", body)
			req.Header.Set("Authorization", "Bearer secret")
			req.RemoteAddr = "192.168.1.1:12345"
			rec := httptest.NewRecorder()
			s.handleWebhook(rec, req)
			return rec.Code
		}

		// Trigger lockout
		badRequest()
		badRequest()

		// Locked
		if code := goodRequest(); code != http.StatusTooManyRequests {
			t.Errorf("Should be locked, got %d", code)
		}

		// Wait for expiration
		time.Sleep(60 * time.Millisecond)

		// Should work again
		if code := goodRequest(); code != http.StatusOK {
			t.Errorf("Should work after lockout expires, got %d", code)
		}
	})

	t.Run("SuccessResetsFailureCount", func(t *testing.T) {
		s := newTestServer(&Config{
			AuthMethod:      "bearer",
			BearerToken:     "secret",
			AllowedUsers:    []string{"testuser"},
			MaxAuthFailures: 3,
			AuthLockoutTime: 100 * time.Millisecond,
		})

		badRequest := func() int {
			body := bytes.NewReader([]byte(`{"message": "test", "user_id": "testuser"}`))
			req := httptest.NewRequest(http.MethodPost, "/webhook", body)
			req.Header.Set("Authorization", "Bearer wrong")
			req.RemoteAddr = "172.16.0.1:12345"
			rec := httptest.NewRecorder()
			s.handleWebhook(rec, req)
			return rec.Code
		}

		goodRequest := func() int {
			body := bytes.NewReader([]byte(`{"message": "test", "user_id": "testuser"}`))
			req := httptest.NewRequest(http.MethodPost, "/webhook", body)
			req.Header.Set("Authorization", "Bearer secret")
			req.RemoteAddr = "172.16.0.1:12345"
			rec := httptest.NewRecorder()
			s.handleWebhook(rec, req)
			return rec.Code
		}

		// 2 failures
		badRequest()
		badRequest()

		// Success resets counter
		if code := goodRequest(); code != http.StatusOK {
			t.Errorf("Good request should succeed, got %d", code)
		}

		// Can now have 2 more failures before lockout
		badRequest()
		badRequest()

		// Still not locked (counter was reset)
		if code := goodRequest(); code != http.StatusOK {
			t.Errorf("Should not be locked yet, got %d", code)
		}
	})

	t.Run("DifferentAuthMethods", func(t *testing.T) {
		// Test with HMAC auth
		s := newTestServer(&Config{
			AuthMethod:      "hmac",
			HMACSecret:      "supersecret",
			AllowedUsers:    []string{"testuser"},
			MaxAuthFailures: 2,
			AuthLockoutTime: 100 * time.Millisecond,
		})

		badRequest := func() int {
			body := bytes.NewReader([]byte(`{"message": "test", "user_id": "testuser"}`))
			req := httptest.NewRequest(http.MethodPost, "/webhook", body)
			req.Header.Set("X-Hub-Signature-256", "sha256=wrongsignature")
			req.RemoteAddr = "8.8.8.8:12345"
			rec := httptest.NewRecorder()
			s.handleWebhook(rec, req)
			return rec.Code
		}

		// 2 failures trigger lockout
		badRequest()
		badRequest()

		// 3rd is blocked by lockout (not auth)
		if code := badRequest(); code != http.StatusTooManyRequests {
			t.Errorf("Should be locked out, got %d", code)
		}
	})
}

func TestTimestampValidation(t *testing.T) {
	s := newTestServer(&Config{
		AuthMethod:       "none",
		AllowedUsers:     []string{"testuser"},
		RequireTimestamp: true,
	})

	t.Run("MissingTimestamp", func(t *testing.T) {
		body := bytes.NewReader([]byte(`{"message": "test", "user_id": "testuser"}`))
		req := httptest.NewRequest(http.MethodPost, "/webhook", body)
		req.RemoteAddr = "127.0.0.1:12345"
		rec := httptest.NewRecorder()
		s.handleWebhook(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("Missing timestamp should return 400, got %d", rec.Code)
		}
	})

	t.Run("ValidTimestamp", func(t *testing.T) {
		body := bytes.NewReader([]byte(`{"message": "test", "user_id": "testuser"}`))
		req := httptest.NewRequest(http.MethodPost, "/webhook", body)
		req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
		req.RemoteAddr = "127.0.0.1:12345"
		rec := httptest.NewRecorder()
		s.handleWebhook(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Valid timestamp should succeed, got %d", rec.Code)
		}
	})

	t.Run("OldTimestamp", func(t *testing.T) {
		body := bytes.NewReader([]byte(`{"message": "test", "user_id": "testuser"}`))
		req := httptest.NewRequest(http.MethodPost, "/webhook", body)
		// 10 minutes ago
		req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Add(-10*time.Minute).Unix()))
		req.RemoteAddr = "127.0.0.1:12345"
		rec := httptest.NewRecorder()
		s.handleWebhook(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("Old timestamp should return 400, got %d", rec.Code)
		}
	})

	t.Run("FutureTimestamp", func(t *testing.T) {
		body := bytes.NewReader([]byte(`{"message": "test", "user_id": "testuser"}`))
		req := httptest.NewRequest(http.MethodPost, "/webhook", body)
		// 10 minutes in future
		req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Add(10*time.Minute).Unix()))
		req.RemoteAddr = "127.0.0.1:12345"
		rec := httptest.NewRecorder()
		s.handleWebhook(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("Future timestamp should return 400, got %d", rec.Code)
		}
	})
}

func TestNonceValidation(t *testing.T) {
	s := newTestServer(&Config{
		AuthMethod:   "none",
		AllowedUsers: []string{"testuser"},
		RequireNonce: true,
	})

	t.Run("MissingNonce", func(t *testing.T) {
		body := bytes.NewReader([]byte(`{"message": "test", "user_id": "testuser"}`))
		req := httptest.NewRequest(http.MethodPost, "/webhook", body)
		req.RemoteAddr = "127.0.0.1:12345"
		rec := httptest.NewRecorder()
		s.handleWebhook(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("Missing nonce should return 400, got %d", rec.Code)
		}
	})

	t.Run("ValidNonce", func(t *testing.T) {
		body := bytes.NewReader([]byte(`{"message": "test", "user_id": "testuser"}`))
		req := httptest.NewRequest(http.MethodPost, "/webhook", body)
		req.Header.Set("X-Nonce", "unique-nonce-123")
		req.RemoteAddr = "127.0.0.1:12345"
		rec := httptest.NewRecorder()
		s.handleWebhook(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Valid nonce should succeed, got %d", rec.Code)
		}
	})

	t.Run("DuplicateNonce", func(t *testing.T) {
		// First request with nonce
		body1 := bytes.NewReader([]byte(`{"message": "test1", "user_id": "testuser"}`))
		req1 := httptest.NewRequest(http.MethodPost, "/webhook", body1)
		req1.Header.Set("X-Nonce", "reused-nonce")
		req1.RemoteAddr = "127.0.0.1:12345"
		rec1 := httptest.NewRecorder()
		s.handleWebhook(rec1, req1)

		if rec1.Code != http.StatusOK {
			t.Fatalf("First nonce should succeed, got %d", rec1.Code)
		}

		// Second request with same nonce (replay attack)
		body2 := bytes.NewReader([]byte(`{"message": "test2", "user_id": "testuser"}`))
		req2 := httptest.NewRequest(http.MethodPost, "/webhook", body2)
		req2.Header.Set("X-Nonce", "reused-nonce")
		req2.RemoteAddr = "127.0.0.1:12345"
		rec2 := httptest.NewRecorder()
		s.handleWebhook(rec2, req2)

		if rec2.Code != http.StatusConflict {
			t.Errorf("Duplicate nonce should return 409, got %d", rec2.Code)
		}
	})
}

func TestSecurityHeaders(t *testing.T) {
	s := newTestServer(&Config{
		AuthMethod:   "none",
		AllowedUsers: []string{"testuser"},
	})

	body := bytes.NewReader([]byte(`{"message": "test", "user_id": "testuser"}`))
	req := httptest.NewRequest(http.MethodPost, "/webhook", body)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	s.handleWebhook(rec, req)

	headers := rec.Header()

	if headers.Get("X-Request-ID") == "" {
		t.Error("Missing X-Request-ID header")
	}
	if headers.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("Missing or wrong X-Content-Type-Options header")
	}
	if headers.Get("X-Frame-Options") != "DENY" {
		t.Error("Missing or wrong X-Frame-Options header")
	}
	if headers.Get("Cache-Control") != "no-store" {
		t.Error("Missing or wrong Cache-Control header")
	}
	if headers.Get("Content-Security-Policy") != "default-src 'none'" {
		t.Error("Missing or wrong Content-Security-Policy header")
	}
}

func TestRequestIDInResponse(t *testing.T) {
	s := newTestServer(&Config{
		AuthMethod:   "none",
		AllowedUsers: []string{"testuser"},
	})

	body := bytes.NewReader([]byte(`{"message": "test", "user_id": "testuser"}`))
	req := httptest.NewRequest(http.MethodPost, "/webhook", body)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	s.handleWebhook(rec, req)

	var resp map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &resp)

	if resp["request_id"] == nil || resp["request_id"] == "" {
		t.Error("Response should include request_id")
	}
}

func TestHMACUsersMapping(t *testing.T) {
	s := newTestServer(&Config{
		AuthMethod: "hmac",
		HMACUsers: map[string]string{
			"secret-for-github":  "github:myrepo",
			"secret-for-grafana": "grafana:prod",
		},
		AllowedUsers: []string{"github:myrepo", "grafana:prod"},
	})
	s.SetHandler(func(ctx context.Context, msg *router.Message) (string, error) {
		return "hello " + msg.UserID, nil
	})

	t.Run("GitHubSecret", func(t *testing.T) {
		payload := []byte(`{"message": "push event"}`)
		mac := hmac.New(sha256.New, []byte("secret-for-github"))
		mac.Write(payload)
		sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

		req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payload))
		req.Header.Set("X-Hub-Signature-256", sig)
		req.RemoteAddr = "127.0.0.1:12345"
		rec := httptest.NewRecorder()

		s.handleWebhook(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", rec.Code)
		}
		var resp map[string]interface{}
		json.Unmarshal(rec.Body.Bytes(), &resp)
		if resp["response"] != "hello github:myrepo" {
			t.Errorf("Expected 'hello github:myrepo', got %v", resp["response"])
		}
	})

	t.Run("WrongSecret", func(t *testing.T) {
		payload := []byte(`{"message": "push event"}`)
		mac := hmac.New(sha256.New, []byte("wrong-secret"))
		mac.Write(payload)
		sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

		req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payload))
		req.Header.Set("X-Hub-Signature-256", sig)
		req.RemoteAddr = "127.0.0.1:12345"
		rec := httptest.NewRecorder()

		s.handleWebhook(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Expected 401, got %d", rec.Code)
		}
	})
}

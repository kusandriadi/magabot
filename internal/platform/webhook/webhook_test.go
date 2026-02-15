package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
		AuthMethod: "none",
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
		body := bytes.NewReader([]byte(`{"message": "hello"}`))
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

		body := bytes.NewReader([]byte(`{"message": "hello"}`))
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

		if !s.authenticate(req) {
			t.Error("Valid bearer token should authenticate")
		}
	})

	t.Run("InvalidToken", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
		req.Header.Set("Authorization", "Bearer wrong-token")

		if s.authenticate(req) {
			t.Error("Invalid bearer token should not authenticate")
		}
	})

	t.Run("MissingToken", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/webhook", nil)

		if s.authenticate(req) {
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

		if !s.authenticate(req) {
			t.Error("Valid basic auth should authenticate")
		}
	})

	t.Run("WrongPassword", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
		req.SetBasicAuth("admin", "wrongpassword")

		if s.authenticate(req) {
			t.Error("Wrong password should not authenticate")
		}
	})

	t.Run("WrongUser", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
		req.SetBasicAuth("wronguser", "password123")

		if s.authenticate(req) {
			t.Error("Wrong user should not authenticate")
		}
	})

	t.Run("NoAuth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/webhook", nil)

		if s.authenticate(req) {
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

		if !s.authenticate(req) {
			t.Error("Valid HMAC should authenticate")
		}
	})

	t.Run("InvalidSignature", func(t *testing.T) {
		body := []byte(`{"event": "test"}`)
		req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
		req.Header.Set("X-Hub-Signature-256", "sha256=invalid")

		if s.authenticate(req) {
			t.Error("Invalid HMAC should not authenticate")
		}
	})

	t.Run("MissingSignature", func(t *testing.T) {
		body := []byte(`{"event": "test"}`)
		req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))

		if s.authenticate(req) {
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

		if !s.authenticate(req) {
			t.Error("X-Signature header should work")
		}
	})
}

func TestAuthenticateNone(t *testing.T) {
	t.Run("AuthNone", func(t *testing.T) {
		s := newTestServer(&Config{AuthMethod: "none"})
		req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
		if !s.authenticate(req) {
			t.Error("Auth none should always pass")
		}
	})

	t.Run("AuthEmpty", func(t *testing.T) {
		s := newTestServer(&Config{AuthMethod: ""})
		req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
		if !s.authenticate(req) {
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
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	s.handleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "OK" {
		t.Errorf("Expected 'OK', got %s", rec.Body.String())
	}
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
	s := newTestServer(&Config{AuthMethod: "none"})
	s.SetHandler(func(ctx context.Context, msg *router.Message) (string, error) {
		return "bot response", nil
	})

	body := bytes.NewReader([]byte(`{"message": "test"}`))
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

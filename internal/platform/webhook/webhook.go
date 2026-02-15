// Package webhook provides HTTP webhook receiver
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kusa/magabot/internal/router"
)

// Server represents a webhook server
type Server struct {
	server    *http.Server
	handler   router.MessageHandler
	handlerMu sync.RWMutex
	logger    *slog.Logger
	config    *Config
	done      chan struct{}
	wg        sync.WaitGroup
}

// Config for webhook server
type Config struct {
	Port         int
	Path         string
	Bind         string
	AuthMethod   string            // none, bearer, basic, hmac
	BearerTokens map[string]string // token -> user_id mapping (secure: token IS the identity)
	BearerToken  string            // legacy: single token (user_id from payload - less secure)
	BasicUser    string
	BasicPass    string
	HMACSecret   string
	HMACUsers    map[string]string // secret -> user_id mapping
	AllowedIPs   []string
	AllowedUsers []string // Required: allowed user IDs
	MaxBodySize  int64
	Logger       *slog.Logger
}

// New creates a new webhook server
func New(cfg *Config) (*Server, error) {
	if cfg.Bind == "" {
		cfg.Bind = "127.0.0.1"
	}
	if cfg.Port == 0 {
		cfg.Port = 8080
	}
	if cfg.Path == "" {
		cfg.Path = "/webhook"
	}
	if cfg.MaxBodySize == 0 {
		cfg.MaxBodySize = 1048576 // 1MB
	}

	return &Server{
		config: cfg,
		logger: cfg.Logger,
		done:   make(chan struct{}),
	}, nil
}

// Name returns the platform name
func (s *Server) Name() string {
	return "webhook"
}

// Start starts the webhook server
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc(s.config.Path, s.handleWebhook)
	mux.HandleFunc("/health", s.handleHealth)

	addr := fmt.Sprintf("%s:%d", s.config.Bind, s.config.Port)
	s.server = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.logger.Info("webhook server starting", "addr", addr, "path", s.config.Path)
		if err := s.server.ListenAndServe(); err != http.ErrServerClosed {
			s.logger.Error("webhook server error", "error", err)
		}
	}()

	return nil
}

// Stop stops the server
func (s *Server) Stop() error {
	close(s.done)
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	if err := s.server.Shutdown(ctx); err != nil {
		return err
	}
	
	s.wg.Wait()
	return nil
}

// Send is not applicable for webhooks (receive-only)
func (s *Server) Send(chatID, message string) error {
	return fmt.Errorf("webhook platform is receive-only")
}

// SetHandler sets the message handler
func (s *Server) SetHandler(h router.MessageHandler) {
	s.handlerMu.Lock()
	s.handler = h
	s.handlerMu.Unlock()
}

// handleWebhook handles incoming webhook requests
func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	// Only POST allowed
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// IP whitelist check
	if !s.checkIP(r) {
		s.logger.Warn("webhook blocked by IP", "ip", getClientIP(r))
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Authentication - returns user_id from token mapping
	authUserID, ok := s.authenticate(r)
	if !ok {
		s.logger.Warn("webhook auth failed", "ip", getClientIP(r))
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Read body
	body, err := io.ReadAll(io.LimitReader(r.Body, s.config.MaxBodySize))
	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse message from payload
	text, payloadUserID := s.parsePayload(body, r)
	if text == "" {
		http.Error(w, "No message found", http.StatusBadRequest)
		return
	}

	// User ID priority: auth token > header > payload
	userID := authUserID
	if userID == "" {
		if headerUserID := r.Header.Get("X-User-ID"); headerUserID != "" {
			userID = headerUserID
		} else if payloadUserID != "" {
			userID = payloadUserID
		} else {
			userID = r.Header.Get("X-Webhook-Source")
		}
	}
	if userID == "" {
		s.logger.Warn("webhook rejected: no user_id", "ip", getClientIP(r))
		http.Error(w, "Forbidden: user_id required", http.StatusForbidden)
		return
	}

	// User allowlist check (mandatory)
	if !s.checkUser(userID) {
		s.logger.Warn("webhook blocked by user allowlist", "user_id", userID, "ip", getClientIP(r))
		http.Error(w, "Forbidden: user not allowed", http.StatusForbidden)
		return
	}

	// Build router message
	msg := &router.Message{
		Platform:  "webhook",
		ChatID:    getClientIP(r),
		UserID:    userID,
		Text:      text,
		Timestamp: time.Now(),
		Raw:       body,
	}

	if msg.UserID == "" {
		msg.UserID = "webhook"
	}

	// Process
	s.handlerMu.RLock()
	handler := s.handler
	s.handlerMu.RUnlock()

	if handler != nil {
		response, err := handler(r.Context(), msg)
		if err != nil {
			s.logger.Warn("handler error", "error", err)
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":       true,
			"response": response,
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// handleHealth handles health check
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// authenticate verifies the request and returns the user_id from token mapping.
// Returns (userID, true) on success, ("", false) on failure.
// If using token-to-user mapping, the token determines the user identity (secure).
// If using legacy single token, returns ("", true) and user_id comes from payload (less secure).
func (s *Server) authenticate(r *http.Request) (string, bool) {
	switch s.config.AuthMethod {
	case "none", "":
		return "", true
		
	case "bearer":
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			return "", false
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		
		// Check token-to-user mapping (secure: token IS the identity)
		if len(s.config.BearerTokens) > 0 {
			for t, userID := range s.config.BearerTokens {
				if subtle.ConstantTimeCompare([]byte(token), []byte(t)) == 1 {
					return userID, true
				}
			}
			return "", false
		}
		
		// Legacy: single token (user_id from payload)
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.config.BearerToken)) == 1 {
			return "", true
		}
		return "", false
		
	case "basic":
		user, pass, ok := r.BasicAuth()
		if !ok {
			return "", false
		}
		if subtle.ConstantTimeCompare([]byte(user), []byte(s.config.BasicUser)) == 1 &&
			subtle.ConstantTimeCompare([]byte(pass), []byte(s.config.BasicPass)) == 1 {
			return user, true // username is the user_id
		}
		return "", false
			
	case "hmac":
		// Check X-Hub-Signature-256 (GitHub style)
		sig := r.Header.Get("X-Hub-Signature-256")
		if sig == "" {
			sig = r.Header.Get("X-Signature")
		}
		if sig == "" {
			return "", false
		}

		// Read body for signature verification
		body, err := io.ReadAll(io.LimitReader(r.Body, s.config.MaxBodySize))
		if err != nil {
			return "", false
		}
		r.Body = io.NopCloser(bytes.NewReader(body))

		// Check HMAC-to-user mapping
		if len(s.config.HMACUsers) > 0 {
			for secret, userID := range s.config.HMACUsers {
				mac := hmac.New(sha256.New, []byte(secret))
				mac.Write(body)
				expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
				if subtle.ConstantTimeCompare([]byte(sig), []byte(expected)) == 1 {
					return userID, true
				}
			}
			return "", false
		}

		// Legacy: single secret
		if s.config.HMACSecret == "" {
			return "", false
		}
		mac := hmac.New(sha256.New, []byte(s.config.HMACSecret))
		mac.Write(body)
		expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if subtle.ConstantTimeCompare([]byte(sig), []byte(expected)) == 1 {
			return "", true
		}
		return "", false
	}
	
	return "", false
}

// checkIP checks if the client IP is allowed
func (s *Server) checkIP(r *http.Request) bool {
	if len(s.config.AllowedIPs) == 0 {
		return true
	}

	clientIP := net.ParseIP(getClientIP(r))
	if clientIP == nil {
		return false
	}

	for _, allowed := range s.config.AllowedIPs {
		if strings.Contains(allowed, "/") {
			_, cidr, err := net.ParseCIDR(allowed)
			if err == nil && cidr.Contains(clientIP) {
				return true
			}
		} else {
			if allowed == clientIP.String() {
				return true
			}
		}
	}

	return false
}

// checkUser checks if the user ID is allowed
func (s *Server) checkUser(userID string) bool {
	if len(s.config.AllowedUsers) == 0 {
		return true // No allowlist = allow all
	}

	for _, allowed := range s.config.AllowedUsers {
		if allowed == userID {
			return true
		}
		// Support wildcards: "telegram:*" matches any telegram user
		if strings.HasSuffix(allowed, ":*") {
			prefix := strings.TrimSuffix(allowed, "*")
			if strings.HasPrefix(userID, prefix) {
				return true
			}
		}
	}

	return false
}

// parsePayload extracts message and user ID from payload
func (s *Server) parsePayload(body []byte, r *http.Request) (text string, userID string) {
	// Try JSON
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err == nil {
		// Extract user ID from common fields
		for _, field := range []string{"user_id", "userId", "user", "sender", "from"} {
			if v, ok := data[field].(string); ok && v != "" {
				userID = v
				break
			}
		}

		// Common message fields
		for _, field := range []string{"message", "text", "content", "body", "msg"} {
			if v, ok := data[field].(string); ok && v != "" {
				text = v
				break
			}
		}
		if text != "" {
			return text, userID
		}
		
		// GitHub webhook
		if commits, ok := data["commits"].([]interface{}); ok && len(commits) > 0 {
			if commit, ok := commits[0].(map[string]interface{}); ok {
				if msg, ok := commit["message"].(string); ok {
					text = fmt.Sprintf("GitHub push: %s", msg)
				}
			}
			// GitHub sender
			if sender, ok := data["sender"].(map[string]interface{}); ok {
				if login, ok := sender["login"].(string); ok {
					userID = "github:" + login
				}
			}
			return text, userID
		}
		
		// Grafana alert
		if title, ok := data["title"].(string); ok {
			if state, ok := data["state"].(string); ok {
				text = fmt.Sprintf("Grafana [%s]: %s", state, title)
				userID = "grafana"
				return text, userID
			}
		}
	}

	// Fallback: raw body as text
	return string(body), ""
}

// getClientIP returns the direct TCP peer address for security checks.
// Proxy headers (X-Forwarded-For, X-Real-IP) are NOT trusted since they
// can be spoofed by clients to bypass IP allowlists.
func getClientIP(r *http.Request) string {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}

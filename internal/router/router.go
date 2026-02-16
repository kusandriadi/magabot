// Package router handles message routing across platforms
package router

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/kusa/magabot/internal/config"
	"github.com/kusa/magabot/internal/hooks"
	"github.com/kusa/magabot/internal/security"
	"github.com/kusa/magabot/internal/storage"
)

// Platform interface for chat platforms
type Platform interface {
	// Name returns the platform name
	Name() string

	// Start starts the platform handler
	Start(ctx context.Context) error

	// Stop gracefully stops the platform
	Stop() error

	// Send sends a message
	Send(chatID, message string) error

	// SetHandler sets the message handler
	SetHandler(handler MessageHandler)
}

// Message represents an incoming message
type Message struct {
	Platform  string
	ChatID    string
	UserID    string
	Username  string
	Text      string
	Media     []string // File paths for images/voice/documents
	Timestamp time.Time
	Raw       interface{} // Platform-specific raw message
}

// MessageHandler handles incoming messages
type MessageHandler func(ctx context.Context, msg *Message) (string, error)

// Router routes messages between platforms
type Router struct {
	platforms    map[string]Platform
	store        *storage.Store
	vault        *security.Vault
	cfg          *config.Config
	authorizer   *security.Authorizer // fallback for legacy security.allowed_users
	rateLimiter  *security.RateLimiter
	sessionMgr   *security.SessionManager
	authAttempts *security.AuthAttempts
	auditLogger  *security.AuditLogger
	hooks        *hooks.Manager
	handler      MessageHandler
	logger       *slog.Logger
	mu           sync.RWMutex
}

// NewRouter creates a new router
func NewRouter(store *storage.Store, vault *security.Vault, cfg *config.Config, authorizer *security.Authorizer, rateLimiter *security.RateLimiter, logger *slog.Logger) *Router {
	return &Router{
		platforms:    make(map[string]Platform),
		store:        store,
		vault:        vault,
		cfg:          cfg,
		authorizer:   authorizer,
		rateLimiter:  rateLimiter,
		sessionMgr:   security.NewSessionManager(),
		authAttempts: security.NewAuthAttempts(),
		logger:       logger,
	}
}

// SetAuditLogger sets the audit logger for security events
func (r *Router) SetAuditLogger(al *security.AuditLogger) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.auditLogger = al
}

// SetHooks sets the hooks manager for event-driven shell commands.
func (r *Router) SetHooks(h *hooks.Manager) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hooks = h
}

// Register registers a platform
func (r *Router) Register(p Platform) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.platforms[p.Name()] = p
	p.SetHandler(r.handleMessage)
}

// SetHandler sets the global message handler
func (r *Router) SetHandler(h MessageHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handler = h
}

// Start starts all registered platforms
func (r *Router) Start(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for name, p := range r.platforms {
		r.logger.Info("starting platform", "platform", name)
		if err := p.Start(ctx); err != nil {
			return fmt.Errorf("start %s: %w", name, err)
		}
	}

	return nil
}

// Stop stops all platforms
func (r *Router) Stop() {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for name, p := range r.platforms {
		r.logger.Info("stopping platform", "platform", name)
		if err := p.Stop(); err != nil {
			r.logger.Error("stop platform failed", "platform", name, "error", err)
		}
	}
}

// handleMessage processes incoming messages
func (r *Router) handleMessage(ctx context.Context, msg *Message) (string, error) {
	userKey := fmt.Sprintf("%s:%s", msg.Platform, msg.UserID)
	hashedUser := security.HashUserID(msg.Platform, msg.UserID)

	// Check account lockout (A07 fix)
	if r.authAttempts.IsLocked(userKey) {
		r.logger.Warn("account locked",
			"platform", msg.Platform,
			"user_hash", hashedUser,
		)
		if r.auditLogger != nil {
			r.auditLogger.LogAuthLockout(msg.Platform, msg.UserID)
		}
		return "", security.ErrAccountLocked
	}

	// Authorization check: use config (per-platform rules) with authorizer fallback
	isAllowed := false
	if r.cfg != nil {
		isAllowed = r.cfg.IsAllowed(msg.Platform, msg.UserID, msg.ChatID, msg.ChatID != msg.UserID)
	}
	if !isAllowed && r.authorizer != nil {
		isAllowed = r.authorizer.IsAuthorized(msg.Platform, msg.UserID)
	}
	if !isAllowed {
		r.logger.Warn("unauthorized user",
			"platform", msg.Platform,
			"user_hash", hashedUser,
		)
		_ = r.store.AuditLog(msg.Platform, hashedUser, "unauthorized", "")

		// Track failed attempts (A07 fix)
		r.authAttempts.RecordFailure(userKey)
		if r.auditLogger != nil {
			r.auditLogger.LogAuthFailure(msg.Platform, msg.UserID, "not in allowlist")
		}

		return "", security.ErrNotAuthorized
	}

	// Clear any previous failures on successful auth
	r.authAttempts.ClearFailures(userKey)

	// Update/create session (A07 fix)
	r.sessionMgr.GetOrCreate(msg.Platform, msg.UserID)

	// Rate limit check
	isCommand := len(msg.Text) > 0 && msg.Text[0] == '/'
	if isCommand {
		if !r.rateLimiter.AllowCommand(userKey) {
			r.logger.Warn("rate limited (command)", "user_hash", hashedUser)
			if r.auditLogger != nil {
				r.auditLogger.LogRateLimited(msg.Platform, msg.UserID)
			}
			return "", security.ErrRateLimited
		}
	} else {
		if !r.rateLimiter.AllowMessage(userKey) {
			r.logger.Warn("rate limited (message)", "user_hash", hashedUser)
			if r.auditLogger != nil {
				r.auditLogger.LogRateLimited(msg.Platform, msg.UserID)
			}
			return "", security.ErrRateLimited
		}
	}

	// Log incoming message (encrypted)
	encryptedContent, encErr := r.vault.Encrypt([]byte(msg.Text))
	if encErr != nil {
		r.logger.Error("encrypt message failed", "error", encErr, "direction", "in")
	} else {
		if err := r.store.SaveMessage(&storage.Message{
			Platform:  msg.Platform,
			ChatID:    msg.ChatID,
			UserID:    hashedUser,
			Username:  msg.Username,
			Content:   encryptedContent,
			Timestamp: msg.Timestamp,
			Direction: "in",
		}); err != nil {
			r.logger.Error("save message failed", "error", err, "direction", "in")
		}
	}

	// Fire pre_message hook (can block or modify the message text)
	r.mu.RLock()
	hooksMgr := r.hooks
	r.mu.RUnlock()

	if hooksMgr != nil && hooksMgr.HasHooks(hooks.PreMessage) {
		result := hooksMgr.Fire(hooks.PreMessage, &hooks.EventData{
			Platform: msg.Platform,
			UserID:   msg.UserID,
			ChatID:   msg.ChatID,
			Text:     msg.Text,
		})
		if result.Blocked {
			r.logger.Info("message blocked by pre_message hook", "user_hash", hashedUser)
			return "", nil
		}
		if result.Output != "" {
			msg.Text = result.Output
		}
	}

	// Process message
	r.mu.RLock()
	handler := r.handler
	r.mu.RUnlock()

	if handler == nil {
		return "", nil
	}

	response, err := handler(ctx, msg)
	if err != nil {
		r.logger.Error("handler error", "error", err, "user_hash", hashedUser)
		// Fire on_error hook
		if hooksMgr != nil {
			hooksMgr.FireAsync(hooks.OnError, &hooks.EventData{
				Platform: msg.Platform,
				UserID:   msg.UserID,
				ChatID:   msg.ChatID,
				Text:     msg.Text,
				Error:    err.Error(),
			})
		}
		return "", err
	}

	// Fire post_response hook (can modify the response text)
	if hooksMgr != nil && response != "" && hooksMgr.HasHooks(hooks.PostResponse) {
		result := hooksMgr.Fire(hooks.PostResponse, &hooks.EventData{
			Platform: msg.Platform,
			UserID:   msg.UserID,
			ChatID:   msg.ChatID,
			Text:     msg.Text,
			Response: response,
		})
		if result.Output != "" {
			response = result.Output
		}
	}

	// Log outgoing message
	if response != "" {
		encryptedResponse, encErr := r.vault.Encrypt([]byte(response))
		if encErr != nil {
			r.logger.Error("encrypt message failed", "error", encErr, "direction", "out")
		} else {
			if err := r.store.SaveMessage(&storage.Message{
				Platform:  msg.Platform,
				ChatID:    msg.ChatID,
				UserID:    "bot",
				Content:   encryptedResponse,
				Timestamp: time.Now(),
				Direction: "out",
			}); err != nil {
				r.logger.Error("save message failed", "error", err, "direction", "out")
			}
		}
	}

	return response, nil
}

// Send sends a message to a specific platform and chat
func (r *Router) Send(platform, chatID, message string) error {
	r.mu.RLock()
	p, ok := r.platforms[platform]
	r.mu.RUnlock()

	if !ok {
		return fmt.Errorf("unknown platform: %s", platform)
	}

	return p.Send(chatID, message)
}

// Platforms returns list of registered platforms
func (r *Router) Platforms() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.platforms))
	for name := range r.platforms {
		names = append(names, name)
	}
	return names
}

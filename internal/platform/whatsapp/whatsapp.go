// Package whatsapp provides WhatsApp integration via whatsmeow (multi-device)
// Note: Full implementation requires github.com/tulir/whatsmeow
package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kusa/magabot/internal/router"
	"github.com/kusa/magabot/internal/security"
)

// Bot represents a WhatsApp bot
type Bot struct {
	sessionPath string
	vault       *security.Vault
	handler     router.MessageHandler
	logger      *slog.Logger
	connected   bool
	done        chan struct{}
	mu          sync.RWMutex
}

// Config for WhatsApp bot
type Config struct {
	SessionPath string
	Vault       *security.Vault
	Logger      *slog.Logger
}

// SessionData represents encrypted session data
type SessionData struct {
	DeviceID  string `json:"device_id"`
	Session   string `json:"session"` // Encrypted
	UpdatedAt string `json:"updated_at"`
}

// New creates a new WhatsApp bot
func New(cfg *Config) (*Bot, error) {
	if err := os.MkdirAll(cfg.SessionPath, 0700); err != nil {
		return nil, fmt.Errorf("create session dir: %w", err)
	}

	return &Bot{
		sessionPath: cfg.SessionPath,
		vault:       cfg.Vault,
		logger:      cfg.Logger,
		done:        make(chan struct{}),
	}, nil
}

// Name returns the platform name
func (b *Bot) Name() string {
	return "whatsapp"
}

// Start starts the WhatsApp client
func (b *Bot) Start(ctx context.Context) error {
	sessionFile := filepath.Join(b.sessionPath, "whatsapp_session.json")
	
	if _, err := os.Stat(sessionFile); os.IsNotExist(err) {
		b.logger.Info("no WhatsApp session found")
		b.logger.Info("run 'magabot whatsapp login' to scan QR code")
		return nil
	}

	data, err := os.ReadFile(sessionFile)
	if err != nil {
		return fmt.Errorf("read session: %w", err)
	}

	var session SessionData
	if err := json.Unmarshal(data, &session); err != nil {
		return fmt.Errorf("parse session: %w", err)
	}

	if b.vault != nil {
		decrypted, err := b.vault.Decrypt(session.Session)
		if err != nil {
			return fmt.Errorf("decrypt session: %w", err)
		}
		session.Session = string(decrypted)
	}

	// TODO: Initialize whatsmeow client with session
	b.logger.Info("WhatsApp session loaded", "device_id", session.DeviceID)
	b.connected = true

	return nil
}

// Stop stops the WhatsApp client
func (b *Bot) Stop() error {
	close(b.done)
	return nil
}

// Send sends a message
func (b *Bot) Send(chatID, message string) error {
	b.mu.RLock()
	connected := b.connected
	b.mu.RUnlock()

	if !connected {
		return fmt.Errorf("WhatsApp not connected")
	}

	// TODO: Send via whatsmeow
	b.logger.Info("send message", "chat_id", chatID, "length", len(message))
	return nil
}

// SetHandler sets the message handler
func (b *Bot) SetHandler(h router.MessageHandler) {
	b.handler = h
}

// SaveSession saves encrypted session data
func (b *Bot) SaveSession(deviceID, sessionData string) error {
	var encryptedSession string
	if b.vault != nil {
		var err error
		encryptedSession, err = b.vault.Encrypt([]byte(sessionData))
		if err != nil {
			return fmt.Errorf("encrypt session: %w", err)
		}
	} else {
		encryptedSession = sessionData
	}

	session := SessionData{
		DeviceID:  deviceID,
		Session:   encryptedSession,
		UpdatedAt: time.Now().Format(time.RFC3339),
	}

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}

	sessionFile := filepath.Join(b.sessionPath, "whatsapp_session.json")
	return os.WriteFile(sessionFile, data, 0600)
}

// ClearSession removes the session file
func (b *Bot) ClearSession() error {
	sessionFile := filepath.Join(b.sessionPath, "whatsapp_session.json")
	if err := os.Remove(sessionFile); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// IsConnected returns connection status
func (b *Bot) IsConnected() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.connected
}

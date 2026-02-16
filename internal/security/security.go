// Package security provides encryption, authentication and authorization
package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

var (
	ErrInvalidKey    = errors.New("invalid encryption key")
	ErrDecryptFailed = errors.New("decryption failed")
	ErrNotAuthorized = errors.New("user not authorized")
	ErrRateLimited   = errors.New("rate limit exceeded")
)

// Vault handles encryption/decryption of sensitive data
type Vault struct {
	gcm cipher.AEAD
	mu  sync.RWMutex
}

// NewVault creates a new vault with the given key (base64 encoded, 32 bytes)
func NewVault(keyBase64 string) (*Vault, error) {
	key, err := base64.StdEncoding.DecodeString(keyBase64)
	if err != nil || len(key) != 32 {
		return nil, ErrInvalidKey
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return &Vault{gcm: gcm}, nil
}

// Encrypt encrypts plaintext and returns base64 encoded ciphertext
func (v *Vault) Encrypt(plaintext []byte) (string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	nonce := make([]byte, v.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := v.gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts base64 encoded ciphertext
func (v *Vault) Decrypt(ciphertextBase64 string) ([]byte, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextBase64)
	if err != nil {
		return nil, err
	}

	nonceSize := v.gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, ErrDecryptFailed
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := v.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrDecryptFailed
	}

	return plaintext, nil
}

// GenerateKey generates a new random encryption key
func GenerateKey() string {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return base64.StdEncoding.EncodeToString(key)
}

// HashUserID creates a consistent hash of user ID for logging (privacy)
func HashUserID(platform, userID string) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s:%s", platform, userID)))
	return base64.StdEncoding.EncodeToString(h[:8])
}

// Authorizer manages user authorization
type Authorizer struct {
	allowedUsers map[string]map[string]bool // platform -> userID -> allowed
	mu           sync.RWMutex
}

// NewAuthorizer creates a new authorizer
func NewAuthorizer() *Authorizer {
	return &Authorizer{
		allowedUsers: make(map[string]map[string]bool),
	}
}

// SetAllowedUsers sets the allowed users for a platform
func (a *Authorizer) SetAllowedUsers(platform string, userIDs []string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.allowedUsers[platform] = make(map[string]bool)
	for _, id := range userIDs {
		a.allowedUsers[platform][id] = true
	}
}

// IsAuthorized checks if a user is authorized
func (a *Authorizer) IsAuthorized(platform, userID string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	users, ok := a.allowedUsers[platform]
	if !ok {
		return false
	}

	// Empty whitelist = allow all (for initial setup)
	if len(users) == 0 {
		return true
	}

	return users[userID]
}

// RateLimiter implements sliding window rate limiting
type RateLimiter struct {
	limits    map[string]*userLimit
	maxMsg    int
	maxCmd    int
	window    time.Duration
	mu        sync.Mutex
	callCount int // for periodic cleanup
}

type userLimit struct {
	messages []time.Time
	commands []time.Time
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(messagesPerMinute, commandsPerMinute int) *RateLimiter {
	return &RateLimiter{
		limits: make(map[string]*userLimit),
		maxMsg: messagesPerMinute,
		maxCmd: commandsPerMinute,
		window: time.Minute,
	}
}

// AllowMessage checks if a message is allowed
func (r *RateLimiter) AllowMessage(userKey string) bool {
	return r.allow(userKey, false)
}

// AllowCommand checks if a command is allowed
func (r *RateLimiter) AllowCommand(userKey string) bool {
	return r.allow(userKey, true)
}

func (r *RateLimiter) allow(userKey string, isCommand bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-r.window)

	// Periodic cleanup: every 200 calls, purge stale entries to prevent unbounded growth
	r.callCount++
	if r.callCount%200 == 0 {
		staleThreshold := now.Add(-r.window * 2)
		for key, limit := range r.limits {
			if key == userKey {
				continue
			}
			hasRecent := false
			for _, t := range limit.messages {
				if t.After(staleThreshold) {
					hasRecent = true
					break
				}
			}
			if !hasRecent {
				for _, t := range limit.commands {
					if t.After(staleThreshold) {
						hasRecent = true
						break
					}
				}
			}
			if !hasRecent {
				delete(r.limits, key)
			}
		}
	}

	limit, ok := r.limits[userKey]
	if !ok {
		limit = &userLimit{}
		r.limits[userKey] = limit
	}

	if isCommand {
		// Clean old entries
		var fresh []time.Time
		for _, t := range limit.commands {
			if t.After(cutoff) {
				fresh = append(fresh, t)
			}
		}
		limit.commands = fresh

		if len(limit.commands) >= r.maxCmd {
			return false
		}
		limit.commands = append(limit.commands, now)
	} else {
		var fresh []time.Time
		for _, t := range limit.messages {
			if t.After(cutoff) {
				fresh = append(fresh, t)
			}
		}
		limit.messages = fresh

		if len(limit.messages) >= r.maxMsg {
			return false
		}
		limit.messages = append(limit.messages, now)
	}

	return true
}

// Cleanup removes stale entries (call periodically)
func (r *RateLimiter) Cleanup() {
	r.mu.Lock()
	defer r.mu.Unlock()

	cutoff := time.Now().Add(-r.window * 2)
	for key, limit := range r.limits {
		hasRecent := false
		for _, t := range limit.messages {
			if t.After(cutoff) {
				hasRecent = true
				break
			}
		}
		for _, t := range limit.commands {
			if t.After(cutoff) {
				hasRecent = true
				break
			}
		}
		if !hasRecent {
			delete(r.limits, key)
		}
	}
}

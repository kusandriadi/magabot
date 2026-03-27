// Package security - Data integrity with HMAC (A08 fix)
package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	ErrInvalidSignature = errors.New("invalid signature")
	ErrExpiredMessage   = errors.New("message expired")
	ErrInvalidFormat    = errors.New("invalid message format")
)

// SignedMessage represents a message with HMAC signature
type SignedMessage struct {
	Content   string `json:"c"` // Base64 encoded content
	Timestamp int64  `json:"t"` // Unix timestamp
	Signature string `json:"s"` // HMAC-SHA256 signature
}

// Signer provides HMAC signing and verification
type Signer struct {
	key []byte
	ttl time.Duration // Message time-to-live (0 = no expiry)
}

// NewSigner creates a new HMAC signer
func NewSigner(keyBase64 string, ttl time.Duration) (*Signer, error) {
	key, err := base64.StdEncoding.DecodeString(keyBase64)
	if err != nil {
		return nil, ErrInvalidKey
	}

	if len(key) < 32 {
		return nil, errors.New("key must be at least 256 bits")
	}

	return &Signer{
		key: key,
		ttl: ttl,
	}, nil
}

// Sign creates an HMAC-signed message
func (s *Signer) Sign(content []byte) (string, error) {
	msg := SignedMessage{
		Content:   base64.StdEncoding.EncodeToString(content),
		Timestamp: time.Now().Unix(),
	}

	// Create signature over content + timestamp
	sig := s.computeSignature(msg.Content, msg.Timestamp)
	msg.Signature = base64.StdEncoding.EncodeToString(sig)

	// Encode entire message as JSON then base64
	jsonBytes, err := json.Marshal(msg)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(jsonBytes), nil
}

// Verify verifies and extracts content from signed message
func (s *Signer) Verify(signedMessage string) ([]byte, error) {
	// Decode outer base64
	jsonBytes, err := base64.StdEncoding.DecodeString(signedMessage)
	if err != nil {
		return nil, ErrInvalidFormat
	}

	// Parse JSON
	var msg SignedMessage
	if err := json.Unmarshal(jsonBytes, &msg); err != nil {
		return nil, ErrInvalidFormat
	}

	// Check expiry if TTL is set
	if s.ttl > 0 {
		msgTime := time.Unix(msg.Timestamp, 0)
		if time.Since(msgTime) > s.ttl {
			return nil, ErrExpiredMessage
		}
	}

	// Verify signature
	expectedSig := s.computeSignature(msg.Content, msg.Timestamp)
	actualSig, err := base64.StdEncoding.DecodeString(msg.Signature)
	if err != nil {
		return nil, ErrInvalidSignature
	}

	if !hmac.Equal(expectedSig, actualSig) {
		return nil, ErrInvalidSignature
	}

	// Decode content
	content, err := base64.StdEncoding.DecodeString(msg.Content)
	if err != nil {
		return nil, ErrInvalidFormat
	}

	return content, nil
}

func (s *Signer) computeSignature(contentBase64 string, timestamp int64) []byte {
	h := hmac.New(sha256.New, s.key)
	h.Write([]byte(fmt.Sprintf("%s|%d", contentBase64, timestamp)))
	return h.Sum(nil)
}

// GenerateSigningKey generates a new random signing key
func GenerateSigningKey() string {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return base64.StdEncoding.EncodeToString(key)
}

// HashForIntegrity creates a SHA-256 hash of data for integrity checking
func HashForIntegrity(data []byte) string {
	h := sha256.Sum256(data)
	return base64.StdEncoding.EncodeToString(h[:])
}

// VerifyHash checks if data matches the expected hash
func VerifyHash(data []byte, expectedHash string) bool {
	actualHash := HashForIntegrity(data)
	return hmac.Equal([]byte(actualHash), []byte(expectedHash))
}

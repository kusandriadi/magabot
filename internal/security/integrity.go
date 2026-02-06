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
	Content   string `json:"c"`           // Base64 encoded content
	Timestamp int64  `json:"t"`           // Unix timestamp
	Signature string `json:"s"`           // HMAC-SHA256 signature
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

// IntegrityVault combines encryption with HMAC for authenticated encryption
// Note: AES-GCM already provides authentication, but this adds an extra layer
// for sensitive data and enables signature verification without decryption
type IntegrityVault struct {
	vault  *Vault
	signer *Signer
}

// NewIntegrityVault creates a vault with both encryption and signing
func NewIntegrityVault(encKeyBase64, signKeyBase64 string) (*IntegrityVault, error) {
	vault, err := NewVault(encKeyBase64)
	if err != nil {
		return nil, fmt.Errorf("create vault: %w", err)
	}
	
	signer, err := NewSigner(signKeyBase64, 0) // No TTL for stored data
	if err != nil {
		return nil, fmt.Errorf("create signer: %w", err)
	}
	
	return &IntegrityVault{
		vault:  vault,
		signer: signer,
	}, nil
}

// EncryptAndSign encrypts then signs data
func (iv *IntegrityVault) EncryptAndSign(plaintext []byte) (string, error) {
	// First encrypt
	ciphertext, err := iv.vault.Encrypt(plaintext)
	if err != nil {
		return "", fmt.Errorf("encrypt: %w", err)
	}
	
	// Then sign the ciphertext
	signed, err := iv.signer.Sign([]byte(ciphertext))
	if err != nil {
		return "", fmt.Errorf("sign: %w", err)
	}
	
	return signed, nil
}

// VerifyAndDecrypt verifies signature then decrypts
func (iv *IntegrityVault) VerifyAndDecrypt(signedCiphertext string) ([]byte, error) {
	// First verify signature
	ciphertext, err := iv.signer.Verify(signedCiphertext)
	if err != nil {
		return nil, fmt.Errorf("verify: %w", err)
	}
	
	// Then decrypt
	plaintext, err := iv.vault.Decrypt(string(ciphertext))
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	
	return plaintext, nil
}

// GenerateSigningKey generates a new random signing key
func GenerateSigningKey() string {
	key := make([]byte, 32)
	rand.Read(key)
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

// FileIntegrity provides file integrity checking
type FileIntegrity struct {
	hashes map[string]string // path -> hash
}

// NewFileIntegrity creates a new file integrity checker
func NewFileIntegrity() *FileIntegrity {
	return &FileIntegrity{
		hashes: make(map[string]string),
	}
}

// RecordHash records the hash of file content
func (fi *FileIntegrity) RecordHash(path string, content []byte) {
	fi.hashes[path] = HashForIntegrity(content)
}

// VerifyFile checks if file content matches recorded hash
func (fi *FileIntegrity) VerifyFile(path string, content []byte) bool {
	expected, ok := fi.hashes[path]
	if !ok {
		return false
	}
	return VerifyHash(content, expected)
}

// GetHash returns the recorded hash for a path
func (fi *FileIntegrity) GetHash(path string) (string, bool) {
	h, ok := fi.hashes[path]
	return h, ok
}

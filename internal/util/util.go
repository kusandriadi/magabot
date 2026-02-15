// Package util provides common utility functions
package util

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// DefaultHTTPTimeout is the default timeout for HTTP clients
const DefaultHTTPTimeout = 30 * time.Second

// NewHTTPClient creates a new HTTP client with the specified timeout
func NewHTTPClient(timeout time.Duration) *http.Client {
	if timeout == 0 {
		timeout = DefaultHTTPTimeout
	}
	return &http.Client{Timeout: timeout}
}

// ResolveAPIKey returns configKey if non-empty, otherwise checks each
// environment variable in order and returns the first non-empty value.
func ResolveAPIKey(configKey string, envVars ...string) string {
	if configKey != "" {
		return configKey
	}
	for _, env := range envVars {
		if v := os.Getenv(env); v != "" {
			return v
		}
	}
	return ""
}

// Truncate shortens a string to max length with ellipsis
func Truncate(s string, max int) string {
	if max <= 3 {
		return s
	}
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// TruncateRunes truncates by rune count (for unicode)
func TruncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}

// RandomID generates a random hex ID
func RandomID(length int) string {
	bytes := make([]byte, length/2+1)
	if _, err := rand.Read(bytes); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(bytes)[:length]
}

// RandomToken generates a secure random token
func RandomToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// SanitizeInput removes potentially dangerous characters
func SanitizeInput(s string) string {
	// Remove null bytes
	s = strings.ReplaceAll(s, "\x00", "")
	// Remove control characters except newline and tab
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' && r != '\r' {
			return -1
		}
		return r
	}, s)
}

// SanitizeFilename removes unsafe characters from filename
func SanitizeFilename(s string) string {
	// Remove path separators and null bytes
	unsafe := regexp.MustCompile(`[/\\:\x00]`)
	return unsafe.ReplaceAllString(s, "_")
}

// Contains checks if slice contains item
func Contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// Remove removes item from slice
func Remove(slice []string, item string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	return result
}

// AddUnique adds item to slice if not already present
func AddUnique(slice []string, item string) []string {
	if !Contains(slice, item) {
		return append(slice, item)
	}
	return slice
}

// MaskSecret masks a secret string for display
func MaskSecret(s string) string {
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "..." + s[len(s)-4:]
}

// IsValidID checks if string is a valid identifier
func IsValidID(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for i, r := range s {
		if i == 0 && !unicode.IsLetter(r) && r != '_' {
			return false
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

// URL validation utilities
package util

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// allowedSchemes for API base URLs (unexported to prevent external mutation)
var allowedSchemes = map[string]bool{
	"http":  true,
	"https": true,
}

// blockedHosts are hosts that should never be accessed by LLM providers (SSRF protection)
var blockedHosts = map[string]bool{
	"169.254.169.254":          true, // AWS metadata
	"metadata.google.internal": true, // GCP metadata
	"100.100.100.200":          true, // Alibaba Cloud metadata
}

// ValidateBaseURL validates a base URL for LLM providers to prevent SSRF.
// If allowLocal is true, localhost/127.0.0.1/private IPs are permitted (for local LLM servers).
func ValidateBaseURL(rawURL string) error {
	return validateBaseURL(rawURL, false)
}

// ValidateLocalBaseURL validates a base URL allowing localhost and private IPs.
// Use this for local/self-hosted LLM servers (Ollama, vLLM, etc).
func ValidateLocalBaseURL(rawURL string) error {
	return validateBaseURL(rawURL, true)
}

func validateBaseURL(rawURL string, allowLocal bool) error {
	if rawURL == "" {
		return nil // Empty is OK (use provider default)
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Check scheme
	if !allowedSchemes[u.Scheme] {
		return fmt.Errorf("invalid URL scheme: %s (allowed: http, https)", u.Scheme)
	}

	// Check host is not empty
	if u.Host == "" {
		return fmt.Errorf("invalid URL: missing host")
	}

	host := strings.ToLower(u.Hostname())

	// Always block cloud metadata endpoints (even for local)
	if blockedHosts[host] {
		return fmt.Errorf("blocked host: %s (SSRF protection)", host)
	}

	// If local is allowed, skip remaining checks
	if allowLocal {
		return nil
	}

	// Block localhost variants
	if host == "localhost" || host == "127.0.0.1" || host == "[::1]" || host == "0.0.0.0" {
		return fmt.Errorf("blocked host: %s (use ValidateLocalBaseURL for local providers)", host)
	}

	// Block private IP ranges (RFC 1918)
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("blocked private IP: %s (SSRF protection)", host)
		}
	}

	return nil
}

// IsLocalURL checks if a URL points to localhost/127.0.0.1
func IsLocalURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "localhost" || host == "127.0.0.1" || host == "[::1]"
}

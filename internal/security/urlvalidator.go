// Package security - URL validation for SSRF prevention (A10 fix)
package security

import (
	"errors"
	"net"
	"net/url"
	"strings"
	"time"
)

var (
	ErrInvalidURL     = errors.New("invalid URL")
	ErrBlockedScheme  = errors.New("URL scheme not allowed")
	ErrBlockedHost    = errors.New("host not allowed")
	ErrPrivateIP      = errors.New("private/internal IP not allowed")
	ErrInvalidPort    = errors.New("port not allowed")
	ErrResolveFailed  = errors.New("failed to resolve hostname")
	ErrBlockedKeyword = errors.New("URL contains blocked keyword")
)

// blockedHosts are hostnames that should never be accessed
var blockedHosts = []string{
	"localhost",
	"127.0.0.1",
	"::1",
	"0.0.0.0",
	"metadata.google.internal", // GCP metadata
	"metadata.google",          // GCP metadata
	"169.254.169.254",          // Cloud metadata (AWS, GCP, Azure)
	"metadata.azure.com",       // Azure metadata
	"169.254.170.2",            // AWS ECS metadata
	"fd00:ec2::254",            // AWS EC2 metadata IPv6
}

// blockedSchemes are URL schemes that should not be allowed
var blockedSchemes = []string{
	"file",
	"ftp",
	"gopher",
	"dict",
	"ldap",
	"ldaps",
	"tftp",
}

// blockedKeywords in URLs (case-insensitive check)
var blockedKeywords = []string{
	"..%2f", // Path traversal encoded
	"..%5c", // Path traversal encoded
	"%00",   // Null byte
	"@",     // URL credential injection
}

// allowedPorts for HTTP/HTTPS
var allowedPorts = map[string]bool{
	"":     true, // Default port
	"80":   true,
	"443":  true,
	"8080": true,
	"8443": true,
	"3000": true, // Common dev ports
	"5000": true,
	"8000": true,
	"9000": true,
}

// URLValidator validates URLs to prevent SSRF attacks
type URLValidator struct {
	allowedSchemes  []string
	blockedHosts    []string
	blockedKeywords []string
	allowPrivateIPs bool
	resolveTimeout  time.Duration
	allowedDomains  []string // If set, only these domains allowed
	blockedDomains  []string // Always blocked
}

// NewURLValidator creates a new URL validator with secure defaults
func NewURLValidator() *URLValidator {
	return &URLValidator{
		allowedSchemes:  []string{"http", "https"},
		blockedHosts:    blockedHosts,
		blockedKeywords: blockedKeywords,
		allowPrivateIPs: false,
		resolveTimeout:  5 * time.Second,
	}
}

// WithAllowedDomains sets domains that are allowed (whitelist mode)
func (v *URLValidator) WithAllowedDomains(domains []string) *URLValidator {
	v.allowedDomains = domains
	return v
}

// WithBlockedDomains adds additional blocked domains
func (v *URLValidator) WithBlockedDomains(domains []string) *URLValidator {
	v.blockedDomains = domains
	return v
}

// WithAllowPrivateIPs allows private IP addresses (use with caution!)
func (v *URLValidator) WithAllowPrivateIPs(allow bool) *URLValidator {
	v.allowPrivateIPs = allow
	return v
}

// Validate checks if a URL is safe to fetch
func (v *URLValidator) Validate(rawURL string) error {
	// Parse URL
	u, err := url.Parse(rawURL)
	if err != nil {
		return ErrInvalidURL
	}

	// Check scheme
	if !v.isAllowedScheme(u.Scheme) {
		return ErrBlockedScheme
	}

	// Check for blocked keywords
	lowerURL := strings.ToLower(rawURL)
	for _, keyword := range v.blockedKeywords {
		if strings.Contains(lowerURL, keyword) {
			return ErrBlockedKeyword
		}
	}

	// Check port
	port := u.Port()
	if port != "" && !allowedPorts[port] {
		return ErrInvalidPort
	}

	hostname := u.Hostname()

	// Check blocked hosts
	lowerHost := strings.ToLower(hostname)
	for _, blocked := range v.blockedHosts {
		if lowerHost == strings.ToLower(blocked) {
			return ErrBlockedHost
		}
	}

	// Check additional blocked domains
	for _, blocked := range v.blockedDomains {
		if lowerHost == strings.ToLower(blocked) ||
			strings.HasSuffix(lowerHost, "."+strings.ToLower(blocked)) {
			return ErrBlockedHost
		}
	}

	// Check allowed domains (whitelist mode)
	if len(v.allowedDomains) > 0 {
		allowed := false
		for _, domain := range v.allowedDomains {
			if lowerHost == strings.ToLower(domain) ||
				strings.HasSuffix(lowerHost, "."+strings.ToLower(domain)) {
				allowed = true
				break
			}
		}
		if !allowed {
			return ErrBlockedHost
		}
	}

	// Resolve hostname and check IP
	if err := v.validateResolvedIPs(hostname); err != nil {
		return err
	}

	return nil
}

func (v *URLValidator) isAllowedScheme(scheme string) bool {
	scheme = strings.ToLower(scheme)

	// Check blocked first
	for _, blocked := range blockedSchemes {
		if scheme == blocked {
			return false
		}
	}

	// Check allowed
	for _, allowed := range v.allowedSchemes {
		if scheme == allowed {
			return true
		}
	}

	return false
}

func (v *URLValidator) validateResolvedIPs(hostname string) error {
	// Check if hostname is already an IP
	if ip := net.ParseIP(hostname); ip != nil {
		return v.validateIP(ip)
	}

	// Resolve hostname
	ips, err := net.LookupIP(hostname)
	if err != nil {
		return ErrResolveFailed
	}

	// Check all resolved IPs
	for _, ip := range ips {
		if err := v.validateIP(ip); err != nil {
			return err
		}
	}

	return nil
}

func (v *URLValidator) validateIP(ip net.IP) error {
	if v.allowPrivateIPs {
		return nil
	}

	// Block loopback
	if ip.IsLoopback() {
		return ErrPrivateIP
	}

	// Block private
	if ip.IsPrivate() {
		return ErrPrivateIP
	}

	// Block link-local
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return ErrPrivateIP
	}

	// Block unspecified (0.0.0.0 or ::)
	if ip.IsUnspecified() {
		return ErrPrivateIP
	}

	// Check specific cloud metadata IPs
	ipStr := ip.String()
	for _, blocked := range blockedHosts {
		if ipStr == blocked {
			return ErrBlockedHost
		}
	}

	// Block reserved ranges (additional safety)
	if v.isReservedIP(ip) {
		return ErrPrivateIP
	}

	return nil
}

func (v *URLValidator) isReservedIP(ip net.IP) bool {
	// Additional reserved ranges to block
	reservedCIDRs := []string{
		"0.0.0.0/8",          // Current network
		"10.0.0.0/8",         // Private
		"100.64.0.0/10",      // Shared address space (CGNAT)
		"127.0.0.0/8",        // Loopback
		"169.254.0.0/16",     // Link-local
		"172.16.0.0/12",      // Private
		"192.0.0.0/24",       // IETF protocol assignments
		"192.0.2.0/24",       // TEST-NET-1
		"192.168.0.0/16",     // Private
		"198.18.0.0/15",      // Benchmarking
		"198.51.100.0/24",    // TEST-NET-2
		"203.0.113.0/24",     // TEST-NET-3
		"224.0.0.0/4",        // Multicast
		"240.0.0.0/4",        // Reserved
		"255.255.255.255/32", // Broadcast
		// IPv6
		"::1/128",   // Loopback
		"::/128",    // Unspecified
		"fc00::/7",  // Unique local
		"fe80::/10", // Link-local
		"ff00::/8",  // Multicast
	}

	for _, cidr := range reservedCIDRs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}

	return false
}

// ValidateURL is a convenience function using default validator
func ValidateURL(rawURL string) error {
	return NewURLValidator().Validate(rawURL)
}

// MustValidateURL panics if URL is invalid (use in tests)
func MustValidateURL(rawURL string) {
	if err := ValidateURL(rawURL); err != nil {
		panic(err)
	}
}

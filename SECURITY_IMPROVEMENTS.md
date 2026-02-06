# Security Improvements Roadmap

## Priority 1: Update Dependencies (A06 - Vulnerable Components)

```bash
# Run on your machine with Go installed:
cd /home/kusa/code/magabot
go get -u ./...
go mod tidy
go build -o bin/magabot ./cmd/magabot
```

### Specific packages to update:
- `golang.org/x/crypto` → latest (currently 2021, has CVEs)
- `golang.org/x/net` → latest (HTTP/2 vulnerabilities)
- `golang.org/x/sys` → latest
- `golang.org/x/text` → v0.14+
- `google.golang.org/protobuf` → v1.32+
- `gorilla/websocket` → v1.5.1+

---

## Priority 2: Authentication Hardening (A07)

### Add session timeout
```go
// internal/security/session.go
type Session struct {
    UserID    string
    CreatedAt time.Time
    ExpiresAt time.Time
    LastSeen  time.Time
}

const (
    SessionTimeout = 24 * time.Hour
    IdleTimeout    = 4 * time.Hour
)

func (s *Session) IsValid() bool {
    now := time.Now()
    return now.Before(s.ExpiresAt) && 
           now.Sub(s.LastSeen) < IdleTimeout
}
```

### Add rate limit for failed auth
```go
// Track failed attempts per IP/user
type AuthAttempts struct {
    failures map[string][]time.Time
    mu       sync.Mutex
}

func (a *AuthAttempts) RecordFailure(key string) {
    // Lock after 5 failures in 15 minutes
}
```

---

## Priority 3: Security Logging (A09)

### Events to log:
- [ ] Failed authentication attempts
- [ ] Admin privilege changes
- [ ] Config modifications
- [ ] Rate limit triggers
- [ ] Encryption/decryption errors

### Add structured security log
```go
// internal/security/audit.go
type SecurityEvent struct {
    Timestamp   time.Time `json:"ts"`
    EventType   string    `json:"event"`
    UserID      string    `json:"user_id,omitempty"`  // hashed
    Platform    string    `json:"platform,omitempty"`
    IP          string    `json:"ip,omitempty"`
    Success     bool      `json:"success"`
    Details     string    `json:"details,omitempty"`
}

func LogSecurityEvent(event SecurityEvent) {
    // Write to separate security.log
    // Consider sending alerts for critical events
}
```

---

## Priority 4: SSRF Prevention (A10)

### URL allowlist for scraper/browser
```go
// internal/tools/urlvalidator.go
var (
    blockedHosts = []string{
        "localhost",
        "127.0.0.1",
        "::1",
        "169.254.",      // Link-local
        "10.",           // Private
        "172.16.",       // Private
        "192.168.",      // Private
        "metadata.",     // Cloud metadata
    }
    
    blockedSchemes = []string{
        "file",
        "ftp",
        "gopher",
    }
)

func ValidateURL(rawURL string) error {
    u, err := url.Parse(rawURL)
    if err != nil {
        return err
    }
    
    // Check scheme
    if !slices.Contains([]string{"http", "https"}, u.Scheme) {
        return errors.New("only http/https allowed")
    }
    
    // Resolve and check IP
    ips, err := net.LookupIP(u.Hostname())
    if err != nil {
        return err
    }
    
    for _, ip := range ips {
        if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
            return errors.New("internal IPs not allowed")
        }
    }
    
    return nil
}
```

---

## Updated OWASP Scores After Fixes

| # | Category | Before | After | Status |
|---|----------|:------:|:-----:|--------|
| A01 | Broken Access Control | 10 | 10 | ✅ |
| A02 | Cryptographic Failures | 10 | 10 | ✅ |
| A03 | Injection | 10 | 10 | ✅ |
| A04 | Insecure Design | 9 | 10 | Add threat model |
| A05 | Security Misconfiguration | 9 | 10 | Secure defaults checker |
| A06 | Vulnerable Components | 7 | 10 | **Update deps** |
| A07 | Auth Failures | 8 | 10 | **Session timeout** |
| A08 | Data Integrity | 9 | 10 | Add HMAC verification |
| A09 | Logging Failures | 8 | 10 | **Security events** |
| A10 | SSRF | 9 | 10 | **URL validation** |

---

## Quick Win Checklist

- [ ] `go get -u ./...` - Update all dependencies
- [ ] Add `ValidateURL()` to scraper and browser tools
- [ ] Add `SessionTimeout` checking
- [ ] Add `LogSecurityEvent()` for failed auth
- [ ] Run `go mod tidy` after updates
- [ ] Rebuild binary: `go build -o bin/magabot ./cmd/magabot`

---

## Testing Security

```bash
# Check for known vulnerabilities
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...

# Static analysis
go install honnef.co/go/tools/cmd/staticcheck@latest
staticcheck ./...

# Security linter
go install github.com/securego/gosec/v2/cmd/gosec@latest
gosec ./...
```

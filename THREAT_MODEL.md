# Threat Model - Magabot

## Overview

Magabot is a self-hosted multi-platform chatbot with LLM integration. This document identifies threats, attack surfaces, and mitigations.

## Assets

| Asset | Sensitivity | Description |
|-------|-------------|-------------|
| API Keys | **Critical** | LLM provider keys, platform tokens |
| User Messages | **High** | Chat history, personal data |
| Config File | **High** | Contains encrypted secrets |
| SQLite Database | **High** | Encrypted messages, sessions |
| Encryption Key | **Critical** | Master key for AES-256-GCM |

## Trust Boundaries

```
┌─────────────────────────────────────────────────────────────┐
│                     UNTRUSTED ZONE                          │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐        │
│  │ Internet│  │ Users   │  │ Websites│  │ LLM APIs│        │
│  └────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘        │
└───────┼────────────┼────────────┼────────────┼──────────────┘
        │            │            │            │
════════╪════════════╪════════════╪════════════╪══════════════
        │   TRUST BOUNDARY (TLS + Auth)        │
════════╪════════════╪════════════╪════════════╪══════════════
        │            │            │            │
┌───────▼────────────▼────────────▼────────────▼──────────────┐
│                     TRUSTED ZONE                            │
│  ┌──────────────────────────────────────────────────────┐  │
│  │                    MAGABOT                            │  │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐           │  │
│  │  │ Router   │  │ Security │  │ Storage  │           │  │
│  │  │ Auth     │  │ Vault    │  │ SQLite   │           │  │
│  │  │ RateLimit│  │ Session  │  │ Encrypted│           │  │
│  │  └──────────┘  └──────────┘  └──────────┘           │  │
│  └──────────────────────────────────────────────────────┘  │
│                                                             │
│  ┌──────────────────────────────────────────────────────┐  │
│  │                 FILE SYSTEM                           │  │
│  │  ~/.magabot/                                          │  │
│  │  ├── config.yaml (0600)                              │  │
│  │  └── data/magabot.db (0600)                          │  │
│  └──────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

## Threat Categories (STRIDE)

### 1. Spoofing Identity

| Threat | Likelihood | Impact | Mitigation |
|--------|:----------:|:------:|------------|
| Attacker impersonates allowed user | Medium | High | Platform-native auth (Telegram user ID, etc.) |
| Stolen API token reuse | Low | Critical | Tokens stored encrypted, file permissions 0600 |
| Session hijacking | Low | High | Sessions bound to platform+userID, timeout enforced |

### 2. Tampering

| Threat | Likelihood | Impact | Mitigation |
|--------|:----------:|:------:|------------|
| Config file modification | Low | Critical | File permissions 0600, HMAC validation |
| Database tampering | Low | High | SQLite encryption, secure_delete ON |
| Man-in-the-middle | Low | High | TLS 1.3 for all API calls |

### 3. Repudiation

| Threat | Likelihood | Impact | Mitigation |
|--------|:----------:|:------:|------------|
| User denies sending message | Medium | Low | Audit logging with hashed user IDs |
| Admin denies config change | Medium | Medium | Config changes logged in security.log |

### 4. Information Disclosure

| Threat | Likelihood | Impact | Mitigation |
|--------|:----------:|:------:|------------|
| Secrets in logs | Medium | Critical | User IDs hashed, secrets masked |
| Database leak | Low | High | AES-256-GCM encryption at rest |
| Memory dump | Low | Medium | Sensitive data zeroed after use |
| Error messages leak info | Medium | Medium | Generic error messages to users |

### 5. Denial of Service

| Threat | Likelihood | Impact | Mitigation |
|--------|:----------:|:------:|------------|
| Message flood | High | Medium | Rate limiting (msgs/min, cmds/min) |
| Resource exhaustion | Medium | Medium | Timeouts on all operations |
| Account lockout abuse | Low | Low | Lockout based on user key, not IP |

### 6. Elevation of Privilege

| Threat | Likelihood | Impact | Mitigation |
|--------|:----------:|:------:|------------|
| User becomes admin | Low | Critical | Explicit admin hierarchy, allowlist-first |
| Tool escapes sandbox | Low | High | URL validation, no file:// scheme |
| LLM prompt injection | Medium | Medium | Input sanitization, output limits |

## Attack Surfaces

### 1. Platform Adapters (Telegram, Discord, etc.)

**Entry Points:**
- Incoming messages
- Webhooks
- Bot commands

**Mitigations:**
- ✅ Allowlist-based access control
- ✅ Rate limiting per user
- ✅ Input sanitization
- ✅ Message size limits

### 2. Web Scraper / Browser

**Entry Points:**
- User-provided URLs
- Redirect chains
- JavaScript execution

**Mitigations:**
- ✅ URL validation (SSRF prevention)
- ✅ Block internal IPs (127.0.0.1, 10.x, 192.168.x)
- ✅ Block cloud metadata endpoints
- ✅ Timeout enforcement
- ✅ Headless browser isolation

### 3. LLM Integration

**Entry Points:**
- User prompts
- Tool responses
- System prompts

**Mitigations:**
- ✅ Input length limits
- ✅ Output truncation
- ✅ API key encryption
- ⚠️ Consider: prompt injection detection

### 4. Storage Layer

**Entry Points:**
- SQLite database
- Config file
- Log files

**Mitigations:**
- ✅ AES-256-GCM encryption for content
- ✅ Parameterized SQL queries
- ✅ secure_delete pragma
- ✅ File permissions 0600/0700

### 5. CLI Interface

**Entry Points:**
- Command arguments
- Config editing

**Mitigations:**
- ✅ Input validation
- ✅ Privilege separation (user vs root)
- ✅ Audit logging for admin actions

## Data Flow Security

```
User Message
     │
     ▼
┌─────────────┐
│ Platform    │ ── TLS 1.3 ──▶ Telegram/Discord/etc.
│ Adapter     │
└─────────────┘
     │
     ▼ (plaintext in memory only)
┌─────────────┐
│ Router      │
│ - Auth      │ ◀── Allowlist check
│ - RateLimit │ ◀── Sliding window
│ - Session   │ ◀── Timeout check
└─────────────┘
     │
     ▼ (encrypted)
┌─────────────┐
│ Storage     │ ── AES-256-GCM ──▶ SQLite
└─────────────┘
     │
     ▼ (plaintext in memory only)
┌─────────────┐
│ LLM Handler │ ── TLS 1.3 ──▶ Anthropic/OpenAI/etc.
└─────────────┘
     │
     ▼
Response (encrypted before storage)
```

## Security Controls Summary

| Control | Status | Implementation |
|---------|:------:|----------------|
| Encryption at rest | ✅ | AES-256-GCM |
| Encryption in transit | ✅ | TLS 1.3 |
| Authentication | ✅ | Platform-native + allowlist |
| Authorization | ✅ | Admin hierarchy |
| Rate limiting | ✅ | Per-user sliding window |
| Session management | ✅ | Timeout + lockout |
| Input validation | ✅ | Sanitization + URL validation |
| Audit logging | ✅ | security.log with rotation |
| Secure defaults | ✅ | Config validator |
| Dependency updates | ✅ | 2024 versions |

## Residual Risks

| Risk | Severity | Acceptance |
|------|:--------:|------------|
| Platform API compromise | High | Accept - out of scope |
| LLM provider data retention | Medium | Accept - user choice |
| Local root compromise | Critical | Accept - OS-level security |
| Zero-day in Go runtime | Low | Accept - update promptly |

## Review Schedule

- **Quarterly:** Review threat model for new features
- **On Release:** Security checklist before each release
- **On Incident:** Update mitigations based on findings

---

*Last Updated: 2026-02-06*
*Next Review: 2026-05-06*

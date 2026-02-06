# 🔐 Security Policy

Magabot is designed with security as a core principle, not an afterthought.

## Security Features

### Encryption

| What | How | Details |
|------|-----|---------|
| **API Keys** | AES-256-GCM | Stored encrypted, never logged |
| **Messages** | AES-256-GCM | Chat history encrypted in SQLite |
| **Sessions** | AES-256-GCM | Platform session data encrypted |
| **Config** | File permissions | 0600 (owner read/write only) |
| **Transport** | TLS 1.3 | All external API calls |

### Access Control

- **Allowlist mode** (default): Only explicitly allowed users can interact
- **Denylist mode**: Everyone except blocked users
- **Open mode**: Everyone (not recommended for production)

### Hierarchy

```
Global Admin → Platform Admin → Allowed User → Blocked
     ↓              ↓               ↓            ↓
  Full access   Platform only   Chat only     Ignored
```

### SQLite Security

```sql
PRAGMA secure_delete = ON;     -- Overwrite deleted data
PRAGMA auto_vacuum = INCREMENTAL;
PRAGMA temp_store = MEMORY;    -- No temp files on disk
```

### Input Validation

- All SQL queries use parameterized statements
- User input sanitized (control chars removed)
- Filenames validated before use
- Path traversal prevented

### Audit Logging

All admin actions are logged with:
- Timestamp
- Hashed user ID (privacy)
- Action performed
- IP address (if available)

## Security Checklist

### Before Deployment

- [ ] Generate unique encryption key: `magabot genkey`
- [ ] Set restrictive file permissions
- [ ] Add yourself as global admin first
- [ ] Enable only needed platforms
- [ ] Review allowlist before going public

### File Permissions

```bash
chmod 700 ~/.magabot
chmod 600 ~/.magabot/config.yaml
chmod 700 ~/.magabot/data
chmod 600 ~/.magabot/data/*.db
```

### Environment Variables

Never commit secrets. Use environment variables:

```bash
export MAGABOT_TELEGRAM_TOKEN="your-token"
export MAGABOT_ANTHROPIC_KEY="your-key"
```

## Reporting Vulnerabilities

If you discover a security vulnerability:

1. **DO NOT** open a public issue
2. Email: security@example.com (replace with your email)
3. Include:
   - Description of the vulnerability
   - Steps to reproduce
   - Potential impact
   - Suggested fix (if any)

We will respond within 48 hours and work on a fix.

## Security Updates

- Check for updates regularly: `magabot update check`
- Subscribe to security advisories on GitHub
- Follow semantic versioning: patch releases for security fixes

## Known Limitations

1. **WhatsApp**: Session stored locally (browser-based auth)
2. **Memory**: Stored as JSON files (encrypt disk for extra security)
3. **Logs**: May contain message metadata (not content)

## Best Practices

### For Personal Use

```yaml
access:
  mode: allowlist
  global_admins:
    - "YOUR_ID"
```

### For Team Use

```yaml
access:
  mode: allowlist
  global_admins:
    - "OWNER_ID"

platforms:
  discord:
    admins: ["ADMIN1", "ADMIN2"]
    allowed_chats: ["GUILD_ID"]
```

### For Public Bots

Not recommended without additional rate limiting and monitoring.

## Compliance

Magabot helps with:

- **GDPR**: Encrypted storage, secure delete, data export
- **Data Residency**: Self-hosted, data stays on your server
- **Audit Trail**: All actions logged

## Contact

For security-related questions: Create a private security advisory on GitHub.

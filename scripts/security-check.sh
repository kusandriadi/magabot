#!/bin/bash
# Security check script for OWASP Top 10 compliance
# Run: ./scripts/security-check.sh

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "🔒 Magabot Security Check"
echo "========================="
echo ""

# A01: Broken Access Control
echo "A01: Checking Access Control..."
if grep -rq "IsAuthorized\|IsAllowed\|IsPlatformAdmin" internal/; then
    echo -e "${GREEN}✓ Access control functions found${NC}"
else
    echo -e "${RED}✗ Access control not implemented${NC}"
fi

# A02: Cryptographic Failures  
echo "A02: Checking Cryptography..."
if grep -rq "aes.NewCipher\|cipher.NewGCM" internal/security/; then
    echo -e "${GREEN}✓ AES-GCM encryption found${NC}"
else
    echo -e "${RED}✗ Encryption not implemented${NC}"
fi

# A03: Injection
echo "A03: Checking for Injection vulnerabilities..."
# Check for string concatenation with variables in SQL (not hardcoded strings)
UNSAFE_SQL=$(grep -rn 'db.Exec.*".*" *\+ *[a-z]' internal/ 2>/dev/null | grep -v "_test.go" | wc -l)
if [ "$UNSAFE_SQL" -eq 0 ]; then
    echo -e "${GREEN}✓ No SQL string concatenation with variables${NC}"
else
    echo -e "${RED}✗ Found $UNSAFE_SQL potential SQL injection points${NC}"
fi
# Check all queries use parameterized ?
PARAM_QUERIES=$(grep -rn 'db.Exec\|db.Query' internal/ 2>/dev/null | grep -v "_test.go" | grep -c '\?')
echo -e "${GREEN}✓ Found $PARAM_QUERIES parameterized queries${NC}"

# A04: Insecure Design
echo "A04: Checking Secure Defaults..."
if grep -rq 'Mode.*=.*"allowlist"' internal/; then
    echo -e "${GREEN}✓ Allowlist mode default found${NC}"
else
    echo -e "${YELLOW}⚠ Check if allowlist is default${NC}"
fi

# A05: Security Misconfiguration
echo "A05: Checking Config Validation..."
if [ -f "internal/security/configvalidator.go" ]; then
    echo -e "${GREEN}✓ Config validator found${NC}"
else
    echo -e "${RED}✗ Config validator not found${NC}"
fi

# A06: Vulnerable Components
echo "A06: Checking Dependencies..."
if command -v govulncheck &> /dev/null; then
    echo "Running govulncheck..."
    govulncheck ./... 2>/dev/null || echo -e "${YELLOW}⚠ govulncheck found issues or failed${NC}"
else
    echo -e "${YELLOW}⚠ govulncheck not installed. Run: go install golang.org/x/vuln/cmd/govulncheck@latest${NC}"
fi

# A07: Auth Failures
echo "A07: Checking Session Management..."
if [ -f "internal/security/session.go" ]; then
    echo -e "${GREEN}✓ Session management found${NC}"
else
    echo -e "${RED}✗ Session management not found${NC}"
fi

# A08: Data Integrity
echo "A08: Checking Data Integrity..."
if grep -rq "hmac.New\|HMAC" internal/security/; then
    echo -e "${GREEN}✓ HMAC integrity checks found${NC}"
else
    echo -e "${RED}✗ HMAC not implemented${NC}"
fi

# A09: Security Logging
echo "A09: Checking Audit Logging..."
if [ -f "internal/security/audit.go" ]; then
    echo -e "${GREEN}✓ Audit logger found${NC}"
else
    echo -e "${RED}✗ Audit logger not found${NC}"
fi

# A10: SSRF
echo "A10: Checking SSRF Prevention..."
if [ -f "internal/security/urlvalidator.go" ]; then
    echo -e "${GREEN}✓ URL validator found${NC}"
else
    echo -e "${RED}✗ URL validator not found${NC}"
fi

echo ""
echo "========================="
echo "🔒 Security check complete"

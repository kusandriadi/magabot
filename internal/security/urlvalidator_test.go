package security

import (
	"testing"
)

func TestNewURLValidator(t *testing.T) {
	v := NewURLValidator()
	if v == nil {
		t.Fatal("URLValidator should not be nil")
	}
	if v.allowPrivateIPs {
		t.Error("allowPrivateIPs should default to false")
	}
	if len(v.allowedSchemes) == 0 {
		t.Error("allowedSchemes should have defaults")
	}
}

func TestURLValidatorWithAllowedDomains(t *testing.T) {
	v := NewURLValidator().WithAllowedDomains([]string{"example.com", "test.com"})
	if len(v.allowedDomains) != 2 {
		t.Errorf("Expected 2 allowed domains, got %d", len(v.allowedDomains))
	}
}

func TestURLValidatorWithBlockedDomains(t *testing.T) {
	v := NewURLValidator().WithBlockedDomains([]string{"blocked.com"})
	if len(v.blockedDomains) != 1 {
		t.Errorf("Expected 1 blocked domain, got %d", len(v.blockedDomains))
	}
}

func TestURLValidatorWithAllowPrivateIPs(t *testing.T) {
	v := NewURLValidator().WithAllowPrivateIPs(true)
	if !v.allowPrivateIPs {
		t.Error("allowPrivateIPs should be true")
	}
}

func TestURLValidatorValidate(t *testing.T) {
	v := NewURLValidator()

	t.Run("ValidHTTPS", func(t *testing.T) {
		err := v.Validate("https://www.google.com")
		if err != nil {
			t.Errorf("Valid HTTPS should pass: %v", err)
		}
	})

	t.Run("ValidHTTP", func(t *testing.T) {
		err := v.Validate("http://example.com")
		if err != nil {
			t.Errorf("Valid HTTP should pass: %v", err)
		}
	})

	t.Run("InvalidURL", func(t *testing.T) {
		err := v.Validate("not a url")
		// No scheme means it might parse but fail on scheme check
		if err == nil {
			t.Error("Expected error for invalid URL")
		}
	})

	t.Run("BlockedScheme_File", func(t *testing.T) {
		err := v.Validate("file:///etc/passwd")
		if err != ErrBlockedScheme {
			t.Errorf("Expected ErrBlockedScheme for file://, got %v", err)
		}
	})

	t.Run("BlockedScheme_FTP", func(t *testing.T) {
		err := v.Validate("ftp://ftp.example.com/file")
		if err != ErrBlockedScheme {
			t.Errorf("Expected ErrBlockedScheme for ftp://, got %v", err)
		}
	})

	t.Run("BlockedScheme_Gopher", func(t *testing.T) {
		err := v.Validate("gopher://example.com")
		if err != ErrBlockedScheme {
			t.Errorf("Expected ErrBlockedScheme for gopher://, got %v", err)
		}
	})
}

func TestURLValidatorBlockedHosts(t *testing.T) {
	v := NewURLValidator()

	blockedHosts := []string{
		"http://localhost/",
		"http://127.0.0.1/",
		"http://0.0.0.0/",
		"http://169.254.169.254/latest/meta-data/", // AWS metadata
		"http://metadata.google.internal/",         // GCP metadata
	}

	for _, url := range blockedHosts {
		t.Run(url, func(t *testing.T) {
			err := v.Validate(url)
			if err != ErrBlockedHost && err != ErrPrivateIP {
				t.Errorf("Expected blocked error for %s, got %v", url, err)
			}
		})
	}
}

func TestURLValidatorPrivateIPs(t *testing.T) {
	v := NewURLValidator()

	privateURLs := []string{
		"http://10.0.0.1/",
		"http://172.16.0.1/",
		"http://192.168.1.1/",
		"http://[::1]/",
		"http://[fc00::1]/",
	}

	for _, url := range privateURLs {
		t.Run(url, func(t *testing.T) {
			err := v.Validate(url)
			if err != ErrPrivateIP && err != ErrBlockedHost {
				t.Errorf("Expected private IP error for %s, got %v", url, err)
			}
		})
	}
}

func TestURLValidatorAllowPrivateIPs(t *testing.T) {
	v := NewURLValidator().WithAllowPrivateIPs(true)

	// Verify the flag is set correctly
	if !v.allowPrivateIPs {
		t.Error("allowPrivateIPs should be true")
	}
}

func TestURLValidatorBlockedKeywords(t *testing.T) {
	v := NewURLValidator()

	blockedURLs := []string{
		"http://example.com/..%2f..%2fetc/passwd",
		"http://example.com/..%5c..%5cwindows",
		"http://example.com/path%00null",
		"http://user@example.com/path", // @ for credential injection
	}

	for _, url := range blockedURLs {
		t.Run(url, func(t *testing.T) {
			err := v.Validate(url)
			if err != ErrBlockedKeyword {
				t.Errorf("Expected ErrBlockedKeyword for %s, got %v", url, err)
			}
		})
	}
}

func TestURLValidatorInvalidPort(t *testing.T) {
	v := NewURLValidator()

	err := v.Validate("http://example.com:22/")
	if err != ErrInvalidPort {
		t.Errorf("Expected ErrInvalidPort for port 22, got %v", err)
	}
}

func TestURLValidatorAllowedPorts(t *testing.T) {
	v := NewURLValidator()

	allowedURLs := []string{
		"http://example.com/", // default (80)
		"http://example.com:80/",
		"https://example.com/", // default (443)
		"https://example.com:443/",
		"http://example.com:8080/",
		"https://example.com:8443/",
		"http://example.com:3000/",
		"http://example.com:5000/",
		"http://example.com:8000/",
		"http://example.com:9000/",
	}

	for _, url := range allowedURLs {
		t.Run(url, func(t *testing.T) {
			err := v.Validate(url)
			if err == ErrInvalidPort {
				t.Errorf("Port should be allowed for %s", url)
			}
		})
	}
}

func TestURLValidatorAllowedDomains(t *testing.T) {
	v := NewURLValidator().WithAllowedDomains([]string{"trusted.com"})

	t.Run("AllowedDomain", func(t *testing.T) {
		err := v.Validate("https://trusted.com/path")
		if err != nil {
			t.Errorf("Allowed domain should pass: %v", err)
		}
	})

	t.Run("AllowedSubdomain", func(t *testing.T) {
		// Note: May fail DNS resolution in test environment
		// The domain check itself is tested, DNS resolution is external
		err := v.Validate("https://api.trusted.com/path")
		// Accept either success or DNS resolution failure
		if err != nil && err != ErrResolveFailed {
			t.Errorf("Expected pass or ErrResolveFailed, got: %v", err)
		}
	})

	t.Run("NotAllowedDomain", func(t *testing.T) {
		err := v.Validate("https://untrusted.com/path")
		if err != ErrBlockedHost {
			t.Errorf("Expected ErrBlockedHost for untrusted domain, got %v", err)
		}
	})
}

func TestURLValidatorBlockedDomains(t *testing.T) {
	v := NewURLValidator().WithBlockedDomains([]string{"blocked.com"})

	t.Run("BlockedDomain", func(t *testing.T) {
		err := v.Validate("https://blocked.com/path")
		if err != ErrBlockedHost {
			t.Errorf("Expected ErrBlockedHost for blocked domain, got %v", err)
		}
	})

	t.Run("BlockedSubdomain", func(t *testing.T) {
		err := v.Validate("https://sub.blocked.com/path")
		if err != ErrBlockedHost {
			t.Errorf("Expected ErrBlockedHost for subdomain of blocked domain, got %v", err)
		}
	})

	t.Run("NotBlockedDomain", func(t *testing.T) {
		err := v.Validate("https://example.com/path")
		if err == ErrBlockedHost {
			t.Error("Not-blocked domain should pass")
		}
	})
}

func TestURLValidatorResolveFailed(t *testing.T) {
	v := NewURLValidator()

	err := v.Validate("http://this-domain-definitely-does-not-exist-12345.com/")
	if err != ErrResolveFailed {
		t.Errorf("Expected ErrResolveFailed for nonexistent domain, got %v", err)
	}
}

func TestValidateURL(t *testing.T) {
	// Test convenience function
	err := ValidateURL("https://www.google.com")
	if err != nil {
		t.Errorf("ValidateURL should pass for valid URL: %v", err)
	}

	err = ValidateURL("file:///etc/passwd")
	if err == nil {
		t.Error("ValidateURL should fail for file://")
	}
}

func TestMustValidateURL(t *testing.T) {
	// Should not panic for valid URL
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("MustValidateURL panicked for valid URL: %v", r)
			}
		}()
		MustValidateURL("https://www.google.com")
	}()

	// Should panic for invalid URL
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Error("MustValidateURL should panic for invalid URL")
			}
		}()
		MustValidateURL("file:///etc/passwd")
	}()
}

func TestURLValidatorIsAllowedScheme(t *testing.T) {
	v := NewURLValidator()

	allowed := []string{"http", "https", "HTTP", "HTTPS"}
	for _, scheme := range allowed {
		if !v.isAllowedScheme(scheme) {
			t.Errorf("Scheme %s should be allowed", scheme)
		}
	}

	blocked := []string{"file", "ftp", "gopher", "dict", "ldap"}
	for _, scheme := range blocked {
		if v.isAllowedScheme(scheme) {
			t.Errorf("Scheme %s should be blocked", scheme)
		}
	}
}

func TestURLValidatorValidateIP(t *testing.T) {
	v := NewURLValidator()

	t.Run("PublicIP", func(t *testing.T) {
		// We can't easily test public IPs without making network calls
		// Just verify the function doesn't panic
	})

	t.Run("IPv6Loopback", func(t *testing.T) {
		err := v.Validate("http://[::1]/")
		if err != ErrPrivateIP && err != ErrBlockedHost {
			t.Errorf("IPv6 loopback should be blocked, got %v", err)
		}
	})
}

func TestURLValidatorReservedIP(t *testing.T) {
	v := NewURLValidator()

	reservedURLs := []string{
		"http://100.64.0.1/",   // CGNAT
		"http://192.0.2.1/",    // TEST-NET-1
		"http://198.51.100.1/", // TEST-NET-2
		"http://203.0.113.1/",  // TEST-NET-3
	}

	for _, url := range reservedURLs {
		t.Run(url, func(t *testing.T) {
			err := v.Validate(url)
			if err != ErrPrivateIP {
				t.Errorf("Expected ErrPrivateIP for reserved IP %s, got %v", url, err)
			}
		})
	}
}

func TestURLValidatorErrors(t *testing.T) {
	// Verify all error messages are set
	errors := []error{
		ErrInvalidURL,
		ErrBlockedScheme,
		ErrBlockedHost,
		ErrPrivateIP,
		ErrInvalidPort,
		ErrResolveFailed,
		ErrBlockedKeyword,
	}

	for _, err := range errors {
		if err.Error() == "" {
			t.Errorf("Error %v should have message", err)
		}
	}
}

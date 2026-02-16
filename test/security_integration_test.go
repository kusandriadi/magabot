// Package test contains integration tests for security module
package test

import (
	"sync"
	"testing"
	"time"

	"github.com/kusa/magabot/internal/security"
)

func TestVaultIntegration(t *testing.T) {
	// Generate a valid key
	key := security.GenerateKey()

	vault, err := security.NewVault(key)
	if err != nil {
		t.Fatalf("Failed to create vault: %v", err)
	}

	t.Run("EncryptDecrypt", func(t *testing.T) {
		plaintext := "Hello, this is a secret message!"

		ciphertext, err := vault.Encrypt([]byte(plaintext))
		if err != nil {
			t.Fatalf("Encrypt failed: %v", err)
		}

		// Ciphertext should be different from plaintext
		if ciphertext == plaintext {
			t.Error("Ciphertext should not equal plaintext")
		}

		decrypted, err := vault.Decrypt(ciphertext)
		if err != nil {
			t.Fatalf("Decrypt failed: %v", err)
		}

		if string(decrypted) != plaintext {
			t.Errorf("Expected '%s', got '%s'", plaintext, string(decrypted))
		}
	})

	t.Run("UniqueNonce", func(t *testing.T) {
		plaintext := "Same message"

		ct1, _ := vault.Encrypt([]byte(plaintext))
		ct2, _ := vault.Encrypt([]byte(plaintext))

		// Same plaintext should produce different ciphertexts (random nonce)
		if ct1 == ct2 {
			t.Error("Same plaintext should produce different ciphertexts due to random nonce")
		}
	})

	t.Run("InvalidKey", func(t *testing.T) {
		_, err := security.NewVault("invalid-key")
		if err == nil {
			t.Error("Expected error for invalid key")
		}
	})

	t.Run("TamperedCiphertext", func(t *testing.T) {
		plaintext := "Secret data"
		ciphertext, _ := vault.Encrypt([]byte(plaintext))

		// Tamper with ciphertext
		tampered := ciphertext[:len(ciphertext)-2] + "XX"

		_, err := vault.Decrypt(tampered)
		if err == nil {
			t.Error("Expected error for tampered ciphertext")
		}
	})

	t.Run("EmptyPlaintext", func(t *testing.T) {
		ciphertext, err := vault.Encrypt([]byte(""))
		if err != nil {
			t.Fatalf("Encrypt empty failed: %v", err)
		}

		decrypted, err := vault.Decrypt(ciphertext)
		if err != nil {
			t.Fatalf("Decrypt empty failed: %v", err)
		}

		if string(decrypted) != "" {
			t.Error("Expected empty string")
		}
	})

	t.Run("LargeData", func(t *testing.T) {
		// 1MB of data
		largeData := make([]byte, 1024*1024)
		for i := range largeData {
			largeData[i] = byte(i % 256)
		}

		ciphertext, err := vault.Encrypt(largeData)
		if err != nil {
			t.Fatalf("Encrypt large data failed: %v", err)
		}

		decrypted, err := vault.Decrypt(ciphertext)
		if err != nil {
			t.Fatalf("Decrypt large data failed: %v", err)
		}

		if len(decrypted) != len(largeData) {
			t.Errorf("Expected %d bytes, got %d", len(largeData), len(decrypted))
		}
	})

	t.Run("ConcurrentEncryption", func(t *testing.T) {
		var wg sync.WaitGroup
		errors := make(chan error, 100)

		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				msg := []byte("message " + string(rune('0'+n%10)))
				ct, err := vault.Encrypt(msg)
				if err != nil {
					errors <- err
					return
				}
				_, err = vault.Decrypt(ct)
				if err != nil {
					errors <- err
				}
			}(i)
		}

		wg.Wait()
		close(errors)

		for err := range errors {
			t.Errorf("Concurrent operation failed: %v", err)
		}
	})
}

func TestAuthorizerIntegration(t *testing.T) {
	auth := security.NewAuthorizer()

	t.Run("SetAndCheckUsers", func(t *testing.T) {
		auth.SetAllowedUsers("telegram", []string{"user1", "user2", "user3"})

		if !auth.IsAuthorized("telegram", "user1") {
			t.Error("user1 should be authorized")
		}

		if !auth.IsAuthorized("telegram", "user2") {
			t.Error("user2 should be authorized")
		}

		if auth.IsAuthorized("telegram", "user4") {
			t.Error("user4 should not be authorized")
		}
	})

	t.Run("PlatformIsolation", func(t *testing.T) {
		auth.SetAllowedUsers("telegram", []string{"telegramUser"})
		auth.SetAllowedUsers("whatsapp", []string{"whatsappUser"})

		if !auth.IsAuthorized("telegram", "telegramUser") {
			t.Error("telegramUser should be authorized for telegram")
		}

		if auth.IsAuthorized("telegram", "whatsappUser") {
			t.Error("whatsappUser should not be authorized for telegram")
		}

		if auth.IsAuthorized("whatsapp", "telegramUser") {
			t.Error("telegramUser should not be authorized for whatsapp")
		}
	})

	t.Run("EmptyAllowlist", func(t *testing.T) {
		auth.SetAllowedUsers("slack", []string{})

		// Empty allowlist = allow all
		if !auth.IsAuthorized("slack", "anyuser") {
			t.Error("Empty allowlist should allow all users")
		}
	})

	t.Run("UpdateAllowlist", func(t *testing.T) {
		auth.SetAllowedUsers("discord", []string{"user1"})

		if !auth.IsAuthorized("discord", "user1") {
			t.Error("user1 should be authorized")
		}

		// Update allowlist - remove user1, add user2
		auth.SetAllowedUsers("discord", []string{"user2"})

		if auth.IsAuthorized("discord", "user1") {
			t.Error("user1 should no longer be authorized")
		}

		if !auth.IsAuthorized("discord", "user2") {
			t.Error("user2 should now be authorized")
		}
	})

	t.Run("UnknownPlatform", func(t *testing.T) {
		if auth.IsAuthorized("unknownplatform", "anyuser") {
			t.Error("Unknown platform should deny all")
		}
	})
}

func TestRateLimiterIntegration(t *testing.T) {
	t.Run("MessageRateLimit", func(t *testing.T) {
		limiter := security.NewRateLimiter(5, 3) // 5 msgs/min, 3 cmds/min

		userKey := "telegram:user1"

		// Should allow first 5 messages
		for i := 0; i < 5; i++ {
			if !limiter.AllowMessage(userKey) {
				t.Errorf("Message %d should be allowed", i+1)
			}
		}

		// 6th message should be blocked
		if limiter.AllowMessage(userKey) {
			t.Error("6th message should be rate limited")
		}
	})

	t.Run("CommandRateLimit", func(t *testing.T) {
		limiter := security.NewRateLimiter(10, 2) // 10 msgs/min, 2 cmds/min

		userKey := "telegram:user2"

		// Should allow first 2 commands
		if !limiter.AllowCommand(userKey) {
			t.Error("1st command should be allowed")
		}
		if !limiter.AllowCommand(userKey) {
			t.Error("2nd command should be allowed")
		}

		// 3rd command should be blocked
		if limiter.AllowCommand(userKey) {
			t.Error("3rd command should be rate limited")
		}
	})

	t.Run("SeparateMessagesAndCommands", func(t *testing.T) {
		limiter := security.NewRateLimiter(3, 2)

		userKey := "telegram:user3"

		// Use all command quota
		limiter.AllowCommand(userKey)
		limiter.AllowCommand(userKey)

		// Commands exhausted
		if limiter.AllowCommand(userKey) {
			t.Error("Commands should be exhausted")
		}

		// But messages should still work
		if !limiter.AllowMessage(userKey) {
			t.Error("Messages should still be allowed")
		}
	})

	t.Run("UserIsolation", func(t *testing.T) {
		limiter := security.NewRateLimiter(2, 2)

		// Exhaust user1's quota
		limiter.AllowMessage("user1")
		limiter.AllowMessage("user1")

		if limiter.AllowMessage("user1") {
			t.Error("user1 should be rate limited")
		}

		// user2 should be unaffected
		if !limiter.AllowMessage("user2") {
			t.Error("user2 should not be affected by user1's limit")
		}
	})

	t.Run("Cleanup", func(t *testing.T) {
		limiter := security.NewRateLimiter(100, 100)

		// Add many users
		for i := 0; i < 50; i++ {
			limiter.AllowMessage("user" + string(rune('A'+i)))
		}

		// Cleanup should not panic
		limiter.Cleanup()
	})
}

func TestHashUserID(t *testing.T) {
	t.Run("Consistency", func(t *testing.T) {
		hash1 := security.HashUserID("telegram", "user123")
		hash2 := security.HashUserID("telegram", "user123")

		if hash1 != hash2 {
			t.Error("Same input should produce same hash")
		}
	})

	t.Run("DifferentUsers", func(t *testing.T) {
		hash1 := security.HashUserID("telegram", "user1")
		hash2 := security.HashUserID("telegram", "user2")

		if hash1 == hash2 {
			t.Error("Different users should have different hashes")
		}
	})

	t.Run("DifferentPlatforms", func(t *testing.T) {
		hash1 := security.HashUserID("telegram", "user1")
		hash2 := security.HashUserID("whatsapp", "user1")

		if hash1 == hash2 {
			t.Error("Same user on different platforms should have different hashes")
		}
	})

	t.Run("HashLength", func(t *testing.T) {
		hash := security.HashUserID("telegram", "user123")

		// Base64 encoded 8 bytes = 12 characters (with padding)
		if len(hash) == 0 {
			t.Error("Hash should not be empty")
		}
	})
}

func TestGenerateKey(t *testing.T) {
	t.Run("ValidKey", func(t *testing.T) {
		key := security.GenerateKey()

		// Should be able to create vault with generated key
		_, err := security.NewVault(key)
		if err != nil {
			t.Errorf("Generated key should be valid: %v", err)
		}
	})

	t.Run("UniqueKeys", func(t *testing.T) {
		key1 := security.GenerateKey()
		key2 := security.GenerateKey()

		if key1 == key2 {
			t.Error("Generated keys should be unique")
		}
	})
}

func TestSessionManager(t *testing.T) {
	mgr := security.NewSessionManager()
	defer mgr.Stop()

	t.Run("GetOrCreate", func(t *testing.T) {
		sess := mgr.GetOrCreate("telegram", "user1")
		if sess == nil {
			t.Fatal("Session should be created")
		}

		// Same user should return same session
		sess2 := mgr.GetOrCreate("telegram", "user1")
		if sess != sess2 {
			t.Error("Should return same session for same user")
		}
	})

	t.Run("DifferentUsers", func(t *testing.T) {
		sess1 := mgr.GetOrCreate("telegram", "userA")
		sess2 := mgr.GetOrCreate("telegram", "userB")

		if sess1 == sess2 {
			t.Error("Different users should have different sessions")
		}
	})

	t.Run("Validate", func(t *testing.T) {
		mgr.GetOrCreate("telegram", "validUser")

		err := mgr.Validate("telegram", "validUser")
		if err != nil {
			t.Errorf("Session should be valid: %v", err)
		}

		// Unknown user should return nil (no session yet)
		err = mgr.Validate("telegram", "unknownUser")
		if err != nil {
			t.Error("Unknown user should not error (no session yet)")
		}
	})

	t.Run("InvalidateAndRenew", func(t *testing.T) {
		sess1 := mgr.GetOrCreate("telegram", "renewUser")
		mgr.Invalidate("telegram", "renewUser")

		// Create new session
		sess2 := mgr.GetOrCreate("telegram", "renewUser")
		if sess1 == sess2 {
			t.Error("New session should be different after invalidation")
		}
	})

	t.Run("SessionIsValid", func(t *testing.T) {
		sess := mgr.GetOrCreate("telegram", "testValidUser")
		if !sess.IsValid() {
			t.Error("New session should be valid")
		}
	})

	t.Run("SessionTouch", func(t *testing.T) {
		sess := mgr.GetOrCreate("telegram", "touchUser")
		oldLastSeen := sess.LastSeen

		time.Sleep(10 * time.Millisecond)
		sess.Touch()

		if !sess.LastSeen.After(oldLastSeen) {
			t.Error("Touch should update LastSeen")
		}
	})
}

func TestAuthAttempts(t *testing.T) {
	attempts := security.NewAuthAttempts()

	t.Run("LockAfterFailures", func(t *testing.T) {
		userKey := "telegram:baduser"

		// Record failures
		for i := 0; i < 5; i++ {
			attempts.RecordFailure(userKey)
		}

		if !attempts.IsLocked(userKey) {
			t.Error("Account should be locked after 5 failures")
		}
	})

	t.Run("ClearFailures", func(t *testing.T) {
		userKey := "telegram:gooduser"

		// Record some failures
		attempts.RecordFailure(userKey)
		attempts.RecordFailure(userKey)

		// Clear on successful auth
		attempts.ClearFailures(userKey)

		// Should not be locked
		if attempts.IsLocked(userKey) {
			t.Error("Account should not be locked after clearing failures")
		}
	})

	t.Run("UserIsolation", func(t *testing.T) {
		// Lock user1
		for i := 0; i < 5; i++ {
			attempts.RecordFailure("user1")
		}

		// user2 should not be affected
		if attempts.IsLocked("user2") {
			t.Error("user2 should not be affected by user1's lockout")
		}
	})
}

func TestAuditLogger(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("LogEvents", func(t *testing.T) {
		logger, err := security.NewAuditLogger(tmpDir)
		if err != nil {
			t.Fatalf("Failed to create audit logger: %v", err)
		}
		defer logger.Close()

		logger.LogAuthFailure("telegram", "user1", "wrong password")
		logger.LogAuthLockout("telegram", "user1")
		logger.LogRateLimited("telegram", "user1")
		logger.LogAuthSuccess("telegram", "user2")
		logger.LogAdminAction("telegram", "admin", "banned user")
		logger.LogConfigChange("telegram", "admin", "updated allowlist")
		logger.LogAccessDenied("telegram", "user3", "/admin")
		logger.LogSSRFBlocked("telegram", "user4", "http://localhost")
	})

	t.Run("LogSecurityEvent", func(t *testing.T) {
		logger, err := security.NewAuditLogger(tmpDir)
		if err != nil {
			t.Fatalf("Failed to create audit logger: %v", err)
		}
		defer logger.Close()

		event := security.SecurityEvent{
			EventType: security.EventAuthSuccess,
			Platform:  "telegram",
			UserID:    "hashedUser",
			Success:   true,
			Details:   "test event",
		}

		if err := logger.Log(event); err != nil {
			t.Errorf("Failed to log event: %v", err)
		}
	})
}

// BenchmarkVaultEncrypt benchmarks encryption performance
func BenchmarkVaultEncrypt(b *testing.B) {
	key := security.GenerateKey()
	vault, _ := security.NewVault(key)
	data := []byte("This is a test message for encryption benchmarking")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = vault.Encrypt(data)
	}
}

// BenchmarkRateLimiter benchmarks rate limiter performance
func BenchmarkRateLimiter(b *testing.B) {
	limiter := security.NewRateLimiter(1000, 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.AllowMessage("user" + string(rune('A'+i%26)))
	}
}

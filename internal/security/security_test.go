package security

import (
	"encoding/base64"
	"sync"
	"testing"
	"time"
)

func TestGenerateKey(t *testing.T) {
	t.Run("ValidKey", func(t *testing.T) {
		key := GenerateKey()
		if key == "" {
			t.Error("Generated key should not be empty")
		}

		// Should be base64 decodable
		decoded, err := base64.StdEncoding.DecodeString(key)
		if err != nil {
			t.Errorf("Key should be valid base64: %v", err)
		}

		// Should be 32 bytes
		if len(decoded) != 32 {
			t.Errorf("Key should be 32 bytes, got %d", len(decoded))
		}
	})

	t.Run("UniqueKeys", func(t *testing.T) {
		keys := make(map[string]bool)
		for i := 0; i < 100; i++ {
			key := GenerateKey()
			if keys[key] {
				t.Error("Generated duplicate key")
			}
			keys[key] = true
		}
	})
}

func TestNewVault(t *testing.T) {
	t.Run("ValidKey", func(t *testing.T) {
		key := GenerateKey()
		vault, err := NewVault(key)
		if err != nil {
			t.Fatalf("Failed to create vault: %v", err)
		}
		if vault == nil {
			t.Error("Vault should not be nil")
		}
	})

	t.Run("InvalidBase64", func(t *testing.T) {
		_, err := NewVault("not-valid-base64!!!")
		if err != ErrInvalidKey {
			t.Errorf("Expected ErrInvalidKey, got %v", err)
		}
	})

	t.Run("WrongKeyLength", func(t *testing.T) {
		// Valid base64 but wrong length
		shortKey := base64.StdEncoding.EncodeToString([]byte("short"))
		_, err := NewVault(shortKey)
		if err != ErrInvalidKey {
			t.Errorf("Expected ErrInvalidKey for short key, got %v", err)
		}
	})

	t.Run("EmptyKey", func(t *testing.T) {
		_, err := NewVault("")
		if err != ErrInvalidKey {
			t.Errorf("Expected ErrInvalidKey for empty key, got %v", err)
		}
	})
}

func TestVaultEncryptDecrypt(t *testing.T) {
	key := GenerateKey()
	vault, _ := NewVault(key)

	t.Run("BasicEncryptDecrypt", func(t *testing.T) {
		plaintext := []byte("Hello, World!")
		ciphertext, err := vault.Encrypt(plaintext)
		if err != nil {
			t.Fatalf("Encrypt failed: %v", err)
		}

		if ciphertext == string(plaintext) {
			t.Error("Ciphertext should differ from plaintext")
		}

		decrypted, err := vault.Decrypt(ciphertext)
		if err != nil {
			t.Fatalf("Decrypt failed: %v", err)
		}

		if string(decrypted) != string(plaintext) {
			t.Errorf("Expected '%s', got '%s'", plaintext, decrypted)
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

	t.Run("UniqueNonces", func(t *testing.T) {
		plaintext := []byte("Same message")
		ct1, _ := vault.Encrypt(plaintext)
		ct2, _ := vault.Encrypt(plaintext)

		if ct1 == ct2 {
			t.Error("Same plaintext should produce different ciphertexts")
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
			t.Fatalf("Encrypt large failed: %v", err)
		}

		decrypted, err := vault.Decrypt(ciphertext)
		if err != nil {
			t.Fatalf("Decrypt large failed: %v", err)
		}

		if len(decrypted) != len(largeData) {
			t.Errorf("Size mismatch: got %d, want %d", len(decrypted), len(largeData))
		}
	})

	t.Run("BinaryData", func(t *testing.T) {
		binaryData := []byte{0x00, 0x01, 0xFF, 0xFE, 0x00, 0x00}
		ciphertext, _ := vault.Encrypt(binaryData)
		decrypted, _ := vault.Decrypt(ciphertext)

		if string(decrypted) != string(binaryData) {
			t.Error("Binary data mismatch")
		}
	})
}

func TestVaultDecryptErrors(t *testing.T) {
	key := GenerateKey()
	vault, _ := NewVault(key)

	t.Run("InvalidBase64", func(t *testing.T) {
		_, err := vault.Decrypt("not-valid-base64!!!")
		if err == nil {
			t.Error("Should error on invalid base64")
		}
	})

	t.Run("TooShort", func(t *testing.T) {
		// Too short to contain nonce
		short := base64.StdEncoding.EncodeToString([]byte("short"))
		_, err := vault.Decrypt(short)
		if err != ErrDecryptFailed {
			t.Errorf("Expected ErrDecryptFailed, got %v", err)
		}
	})

	t.Run("TamperedCiphertext", func(t *testing.T) {
		ciphertext, _ := vault.Encrypt([]byte("secret"))
		// Tamper with it
		tampered := ciphertext[:len(ciphertext)-2] + "XX"
		_, err := vault.Decrypt(tampered)
		if err != ErrDecryptFailed {
			t.Errorf("Expected ErrDecryptFailed for tampered data, got %v", err)
		}
	})

	t.Run("WrongKey", func(t *testing.T) {
		// Encrypt with one key
		ciphertext, _ := vault.Encrypt([]byte("secret"))

		// Try to decrypt with different key
		otherKey := GenerateKey()
		otherVault, _ := NewVault(otherKey)
		_, err := otherVault.Decrypt(ciphertext)
		if err != ErrDecryptFailed {
			t.Errorf("Expected ErrDecryptFailed for wrong key, got %v", err)
		}
	})
}

func TestVaultConcurrency(t *testing.T) {
	key := GenerateKey()
	vault, _ := NewVault(key)

	var wg sync.WaitGroup
	errors := make(chan error, 200)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			plaintext := []byte("concurrent message")
			ct, err := vault.Encrypt(plaintext)
			if err != nil {
				errors <- err
				return
			}
			decrypted, err := vault.Decrypt(ct)
			if err != nil {
				errors <- err
				return
			}
			if string(decrypted) != string(plaintext) {
				errors <- ErrDecryptFailed
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent error: %v", err)
	}
}

func TestHashUserID(t *testing.T) {
	t.Run("Consistency", func(t *testing.T) {
		hash1 := HashUserID("telegram", "user123")
		hash2 := HashUserID("telegram", "user123")
		if hash1 != hash2 {
			t.Error("Same input should produce same hash")
		}
	})

	t.Run("DifferentUsers", func(t *testing.T) {
		hash1 := HashUserID("telegram", "user1")
		hash2 := HashUserID("telegram", "user2")
		if hash1 == hash2 {
			t.Error("Different users should have different hashes")
		}
	})

	t.Run("DifferentPlatforms", func(t *testing.T) {
		hash1 := HashUserID("telegram", "user1")
		hash2 := HashUserID("whatsapp", "user1")
		if hash1 == hash2 {
			t.Error("Different platforms should have different hashes")
		}
	})

	t.Run("NotEmpty", func(t *testing.T) {
		hash := HashUserID("telegram", "user")
		if hash == "" {
			t.Error("Hash should not be empty")
		}
	})

	t.Run("FixedLength", func(t *testing.T) {
		hash1 := HashUserID("a", "b")
		hash2 := HashUserID("platform", "very_long_user_id_12345")
		// Both should be same length (base64 of 8 bytes = 12 chars with padding)
		if len(hash1) != len(hash2) {
			t.Errorf("Hashes should have fixed length: %d vs %d", len(hash1), len(hash2))
		}
	})
}

func TestNewAuthorizer(t *testing.T) {
	auth := NewAuthorizer()
	if auth == nil {
		t.Error("Authorizer should not be nil")
	}
	if auth.allowedUsers == nil {
		t.Error("allowedUsers map should be initialized")
	}
}

func TestAuthorizerSetAllowedUsers(t *testing.T) {
	auth := NewAuthorizer()

	t.Run("SetUsers", func(t *testing.T) {
		auth.SetAllowedUsers("telegram", []string{"user1", "user2"})

		if !auth.IsAuthorized("telegram", "user1") {
			t.Error("user1 should be authorized")
		}
		if !auth.IsAuthorized("telegram", "user2") {
			t.Error("user2 should be authorized")
		}
	})

	t.Run("ReplaceUsers", func(t *testing.T) {
		auth.SetAllowedUsers("telegram", []string{"user3"})

		if auth.IsAuthorized("telegram", "user1") {
			t.Error("user1 should no longer be authorized")
		}
		if !auth.IsAuthorized("telegram", "user3") {
			t.Error("user3 should be authorized")
		}
	})

	t.Run("MultiplePlatforms", func(t *testing.T) {
		auth.SetAllowedUsers("telegram", []string{"tgUser"})
		auth.SetAllowedUsers("whatsapp", []string{"waUser"})

		if !auth.IsAuthorized("telegram", "tgUser") {
			t.Error("tgUser should be authorized on telegram")
		}
		if auth.IsAuthorized("telegram", "waUser") {
			t.Error("waUser should not be authorized on telegram")
		}
		if !auth.IsAuthorized("whatsapp", "waUser") {
			t.Error("waUser should be authorized on whatsapp")
		}
	})
}

func TestAuthorizerIsAuthorized(t *testing.T) {
	auth := NewAuthorizer()

	t.Run("UnknownPlatform", func(t *testing.T) {
		if auth.IsAuthorized("unknown", "user") {
			t.Error("Unknown platform should deny")
		}
	})

	t.Run("EmptyAllowlist", func(t *testing.T) {
		auth.SetAllowedUsers("slack", []string{})
		// Empty allowlist = allow all
		if !auth.IsAuthorized("slack", "anyuser") {
			t.Error("Empty allowlist should allow all")
		}
	})

	t.Run("UnauthorizedUser", func(t *testing.T) {
		auth.SetAllowedUsers("discord", []string{"allowed"})
		if auth.IsAuthorized("discord", "notallowed") {
			t.Error("Not allowed user should be denied")
		}
	})
}

func TestAuthorizerConcurrency(t *testing.T) {
	auth := NewAuthorizer()
	auth.SetAllowedUsers("telegram", []string{"user1", "user2", "user3"})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			auth.IsAuthorized("telegram", "user1")
		}()
		go func() {
			defer wg.Done()
			auth.SetAllowedUsers("telegram", []string{"user1", "user2"})
		}()
	}
	wg.Wait()
}

func TestNewRateLimiter(t *testing.T) {
	limiter := NewRateLimiter(10, 5)
	if limiter == nil {
		t.Error("RateLimiter should not be nil")
	}
	if limiter.maxMsg != 10 {
		t.Errorf("Expected maxMsg 10, got %d", limiter.maxMsg)
	}
	if limiter.maxCmd != 5 {
		t.Errorf("Expected maxCmd 5, got %d", limiter.maxCmd)
	}
}

func TestRateLimiterAllowMessage(t *testing.T) {
	limiter := NewRateLimiter(3, 2)

	t.Run("AllowWithinLimit", func(t *testing.T) {
		user := "msg_user_1"
		for i := 0; i < 3; i++ {
			if !limiter.AllowMessage(user) {
				t.Errorf("Message %d should be allowed", i+1)
			}
		}
	})

	t.Run("BlockExceedingLimit", func(t *testing.T) {
		user := "msg_user_2"
		for i := 0; i < 3; i++ {
			limiter.AllowMessage(user)
		}
		if limiter.AllowMessage(user) {
			t.Error("4th message should be blocked")
		}
	})

	t.Run("UserIsolation", func(t *testing.T) {
		// Exhaust user1
		for i := 0; i < 3; i++ {
			limiter.AllowMessage("isolated_user1")
		}

		// user2 should be unaffected
		if !limiter.AllowMessage("isolated_user2") {
			t.Error("Different user should not be affected")
		}
	})
}

func TestRateLimiterAllowCommand(t *testing.T) {
	limiter := NewRateLimiter(10, 2)

	t.Run("AllowWithinLimit", func(t *testing.T) {
		user := "cmd_user_1"
		if !limiter.AllowCommand(user) {
			t.Error("1st command should be allowed")
		}
		if !limiter.AllowCommand(user) {
			t.Error("2nd command should be allowed")
		}
	})

	t.Run("BlockExceedingLimit", func(t *testing.T) {
		user := "cmd_user_2"
		limiter.AllowCommand(user)
		limiter.AllowCommand(user)
		if limiter.AllowCommand(user) {
			t.Error("3rd command should be blocked")
		}
	})

	t.Run("SeparateFromMessages", func(t *testing.T) {
		user := "separate_user"
		// Exhaust commands
		limiter.AllowCommand(user)
		limiter.AllowCommand(user)

		// Messages should still work
		if !limiter.AllowMessage(user) {
			t.Error("Messages should not be affected by command limit")
		}
	})
}

func TestRateLimiterCleanup(t *testing.T) {
	limiter := NewRateLimiter(100, 100)

	// Add many users
	for i := 0; i < 50; i++ {
		limiter.AllowMessage("cleanup_user_" + string(rune('A'+i)))
	}

	// Cleanup should not panic
	limiter.Cleanup()

	// Should still work after cleanup
	if !limiter.AllowMessage("new_user") {
		t.Error("Should allow new user after cleanup")
	}
}

func TestRateLimiterPeriodicCleanup(t *testing.T) {
	limiter := NewRateLimiter(100, 100)

	// Trigger periodic cleanup (every 200 calls)
	for i := 0; i < 250; i++ {
		limiter.AllowMessage("periodic_user")
	}

	// Should not panic and should continue working
	if !limiter.AllowMessage("another_user") {
		t.Error("Should allow after periodic cleanup")
	}
}

func TestRateLimiterConcurrency(t *testing.T) {
	limiter := NewRateLimiter(1000, 100)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			user := "concurrent_" + string(rune('A'+n%26))
			limiter.AllowMessage(user)
			limiter.AllowCommand(user)
		}(i)
	}
	wg.Wait()
}

func TestErrors(t *testing.T) {
	// Verify error messages are set
	if ErrInvalidKey.Error() == "" {
		t.Error("ErrInvalidKey should have message")
	}
	if ErrDecryptFailed.Error() == "" {
		t.Error("ErrDecryptFailed should have message")
	}
	if ErrNotAuthorized.Error() == "" {
		t.Error("ErrNotAuthorized should have message")
	}
	if ErrRateLimited.Error() == "" {
		t.Error("ErrRateLimited should have message")
	}
}

// Benchmarks
func BenchmarkVaultEncrypt(b *testing.B) {
	key := GenerateKey()
	vault, _ := NewVault(key)
	data := []byte("Benchmark encryption test data")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vault.Encrypt(data)
	}
}

func BenchmarkVaultDecrypt(b *testing.B) {
	key := GenerateKey()
	vault, _ := NewVault(key)
	ciphertext, _ := vault.Encrypt([]byte("Benchmark decryption test data"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vault.Decrypt(ciphertext)
	}
}

func BenchmarkHashUserID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		HashUserID("telegram", "user123456")
	}
}

func BenchmarkRateLimiter(b *testing.B) {
	limiter := NewRateLimiter(10000, 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.AllowMessage("bench_user")
	}
}

// Session tests
func TestSession(t *testing.T) {
	t.Run("IsValid_NewSession", func(t *testing.T) {
		now := time.Now()
		sess := &Session{
			UserID:    "user1",
			Platform:  "telegram",
			CreatedAt: now,
			ExpiresAt: now.Add(SessionTimeout),
			LastSeen:  now,
		}
		if !sess.IsValid() {
			t.Error("New session should be valid")
		}
	})

	t.Run("IsValid_Expired", func(t *testing.T) {
		now := time.Now()
		sess := &Session{
			UserID:    "user1",
			Platform:  "telegram",
			CreatedAt: now.Add(-25 * time.Hour),
			ExpiresAt: now.Add(-1 * time.Hour), // Expired 1 hour ago
			LastSeen:  now,
		}
		if sess.IsValid() {
			t.Error("Expired session should be invalid")
		}
	})

	t.Run("IsValid_IdleTimeout", func(t *testing.T) {
		now := time.Now()
		sess := &Session{
			UserID:    "user1",
			Platform:  "telegram",
			CreatedAt: now.Add(-1 * time.Hour),
			ExpiresAt: now.Add(23 * time.Hour),
			LastSeen:  now.Add(-5 * time.Hour), // Idle for 5 hours
		}
		if sess.IsValid() {
			t.Error("Idle session should be invalid")
		}
	})

	t.Run("Touch", func(t *testing.T) {
		now := time.Now()
		sess := &Session{
			UserID:    "user1",
			Platform:  "telegram",
			CreatedAt: now,
			ExpiresAt: now.Add(SessionTimeout),
			LastSeen:  now.Add(-1 * time.Hour),
		}
		oldLastSeen := sess.LastSeen
		sess.Touch()
		if !sess.LastSeen.After(oldLastSeen) {
			t.Error("Touch should update LastSeen")
		}
	})
}

func TestSessionManager(t *testing.T) {
	t.Run("NewSessionManager", func(t *testing.T) {
		sm := NewSessionManager()
		defer sm.Stop()
		if sm == nil {
			t.Fatal("SessionManager should not be nil")
		}
	})

	t.Run("GetOrCreate_New", func(t *testing.T) {
		sm := NewSessionManager()
		defer sm.Stop()

		sess := sm.GetOrCreate("telegram", "user1")
		if sess == nil {
			t.Fatal("Session should be created")
		}
		if sess.Platform != "telegram" {
			t.Errorf("Expected platform 'telegram', got '%s'", sess.Platform)
		}
		if sess.UserID != "user1" {
			t.Errorf("Expected userID 'user1', got '%s'", sess.UserID)
		}
	})

	t.Run("GetOrCreate_Existing", func(t *testing.T) {
		sm := NewSessionManager()
		defer sm.Stop()

		sess1 := sm.GetOrCreate("telegram", "user1")
		sess2 := sm.GetOrCreate("telegram", "user1")
		if sess1 != sess2 {
			t.Error("Should return same session")
		}
	})

	t.Run("GetOrCreate_DifferentUsers", func(t *testing.T) {
		sm := NewSessionManager()
		defer sm.Stop()

		sess1 := sm.GetOrCreate("telegram", "user1")
		sess2 := sm.GetOrCreate("telegram", "user2")
		if sess1 == sess2 {
			t.Error("Different users should have different sessions")
		}
	})

	t.Run("Validate_NoSession", func(t *testing.T) {
		sm := NewSessionManager()
		defer sm.Stop()

		err := sm.Validate("telegram", "nonexistent")
		if err != nil {
			t.Error("No session should not error")
		}
	})

	t.Run("Validate_ValidSession", func(t *testing.T) {
		sm := NewSessionManager()
		defer sm.Stop()

		sm.GetOrCreate("telegram", "user1")
		err := sm.Validate("telegram", "user1")
		if err != nil {
			t.Errorf("Valid session should not error: %v", err)
		}
	})

	t.Run("Invalidate", func(t *testing.T) {
		sm := NewSessionManager()
		defer sm.Stop()

		sm.GetOrCreate("telegram", "user1")
		sm.Invalidate("telegram", "user1")

		// Create new session - should get a new one
		sess2 := sm.GetOrCreate("telegram", "user1")
		if sess2 == nil {
			t.Error("Should create new session after invalidate")
		}
	})
}

func TestAuthAttempts(t *testing.T) {
	t.Run("NewAuthAttempts", func(t *testing.T) {
		aa := NewAuthAttempts()
		if aa == nil {
			t.Fatal("AuthAttempts should not be nil")
		}
	})

	t.Run("RecordFailure", func(t *testing.T) {
		aa := NewAuthAttempts()
		aa.RecordFailure("user1")
		// Should not panic
	})

	t.Run("IsLocked_BelowThreshold", func(t *testing.T) {
		aa := NewAuthAttempts()
		for i := 0; i < MaxFailedAttempts-1; i++ {
			aa.RecordFailure("user1")
		}
		if aa.IsLocked("user1") {
			t.Error("Should not be locked below threshold")
		}
	})

	t.Run("IsLocked_AtThreshold", func(t *testing.T) {
		aa := NewAuthAttempts()
		for i := 0; i < MaxFailedAttempts; i++ {
			aa.RecordFailure("locked_user")
		}
		if !aa.IsLocked("locked_user") {
			t.Error("Should be locked at threshold")
		}
	})

	t.Run("IsLocked_DifferentUsers", func(t *testing.T) {
		aa := NewAuthAttempts()
		for i := 0; i < MaxFailedAttempts; i++ {
			aa.RecordFailure("bad_user")
		}
		if aa.IsLocked("good_user") {
			t.Error("Different user should not be locked")
		}
	})

	t.Run("ClearFailures", func(t *testing.T) {
		aa := NewAuthAttempts()
		for i := 0; i < MaxFailedAttempts; i++ {
			aa.RecordFailure("clear_user")
		}
		aa.ClearFailures("clear_user")
		if aa.IsLocked("clear_user") {
			t.Error("Should not be locked after clearing")
		}
	})

	t.Run("Cleanup", func(t *testing.T) {
		aa := NewAuthAttempts()
		aa.RecordFailure("cleanup_user")
		aa.Cleanup()
		// Should not panic
	})
}

func TestSecurityErrors(t *testing.T) {
	t.Run("ErrorMessages", func(t *testing.T) {
		if ErrSessionExpired.Error() == "" {
			t.Error("ErrSessionExpired should have message")
		}
		if ErrSessionIdle.Error() == "" {
			t.Error("ErrSessionIdle should have message")
		}
		if ErrAccountLocked.Error() == "" {
			t.Error("ErrAccountLocked should have message")
		}
	})
}

func TestSessionManagerConcurrency(t *testing.T) {
	sm := NewSessionManager()
	defer sm.Stop()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			sm.GetOrCreate("telegram", "concurrent_user")
			sm.Validate("telegram", "concurrent_user")
		}(i)
	}
	wg.Wait()
}

func TestAuthAttemptsConcurrency(t *testing.T) {
	aa := NewAuthAttempts()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			aa.RecordFailure("concurrent_user")
			aa.IsLocked("concurrent_user")
		}(i)
	}
	wg.Wait()
}

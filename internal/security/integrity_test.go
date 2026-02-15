package security

import (
	"encoding/base64"
	"sync"
	"testing"
	"time"
)

func TestGenerateSigningKey(t *testing.T) {
	t.Run("ValidKey", func(t *testing.T) {
		key := GenerateSigningKey()
		if key == "" {
			t.Error("Key should not be empty")
		}

		decoded, err := base64.StdEncoding.DecodeString(key)
		if err != nil {
			t.Errorf("Key should be valid base64: %v", err)
		}
		if len(decoded) != 32 {
			t.Errorf("Key should be 32 bytes, got %d", len(decoded))
		}
	})

	t.Run("UniqueKeys", func(t *testing.T) {
		keys := make(map[string]bool)
		for i := 0; i < 100; i++ {
			key := GenerateSigningKey()
			if keys[key] {
				t.Error("Generated duplicate key")
			}
			keys[key] = true
		}
	})
}

func TestNewSigner(t *testing.T) {
	t.Run("ValidKey", func(t *testing.T) {
		key := GenerateSigningKey()
		signer, err := NewSigner(key, 0)
		if err != nil {
			t.Fatalf("Failed to create signer: %v", err)
		}
		if signer == nil {
			t.Error("Signer should not be nil")
		}
	})

	t.Run("InvalidBase64", func(t *testing.T) {
		_, err := NewSigner("not-valid-base64!!!", 0)
		if err != ErrInvalidKey {
			t.Errorf("Expected ErrInvalidKey, got %v", err)
		}
	})

	t.Run("ShortKey", func(t *testing.T) {
		shortKey := base64.StdEncoding.EncodeToString([]byte("short"))
		_, err := NewSigner(shortKey, 0)
		if err == nil {
			t.Error("Should error on short key")
		}
	})

	t.Run("EmptyKey", func(t *testing.T) {
		_, err := NewSigner("", 0)
		if err == nil {
			t.Error("Expected error for empty key")
		}
	})

	t.Run("WithTTL", func(t *testing.T) {
		key := GenerateSigningKey()
		signer, err := NewSigner(key, time.Hour)
		if err != nil {
			t.Fatalf("Failed to create signer with TTL: %v", err)
		}
		if signer.ttl != time.Hour {
			t.Errorf("Expected TTL of 1 hour, got %v", signer.ttl)
		}
	})
}

func TestSignerSignVerify(t *testing.T) {
	key := GenerateSigningKey()
	signer, _ := NewSigner(key, 0)

	t.Run("BasicSignVerify", func(t *testing.T) {
		content := []byte("Hello, World!")
		signed, err := signer.Sign(content)
		if err != nil {
			t.Fatalf("Sign failed: %v", err)
		}

		if signed == "" {
			t.Error("Signed message should not be empty")
		}

		verified, err := signer.Verify(signed)
		if err != nil {
			t.Fatalf("Verify failed: %v", err)
		}

		if string(verified) != string(content) {
			t.Errorf("Expected '%s', got '%s'", content, verified)
		}
	})

	t.Run("EmptyContent", func(t *testing.T) {
		signed, err := signer.Sign([]byte(""))
		if err != nil {
			t.Fatalf("Sign empty failed: %v", err)
		}

		verified, err := signer.Verify(signed)
		if err != nil {
			t.Fatalf("Verify empty failed: %v", err)
		}

		if string(verified) != "" {
			t.Error("Expected empty string")
		}
	})

	t.Run("BinaryContent", func(t *testing.T) {
		content := []byte{0x00, 0x01, 0xFF, 0xFE, 0x00}
		signed, _ := signer.Sign(content)
		verified, _ := signer.Verify(signed)

		if string(verified) != string(content) {
			t.Error("Binary content mismatch")
		}
	})

	t.Run("LargeContent", func(t *testing.T) {
		content := make([]byte, 1024*100) // 100KB
		for i := range content {
			content[i] = byte(i % 256)
		}

		signed, err := signer.Sign(content)
		if err != nil {
			t.Fatalf("Sign large failed: %v", err)
		}

		verified, err := signer.Verify(signed)
		if err != nil {
			t.Fatalf("Verify large failed: %v", err)
		}

		if len(verified) != len(content) {
			t.Errorf("Size mismatch: got %d, want %d", len(verified), len(content))
		}
	})
}

func TestSignerVerifyErrors(t *testing.T) {
	key := GenerateSigningKey()
	signer, _ := NewSigner(key, 0)

	t.Run("InvalidBase64", func(t *testing.T) {
		_, err := signer.Verify("not-valid-base64!!!")
		if err != ErrInvalidFormat {
			t.Errorf("Expected ErrInvalidFormat, got %v", err)
		}
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		invalidJSON := base64.StdEncoding.EncodeToString([]byte("not json"))
		_, err := signer.Verify(invalidJSON)
		if err != ErrInvalidFormat {
			t.Errorf("Expected ErrInvalidFormat, got %v", err)
		}
	})

	t.Run("TamperedSignature", func(t *testing.T) {
		signed, _ := signer.Sign([]byte("secret"))
		// Tamper with it
		tampered := signed[:len(signed)-4] + "XXXX"
		_, err := signer.Verify(tampered)
		// Could be format or signature error
		if err == nil {
			t.Error("Should error on tampered signature")
		}
	})

	t.Run("WrongKey", func(t *testing.T) {
		signed, _ := signer.Sign([]byte("secret"))

		otherKey := GenerateSigningKey()
		otherSigner, _ := NewSigner(otherKey, 0)
		_, err := otherSigner.Verify(signed)
		if err != ErrInvalidSignature {
			t.Errorf("Expected ErrInvalidSignature for wrong key, got %v", err)
		}
	})
}

func TestSignerTTL(t *testing.T) {
	t.Run("ValidWithinTTL", func(t *testing.T) {
		key := GenerateSigningKey()
		signer, _ := NewSigner(key, 5*time.Second) // Long enough for test
		signed, _ := signer.Sign([]byte("content"))
		_, err := signer.Verify(signed)
		if err != nil {
			t.Errorf("Should be valid within TTL: %v", err)
		}
	})

	t.Run("ExpiredAfterTTL", func(t *testing.T) {
		key := GenerateSigningKey()
		signer, _ := NewSigner(key, 50*time.Millisecond) // Short for expiry test
		signed, _ := signer.Sign([]byte("content"))
		time.Sleep(100 * time.Millisecond)
		_, err := signer.Verify(signed)
		if err != ErrExpiredMessage {
			t.Errorf("Expected ErrExpiredMessage, got %v", err)
		}
	})
}

func TestIntegrityVault(t *testing.T) {
	encKey := GenerateKey()
	signKey := GenerateSigningKey()

	t.Run("NewIntegrityVault_Valid", func(t *testing.T) {
		vault, err := NewIntegrityVault(encKey, signKey)
		if err != nil {
			t.Fatalf("Failed to create IntegrityVault: %v", err)
		}
		if vault == nil {
			t.Error("IntegrityVault should not be nil")
		}
	})

	t.Run("NewIntegrityVault_InvalidEncKey", func(t *testing.T) {
		_, err := NewIntegrityVault("invalid", signKey)
		if err == nil {
			t.Error("Should error on invalid encryption key")
		}
	})

	t.Run("NewIntegrityVault_InvalidSignKey", func(t *testing.T) {
		_, err := NewIntegrityVault(encKey, "invalid")
		if err == nil {
			t.Error("Should error on invalid signing key")
		}
	})

	t.Run("EncryptAndSign_VerifyAndDecrypt", func(t *testing.T) {
		vault, _ := NewIntegrityVault(encKey, signKey)
		plaintext := []byte("Super secret data")

		signed, err := vault.EncryptAndSign(plaintext)
		if err != nil {
			t.Fatalf("EncryptAndSign failed: %v", err)
		}

		decrypted, err := vault.VerifyAndDecrypt(signed)
		if err != nil {
			t.Fatalf("VerifyAndDecrypt failed: %v", err)
		}

		if string(decrypted) != string(plaintext) {
			t.Errorf("Expected '%s', got '%s'", plaintext, decrypted)
		}
	})

	t.Run("VerifyAndDecrypt_TamperedData", func(t *testing.T) {
		vault, _ := NewIntegrityVault(encKey, signKey)
		signed, _ := vault.EncryptAndSign([]byte("data"))

		tampered := signed[:len(signed)-4] + "XXXX"
		_, err := vault.VerifyAndDecrypt(tampered)
		if err == nil {
			t.Error("Should error on tampered data")
		}
	})

	t.Run("VerifyAndDecrypt_InvalidFormat", func(t *testing.T) {
		vault, _ := NewIntegrityVault(encKey, signKey)
		_, err := vault.VerifyAndDecrypt("not-valid-format")
		if err == nil {
			t.Error("Should error on invalid format")
		}
	})
}

func TestHashForIntegrity(t *testing.T) {
	t.Run("Consistency", func(t *testing.T) {
		data := []byte("test data")
		hash1 := HashForIntegrity(data)
		hash2 := HashForIntegrity(data)
		if hash1 != hash2 {
			t.Error("Same data should produce same hash")
		}
	})

	t.Run("DifferentData", func(t *testing.T) {
		hash1 := HashForIntegrity([]byte("data1"))
		hash2 := HashForIntegrity([]byte("data2"))
		if hash1 == hash2 {
			t.Error("Different data should produce different hashes")
		}
	})

	t.Run("NotEmpty", func(t *testing.T) {
		hash := HashForIntegrity([]byte("data"))
		if hash == "" {
			t.Error("Hash should not be empty")
		}
	})

	t.Run("EmptyData", func(t *testing.T) {
		hash := HashForIntegrity([]byte(""))
		if hash == "" {
			t.Error("Hash of empty data should not be empty")
		}
	})
}

func TestVerifyHash(t *testing.T) {
	t.Run("ValidHash", func(t *testing.T) {
		data := []byte("test data")
		hash := HashForIntegrity(data)
		if !VerifyHash(data, hash) {
			t.Error("Valid hash should verify")
		}
	})

	t.Run("InvalidHash", func(t *testing.T) {
		data := []byte("test data")
		if VerifyHash(data, "invalid-hash") {
			t.Error("Invalid hash should not verify")
		}
	})

	t.Run("TamperedData", func(t *testing.T) {
		data := []byte("original")
		hash := HashForIntegrity(data)
		if VerifyHash([]byte("tampered"), hash) {
			t.Error("Tampered data should not verify")
		}
	})
}

func TestFileIntegrity(t *testing.T) {
	t.Run("NewFileIntegrity", func(t *testing.T) {
		fi := NewFileIntegrity()
		if fi == nil {
			t.Fatal("FileIntegrity should not be nil")
		}
	})

	t.Run("RecordAndVerify", func(t *testing.T) {
		fi := NewFileIntegrity()
		content := []byte("file content")
		fi.RecordHash("/path/to/file", content)

		if !fi.VerifyFile("/path/to/file", content) {
			t.Error("File should verify with correct content")
		}
	})

	t.Run("VerifyModifiedFile", func(t *testing.T) {
		fi := NewFileIntegrity()
		fi.RecordHash("/path/to/file", []byte("original"))

		if fi.VerifyFile("/path/to/file", []byte("modified")) {
			t.Error("Modified file should not verify")
		}
	})

	t.Run("VerifyUnknownFile", func(t *testing.T) {
		fi := NewFileIntegrity()
		if fi.VerifyFile("/unknown/path", []byte("content")) {
			t.Error("Unknown file should not verify")
		}
	})

	t.Run("GetHash", func(t *testing.T) {
		fi := NewFileIntegrity()
		content := []byte("content")
		fi.RecordHash("/path", content)

		hash, ok := fi.GetHash("/path")
		if !ok {
			t.Error("Should find recorded hash")
		}
		if hash == "" {
			t.Error("Hash should not be empty")
		}
	})

	t.Run("GetHash_NotFound", func(t *testing.T) {
		fi := NewFileIntegrity()
		_, ok := fi.GetHash("/not/found")
		if ok {
			t.Error("Should not find unrecorded hash")
		}
	})

	t.Run("UpdateHash", func(t *testing.T) {
		fi := NewFileIntegrity()
		fi.RecordHash("/path", []byte("v1"))
		hash1, _ := fi.GetHash("/path")

		fi.RecordHash("/path", []byte("v2"))
		hash2, _ := fi.GetHash("/path")

		if hash1 == hash2 {
			t.Error("Updated hash should differ")
		}
	})
}

func TestFileIntegrityConcurrency(t *testing.T) {
	fi := NewFileIntegrity()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			fi.RecordHash("/concurrent", []byte("content"))
		}(i)
		go func(n int) {
			defer wg.Done()
			fi.VerifyFile("/concurrent", []byte("content"))
			fi.GetHash("/concurrent")
		}(i)
	}
	wg.Wait()
}

func TestSignerConcurrency(t *testing.T) {
	key := GenerateSigningKey()
	signer, _ := NewSigner(key, 0)

	var wg sync.WaitGroup
	errors := make(chan error, 200)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			content := []byte("concurrent message")
			signed, err := signer.Sign(content)
			if err != nil {
				errors <- err
				return
			}
			verified, err := signer.Verify(signed)
			if err != nil {
				errors <- err
				return
			}
			if string(verified) != string(content) {
				errors <- ErrInvalidSignature
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent error: %v", err)
	}
}

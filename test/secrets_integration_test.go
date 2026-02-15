// Package test contains integration tests for secrets module
package test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kusa/magabot/internal/secrets"
)

func TestSecretsLocalBackend(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	backend, err := secrets.NewLocal(&secrets.LocalConfig{
		Path: filepath.Join(tmpDir, "secrets.json"),
	})
	if err != nil {
		t.Fatalf("Failed to create local backend: %v", err)
	}

	t.Run("Name", func(t *testing.T) {
		name := backend.Name()
		if name != "local" {
			t.Errorf("Expected 'local', got '%s'", name)
		}
	})

	t.Run("SetAndGet", func(t *testing.T) {
		err := backend.Set(ctx, "api_key", "secret123")
		if err != nil {
			t.Fatalf("Failed to set secret: %v", err)
		}

		value, err := backend.Get(ctx, "api_key")
		if err != nil {
			t.Fatalf("Failed to get secret: %v", err)
		}

		if value != "secret123" {
			t.Errorf("Expected 'secret123', got '%s'", value)
		}
	})

	t.Run("GetNonExistent", func(t *testing.T) {
		_, err := backend.Get(ctx, "nonexistent")
		if err != secrets.ErrNotFound {
			t.Errorf("Expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Update", func(t *testing.T) {
		_ = backend.Set(ctx, "update_key", "value1")

		err := backend.Set(ctx, "update_key", "value2")
		if err != nil {
			t.Fatalf("Failed to update secret: %v", err)
		}

		value, _ := backend.Get(ctx, "update_key")
		if value != "value2" {
			t.Errorf("Expected 'value2', got '%s'", value)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		_ = backend.Set(ctx, "delete_key", "to_delete")

		err := backend.Delete(ctx, "delete_key")
		if err != nil {
			t.Fatalf("Failed to delete secret: %v", err)
		}

		_, err = backend.Get(ctx, "delete_key")
		if err != secrets.ErrNotFound {
			t.Error("Secret should be deleted")
		}
	})

	t.Run("List", func(t *testing.T) {
		// Clear and add known keys
		_ = backend.Delete(ctx, "api_key")
		_ = backend.Delete(ctx, "update_key")

		_ = backend.Set(ctx, "key1", "val1")
		_ = backend.Set(ctx, "key2", "val2")

		keys, err := backend.List(ctx)
		if err != nil {
			t.Fatalf("Failed to list secrets: %v", err)
		}

		if len(keys) < 2 {
			t.Errorf("Expected at least 2 keys, got %d", len(keys))
		}
	})

	t.Run("Ping", func(t *testing.T) {
		err := backend.Ping(ctx)
		if err != nil {
			t.Errorf("Ping should succeed: %v", err)
		}
	})
}

func TestSecretsEnvBackend(t *testing.T) {
	ctx := context.Background()

	backend := secrets.NewEnv()

	t.Run("Name", func(t *testing.T) {
		name := backend.Name()
		if name != "env" {
			t.Errorf("Expected 'env', got '%s'", name)
		}
	})

	t.Run("GetFromEnv", func(t *testing.T) {
		os.Setenv("ANTHROPIC_API_KEY", "env_secret")
		defer os.Unsetenv("ANTHROPIC_API_KEY")

		value, err := backend.Get(ctx, secrets.KeyAnthropicAPIKey)
		if err != nil {
			t.Fatalf("Failed to get from env: %v", err)
		}

		if value != "env_secret" {
			t.Errorf("Expected 'env_secret', got '%s'", value)
		}
	})

	t.Run("GetNonExistent", func(t *testing.T) {
		os.Unsetenv("NONEXISTENT_KEY_12345")
		_, err := backend.Get(ctx, "nonexistent_key_12345")
		if err != secrets.ErrNotFound {
			t.Errorf("Expected ErrNotFound, got %v", err)
		}
	})

	t.Run("SetNotSupported", func(t *testing.T) {
		err := backend.Set(ctx, "NEW_KEY", "value")
		if err == nil {
			t.Error("Set should not be supported for env backend")
		}
	})

	t.Run("Ping", func(t *testing.T) {
		err := backend.Ping(ctx)
		if err != nil {
			t.Errorf("Ping should succeed: %v", err)
		}
	})
}

func TestSecretsChainBackend(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	// Create local backend
	localBackend, _ := secrets.NewLocal(&secrets.LocalConfig{
		Path: filepath.Join(tmpDir, "secrets.json"),
	})
	_ = localBackend.Set(ctx, "local_only", "from_local")

	// Create env backend
	os.Setenv("ANTHROPIC_API_KEY", "from_env")
	defer os.Unsetenv("ANTHROPIC_API_KEY")
	envBackend := secrets.NewEnv()

	// Create chain (env first, then local)
	chain := secrets.NewChain(envBackend, localBackend)

	t.Run("Name", func(t *testing.T) {
		name := chain.Name()
		if name == "" {
			t.Error("Chain name should not be empty")
		}
	})

	t.Run("GetFromFirstBackend", func(t *testing.T) {
		value, err := chain.Get(ctx, secrets.KeyAnthropicAPIKey)
		if err != nil {
			t.Fatalf("Failed to get from chain: %v", err)
		}

		if value != "from_env" {
			t.Errorf("Expected 'from_env', got '%s'", value)
		}
	})

	t.Run("FallbackToSecondBackend", func(t *testing.T) {
		value, err := chain.Get(ctx, "local_only")
		if err != nil {
			t.Fatalf("Failed to get from chain (fallback): %v", err)
		}

		if value != "from_local" {
			t.Errorf("Expected 'from_local', got '%s'", value)
		}
	})

	t.Run("NotFoundInAnyBackend", func(t *testing.T) {
		_, err := chain.Get(ctx, "nonexistent_anywhere")
		if err != secrets.ErrNotFound {
			t.Errorf("Expected ErrNotFound, got %v", err)
		}
	})

	t.Run("SetUsesFirstBackend", func(t *testing.T) {
		// Chain Set should go to first backend
		err := chain.Set(ctx, "chain_set_key", "chain_value")
		// Env backend might not support Set, so check error
		if err != nil {
			t.Logf("Set to chain failed (expected if first backend is env): %v", err)
		}
	})

	t.Run("Ping", func(t *testing.T) {
		err := chain.Ping(ctx)
		if err != nil {
			t.Errorf("Ping should succeed if any backend is available: %v", err)
		}
	})

	t.Run("ListUsesFirstBackend", func(t *testing.T) {
		keys, err := chain.List(ctx)
		// May return error if first backend doesn't support List
		if err != nil {
			t.Logf("List from chain: %v", err)
		} else {
			t.Logf("Listed %d keys", len(keys))
		}
	})
}

func TestSecretsChainPriority(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	// Create two local backends with same key but different values
	backend1, _ := secrets.NewLocal(&secrets.LocalConfig{
		Path: filepath.Join(tmpDir, "backend1", "secrets.json"),
	})
	backend2, _ := secrets.NewLocal(&secrets.LocalConfig{
		Path: filepath.Join(tmpDir, "backend2", "secrets.json"),
	})

	_ = backend1.Set(ctx, "shared_key", "value_from_first")
	_ = backend2.Set(ctx, "shared_key", "value_from_second")

	chain := secrets.NewChain(backend1, backend2)

	// Should return value from first backend
	value, _ := chain.Get(ctx, "shared_key")
	if value != "value_from_first" {
		t.Errorf("Expected 'value_from_first', got '%s'", value)
	}
}

func TestSecretsEmptyChain(t *testing.T) {
	ctx := context.Background()

	chain := secrets.NewChain()

	t.Run("GetFromEmptyChain", func(t *testing.T) {
		_, err := chain.Get(ctx, "any_key")
		if err != secrets.ErrNotFound {
			t.Errorf("Expected ErrNotFound, got %v", err)
		}
	})

	t.Run("SetToEmptyChain", func(t *testing.T) {
		err := chain.Set(ctx, "key", "value")
		if err == nil {
			t.Error("Should error on empty chain")
		}
	})

	t.Run("DeleteFromEmptyChain", func(t *testing.T) {
		err := chain.Delete(ctx, "key")
		if err == nil {
			t.Error("Should error on empty chain")
		}
	})

	t.Run("ListEmptyChain", func(t *testing.T) {
		_, err := chain.List(ctx)
		if err == nil {
			t.Error("Should error on empty chain")
		}
	})

	t.Run("PingEmptyChain", func(t *testing.T) {
		err := chain.Ping(ctx)
		if err == nil {
			t.Error("Should error on empty chain")
		}
	})
}

func TestSecretsLocalBackendPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	secretsPath := filepath.Join(tmpDir, "persistent_secrets.json")

	// Create backend and set a secret
	backend1, _ := secrets.NewLocal(&secrets.LocalConfig{Path: secretsPath})
	_ = backend1.Set(ctx, "persistent_key", "persistent_value")

	// Create new backend instance pointing to same location
	backend2, err := secrets.NewLocal(&secrets.LocalConfig{Path: secretsPath})
	if err != nil {
		t.Fatalf("Failed to create second backend: %v", err)
	}

	// Should retrieve the persisted value
	value, err := backend2.Get(ctx, "persistent_key")
	if err != nil {
		t.Fatalf("Failed to get persisted secret: %v", err)
	}

	if value != "persistent_value" {
		t.Errorf("Expected 'persistent_value', got '%s'", value)
	}
}

func TestSecretsSpecialCharacters(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	backend, _ := secrets.NewLocal(&secrets.LocalConfig{
		Path: filepath.Join(tmpDir, "secrets.json"),
	})

	testCases := []struct {
		key   string
		value string
	}{
		{"key_with_special!@#$", "value1"},
		{"key with spaces", "value2"},
		{"key/with/slashes", "value3"},
		{"normal_key", "value with special chars !@#$%^&*()"},
		{"unicode_key_日本語", "日本語の値"},
	}

	for _, tc := range testCases {
		err := backend.Set(ctx, tc.key, tc.value)
		if err != nil {
			t.Logf("Set special key %q: %v", tc.key, err)
			continue
		}

		retrieved, err := backend.Get(ctx, tc.key)
		if err != nil {
			t.Logf("Get special key %q: %v", tc.key, err)
			continue
		}

		if retrieved != tc.value {
			t.Errorf("Key %q: expected %q, got %q", tc.key, tc.value, retrieved)
		}
	}
}

package secrets_test

import (
	"context"
	"testing"

	"github.com/kusa/magabot/internal/secrets"
)

// mockBackend is a simple in-memory implementation of secrets.Backend for testing.
type mockBackend struct {
	name    string
	data    map[string]string
	failGet bool
}

func newMockBackend(name string) *mockBackend {
	return &mockBackend{
		name: name,
		data: make(map[string]string),
	}
}

func (m *mockBackend) Name() string { return m.name }

func (m *mockBackend) Get(_ context.Context, key string) (string, error) {
	if m.failGet {
		return "", secrets.ErrBackendError
	}
	val, ok := m.data[key]
	if !ok {
		return "", secrets.ErrNotFound
	}
	return val, nil
}

func (m *mockBackend) Set(_ context.Context, key, value string) error {
	m.data[key] = value
	return nil
}

func (m *mockBackend) Delete(_ context.Context, key string) error {
	delete(m.data, key)
	return nil
}

func (m *mockBackend) List(_ context.Context) ([]string, error) {
	keys := make([]string, 0, len(m.data))
	for k := range m.data {
		keys = append(keys, k)
	}
	return keys, nil
}

func (m *mockBackend) Ping(_ context.Context) error {
	return nil
}

func TestNewManager(t *testing.T) {
	primary := newMockBackend("primary")
	mgr := secrets.NewManager(primary)
	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
	if mgr.Backend() != "primary" {
		t.Errorf("expected backend name 'primary', got %q", mgr.Backend())
	}
}

func TestManager_GetSet(t *testing.T) {
	ctx := context.Background()
	primary := newMockBackend("primary")
	mgr := secrets.NewManager(primary)

	tests := []struct {
		key   string
		value string
	}{
		{"api_key", "sk-12345"},
		{"token", "tok-abc"},
		{"empty", ""},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			if err := mgr.Set(ctx, tc.key, tc.value); err != nil {
				t.Fatalf("Set(%q): %v", tc.key, err)
			}
			got, err := mgr.Get(ctx, tc.key)
			if err != nil {
				t.Fatalf("Get(%q): %v", tc.key, err)
			}
			if got != tc.value {
				t.Errorf("Get(%q) = %q, want %q", tc.key, got, tc.value)
			}
		})
	}
}

func TestManager_Get_Fallback(t *testing.T) {
	ctx := context.Background()

	primary := newMockBackend("primary")
	primary.failGet = true

	fallback := newMockBackend("fallback")
	fallback.data["secret_key"] = "fallback_value"

	mgr := secrets.NewManager(primary)
	mgr.SetFallback(fallback)

	got, err := mgr.Get(ctx, "secret_key")
	if err != nil {
		t.Fatalf("expected fallback to succeed, got error: %v", err)
	}
	if got != "fallback_value" {
		t.Errorf("expected 'fallback_value', got %q", got)
	}
}

func TestManager_Get_FallbackNotFound(t *testing.T) {
	ctx := context.Background()

	primary := newMockBackend("primary")
	primary.failGet = true

	fallback := newMockBackend("fallback")
	// fallback has no data, so it should return ErrNotFound

	mgr := secrets.NewManager(primary)
	mgr.SetFallback(fallback)

	_, err := mgr.Get(ctx, "missing_key")
	if err == nil {
		t.Fatal("expected error when both primary and fallback fail, got nil")
	}
}

func TestManager_Get_NoFallback(t *testing.T) {
	ctx := context.Background()

	primary := newMockBackend("primary")
	primary.failGet = true

	mgr := secrets.NewManager(primary)

	_, err := mgr.Get(ctx, "any_key")
	if err == nil {
		t.Fatal("expected error when primary fails and no fallback, got nil")
	}
}

func TestManager_Get_NotFound_PrimaryOnly(t *testing.T) {
	ctx := context.Background()

	primary := newMockBackend("primary")
	mgr := secrets.NewManager(primary)

	_, err := mgr.Get(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent key, got nil")
	}
}

func TestManager_Delete(t *testing.T) {
	ctx := context.Background()
	primary := newMockBackend("primary")
	mgr := secrets.NewManager(primary)

	// Set then delete.
	if err := mgr.Set(ctx, "to_delete", "value"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if err := mgr.Delete(ctx, "to_delete"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify it is gone.
	_, err := mgr.Get(ctx, "to_delete")
	if err == nil {
		t.Error("expected error after Delete, got nil")
	}
}

func TestManager_Delete_NonExistent(t *testing.T) {
	ctx := context.Background()
	primary := newMockBackend("primary")
	mgr := secrets.NewManager(primary)

	// Deleting a non-existent key should not error (map delete is a no-op).
	if err := mgr.Delete(ctx, "never_existed"); err != nil {
		t.Fatalf("Delete non-existent key: %v", err)
	}
}

func TestManager_Backend(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"local", "local"},
		{"vault", "vault"},
		{"custom-backend", "custom-backend"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			primary := newMockBackend(tc.expected)
			mgr := secrets.NewManager(primary)
			if got := mgr.Backend(); got != tc.expected {
				t.Errorf("Backend() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestManager_Ping(t *testing.T) {
	ctx := context.Background()
	primary := newMockBackend("primary")
	mgr := secrets.NewManager(primary)

	if err := mgr.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestManager_Set_OnlyUsesPrimary(t *testing.T) {
	ctx := context.Background()

	primary := newMockBackend("primary")
	fallback := newMockBackend("fallback")

	mgr := secrets.NewManager(primary)
	mgr.SetFallback(fallback)

	if err := mgr.Set(ctx, "key1", "value1"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Verify value is in primary.
	if primary.data["key1"] != "value1" {
		t.Errorf("expected primary to have key1=value1, got %q", primary.data["key1"])
	}

	// Verify value is NOT in fallback.
	if _, ok := fallback.data["key1"]; ok {
		t.Error("expected fallback to NOT have key1, but it does")
	}
}

func TestManager_Stop(t *testing.T) {
	primary := newMockBackend("primary")
	mgr := secrets.NewManager(primary)

	// Stop should not panic even though mockBackend does not implement Stoppable.
	mgr.Stop()
}

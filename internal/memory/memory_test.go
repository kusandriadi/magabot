package memory

import (
	"os"
	"testing"
)

func TestMemoryStore(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "magabot-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create store
	store, err := NewStore(tmpDir, "test-user")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Test Add
	mem, err := store.Add("fact", "My name is Test User", "test", "telegram", nil, 5)
	if err != nil {
		t.Fatalf("Failed to add memory: %v", err)
	}
	if mem.ID == "" {
		t.Error("Memory should have an ID")
	}
	if mem.Type != "fact" {
		t.Errorf("Memory type should be 'fact', got %s", mem.Type)
	}

	// Test List
	memories := store.List("")
	if len(memories) != 1 {
		t.Errorf("Expected 1 memory, got %d", len(memories))
	}

	// Test Search
	results := store.Search("Test User", 5)
	if len(results) == 0 {
		t.Error("Search should find the memory")
	}
	if results[0].Content != "My name is Test User" {
		t.Errorf("Search returned wrong content: %s", results[0].Content)
	}

	// Test Delete
	err = store.Delete(mem.ID)
	if err != nil {
		t.Fatalf("Failed to delete memory: %v", err)
	}

	memories = store.List("")
	if len(memories) != 0 {
		t.Errorf("Expected 0 memories after delete, got %d", len(memories))
	}
}

func TestMemoryRemember(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "magabot-test-*")
	defer os.RemoveAll(tmpDir)

	store, _ := NewStore(tmpDir, "test-user")

	// Test Remember with different content types
	tests := []struct {
		content      string
		expectedType string
	}{
		{"Nama saya Budi", "fact"},
		{"Saya suka kopi", "preference"},
		{"Kemarin saya pergi ke mall", "event"},
		{"Random note here", "note"},
	}

	for _, tt := range tests {
		mem, err := store.Remember(tt.content, "telegram")
		if err != nil {
			t.Errorf("Remember failed for %q: %v", tt.content, err)
		}
		if mem.Type != tt.expectedType {
			t.Errorf("Expected type %s for %q, got %s", tt.expectedType, tt.content, mem.Type)
		}
	}
}

func TestMemoryStats(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "magabot-test-*")
	defer os.RemoveAll(tmpDir)

	store, _ := NewStore(tmpDir, "test-user")

	// Add some memories
_, _ = store.Add("fact", "Fact 1", "test", "telegram", nil, 5)
	_, _ = store.Add("fact", "Fact 2", "test", "telegram", nil, 5)
	_, _ = store.Add("preference", "Preference 1", "test", "telegram", nil, 5)

	stats := store.Stats()

	if stats["total"] != 3 {
		t.Errorf("Expected total 3, got %d", stats["total"])
	}
	if stats["fact"] != 2 {
		t.Errorf("Expected 2 facts, got %d", stats["fact"])
	}
	if stats["preference"] != 1 {
		t.Errorf("Expected 1 preference, got %d", stats["preference"])
	}
}

func TestMemoryClear(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "magabot-test-*")
	defer os.RemoveAll(tmpDir)

	store, _ := NewStore(tmpDir, "test-user")

	// Add memories
_, _ = store.Add("fact", "Fact 1", "test", "telegram", nil, 5)
	_, _ = store.Add("fact", "Fact 2", "test", "telegram", nil, 5)

	// Clear
	err := store.Clear()
	if err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	stats := store.Stats()
	if stats["total"] != 0 {
		t.Errorf("Expected 0 after clear, got %d", stats["total"])
	}
}

func TestMemoryContext(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "magabot-test-*")
	defer os.RemoveAll(tmpDir)

	store, _ := NewStore(tmpDir, "test-user")

	// Add memories
_, _ = store.Add("fact", "I work at Google", "test", "telegram", nil, 8)
	_, _ = store.Add("preference", "I prefer dark mode", "test", "telegram", nil, 5)

	// Get context
	context := store.GetContext("work", 1000)
	if context == "" {
		t.Error("GetContext should return non-empty for matching query")
	}
	if !contains(context, "Google") {
		t.Error("Context should contain 'Google'")
	}
}

func TestExtractKeywords(t *testing.T) {
	tests := []struct {
		content  string
		expected []string
	}{
		{"The quick brown fox", []string{"quick", "brown", "fox"}},
		{"I love Go programming", []string{"love", "programming"}},
	}

	for _, tt := range tests {
		keywords := extractKeywords(tt.content)
		for _, exp := range tt.expected {
			found := false
			for _, kw := range keywords {
				if kw == exp {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected keyword %s in %v", exp, keywords)
			}
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

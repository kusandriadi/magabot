package embedding

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float32
	}{
		{
			name:     "identical vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{1, 0, 0},
			expected: 1.0,
		},
		{
			name:     "orthogonal vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{0, 1, 0},
			expected: 0.0,
		},
		{
			name:     "opposite vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{-1, 0, 0},
			expected: -1.0,
		},
		{
			name:     "different lengths",
			a:        []float32{1, 0},
			b:        []float32{1, 0, 0},
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CosineSimilarity(tt.a, tt.b)
			// Allow small floating point error
			if result < tt.expected-0.0001 || result > tt.expected+0.0001 {
				t.Errorf("expected %f, got %f", tt.expected, result)
			}
		})
	}
}

func TestEuclideanDistance(t *testing.T) {
	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float32
	}{
		{
			name:     "identical vectors",
			a:        []float32{0, 0, 0},
			b:        []float32{0, 0, 0},
			expected: 0.0,
		},
		{
			name:     "unit distance",
			a:        []float32{0, 0, 0},
			b:        []float32{1, 0, 0},
			expected: 1.0,
		},
		{
			name:     "3-4-5 triangle",
			a:        []float32{0, 0},
			b:        []float32{3, 4},
			expected: 5.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EuclideanDistance(tt.a, tt.b)
			if result < tt.expected-0.0001 || result > tt.expected+0.0001 {
				t.Errorf("expected %f, got %f", tt.expected, result)
			}
		})
	}
}

func TestDotProduct(t *testing.T) {
	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float32
	}{
		{
			name:     "simple",
			a:        []float32{1, 2, 3},
			b:        []float32{4, 5, 6},
			expected: 32.0, // 1*4 + 2*5 + 3*6 = 4 + 10 + 18
		},
		{
			name:     "orthogonal",
			a:        []float32{1, 0},
			b:        []float32{0, 1},
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DotProduct(tt.a, tt.b)
			if result < tt.expected-0.0001 || result > tt.expected+0.0001 {
				t.Errorf("expected %f, got %f", tt.expected, result)
			}
		})
	}
}

func TestVectorStore(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "embedding-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewVectorStore(VectorStoreConfig{
		DBPath:     tmpDir + "/test.db",
		TableName:  "test_embeddings",
		Dimensions: 3,
	})
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Add entries with embeddings
	testEntries := []struct {
		id        string
		content   string
		embedding []float32
		metadata  map[string]interface{}
	}{
		{"1", "hello world", []float32{1, 0, 0}, map[string]interface{}{"type": "greeting"}},
		{"2", "goodbye world", []float32{0, 1, 0}, map[string]interface{}{"type": "farewell"}},
		{"3", "hello there", []float32{0.9, 0.1, 0}, map[string]interface{}{"type": "greeting"}},
	}

	for _, e := range testEntries {
		err := store.AddWithEmbedding(e.id, e.content, e.embedding, e.metadata)
		if err != nil {
			t.Fatalf("failed to add entry %s: %v", e.id, err)
		}
	}

	// Test Get
	entry, err := store.Get("1")
	if err != nil {
		t.Fatalf("failed to get entry: %v", err)
	}
	if entry == nil {
		t.Fatal("entry should not be nil")
	}
	if entry.Content != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", entry.Content)
	}

	// Test Count
	count, err := store.Count()
	if err != nil {
		t.Fatalf("failed to count: %v", err)
	}
	if count != 3 {
		t.Errorf("expected count 3, got %d", count)
	}

	// Test SearchByVector
	queryVector := []float32{1, 0, 0}
	results, err := store.SearchByVector(queryVector, 2)
	if err != nil {
		t.Fatalf("failed to search: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// First result should be the most similar (id=1)
	if results[0].Entry.ID != "1" {
		t.Errorf("expected first result to be id=1, got %s", results[0].Entry.ID)
	}
	if results[0].Similarity < 0.99 {
		t.Errorf("expected similarity ~1.0 for identical vector, got %f", results[0].Similarity)
	}

	// Second result should be id=3 (similar to id=1)
	if results[1].Entry.ID != "3" {
		t.Errorf("expected second result to be id=3, got %s", results[1].Entry.ID)
	}

	// Test Delete
	err = store.Delete("1")
	if err != nil {
		t.Fatalf("failed to delete: %v", err)
	}

	entry, _ = store.Get("1")
	if entry != nil {
		t.Error("entry should be deleted")
	}

	// Test Clear
	err = store.Clear()
	if err != nil {
		t.Fatalf("failed to clear: %v", err)
	}

	count, _ = store.Count()
	if count != 0 {
		t.Errorf("expected count 0 after clear, got %d", count)
	}
}

func TestVectorStoreList(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "embedding-test-*")
	defer os.RemoveAll(tmpDir)

	store, _ := NewVectorStore(VectorStoreConfig{
		DBPath:     tmpDir + "/test.db",
		Dimensions: 3,
	})
	defer store.Close()

	// Add entries
	for i := 0; i < 10; i++ {
		_ = store.AddWithEmbedding(
			string(rune('a'+i)),
			"content",
			[]float32{float32(i), 0, 0},
			nil,
		)
	}

	// Test pagination
	entries, err := store.List(0, 5)
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}
	if len(entries) != 5 {
		t.Errorf("expected 5 entries, got %d", len(entries))
	}

	entries, err = store.List(5, 5)
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}
	if len(entries) != 5 {
		t.Errorf("expected 5 entries, got %d", len(entries))
	}
}

func TestVectorStoreMetadata(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "embedding-test-*")
	defer os.RemoveAll(tmpDir)

	store, _ := NewVectorStore(VectorStoreConfig{
		DBPath:     tmpDir + "/test.db",
		Dimensions: 3,
	})
	defer store.Close()

	metadata := map[string]interface{}{
		"type":       "test",
		"importance": 5,
		"tags":       []interface{}{"a", "b"},
	}

	err := store.AddWithEmbedding("1", "test content", []float32{1, 0, 0}, metadata)
	if err != nil {
		t.Fatalf("failed to add: %v", err)
	}

	entry, err := store.Get("1")
	if err != nil {
		t.Fatalf("failed to get: %v", err)
	}

	if entry.Metadata["type"] != "test" {
		t.Errorf("expected metadata type 'test', got '%v'", entry.Metadata["type"])
	}

	// JSON numbers are float64
	if entry.Metadata["importance"].(float64) != 5 {
		t.Errorf("expected metadata importance 5, got '%v'", entry.Metadata["importance"])
	}
}

func TestNewClient(t *testing.T) {
	client := NewClient(Config{
		Provider: ProviderOpenAI,
		APIKey:   "test-key",
		Model:    "text-embedding-3-small",
	})

	if client == nil {
		t.Fatal("client should not be nil")
	}

	// Check defaults
	if client.config.Timeout <= 0 {
		t.Error("timeout should be set")
	}
	if client.config.MaxBatchSize <= 0 {
		t.Error("max batch size should be set")
	}
	if client.config.Dimensions != 1536 {
		t.Errorf("expected dimensions 1536 for text-embedding-3-small, got %d", client.config.Dimensions)
	}
}

func TestSearchWithClient(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "embedding-test-*")
	defer os.RemoveAll(tmpDir)

	// Create store without client
	store, _ := NewVectorStore(VectorStoreConfig{
		DBPath:     tmpDir + "/test.db",
		Dimensions: 3,
	})
	defer store.Close()

	// Add some data
	_ = store.AddWithEmbedding("1", "test", []float32{1, 0, 0}, nil)

	// Search should fail without client
	_, err := store.Search(context.Background(), "query", 10)
	if err == nil {
		t.Error("expected error without client")
	}
}

func TestUpdateEntry(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "embedding-test-*")
	defer os.RemoveAll(tmpDir)

	store, _ := NewVectorStore(VectorStoreConfig{
		DBPath:     tmpDir + "/test.db",
		Dimensions: 3,
	})
	defer store.Close()

	// Add initial entry
	_ = store.AddWithEmbedding("1", "original", []float32{1, 0, 0}, nil)

	// Update entry (same ID)
	_ = store.AddWithEmbedding("1", "updated", []float32{0, 1, 0}, nil)

	entry, _ := store.Get("1")
	if entry.Content != "updated" {
		t.Errorf("expected 'updated', got '%s'", entry.Content)
	}

	// Count should still be 1
	count, _ := store.Count()
	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}
}

// --- Table name validation tests ---

func TestNewVectorStore_InvalidTableName(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "embedding-test-*")
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		name      string
		tableName string
	}{
		{"SQL injection", "x; DROP TABLE y"},
		{"special chars", "table-name!"},
		{"starts with number", "1table"},
		{"empty after default", ""}, // empty uses default "embeddings" which is valid
		{"too long", strings.Repeat("a", 65)},
		{"spaces", "my table"},
		{"quotes", "table'name"},
		{"semicolon", "table;name"},
		{"parentheses", "table(name)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.tableName == "" {
				t.Skip("empty table name defaults to 'embeddings'")
			}
			_, err := NewVectorStore(VectorStoreConfig{
				DBPath:    tmpDir + "/test.db",
				TableName: tt.tableName,
			})
			if err == nil {
				t.Errorf("expected error for table name %q, got nil", tt.tableName)
			}
		})
	}
}

func TestNewVectorStore_ValidTableNames(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "embedding-test-*")
	defer os.RemoveAll(tmpDir)

	validNames := []string{
		"embeddings",
		"test_table",
		"_private",
		"MyTable123",
		"a",
		strings.Repeat("a", 64), // exactly at limit
	}

	for _, name := range validNames {
		t.Run(name, func(t *testing.T) {
			store, err := NewVectorStore(VectorStoreConfig{
				DBPath:    tmpDir + "/" + name + ".db",
				TableName: name,
			})
			if err != nil {
				t.Errorf("expected valid table name %q to succeed, got: %v", name, err)
				return
			}
			store.Close()
		})
	}
}

func TestSearchByVector_EmptyStore(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "embedding-test-*")
	defer os.RemoveAll(tmpDir)

	store, err := NewVectorStore(VectorStoreConfig{
		DBPath:     tmpDir + "/test.db",
		Dimensions: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	results, err := store.SearchByVector([]float32{1, 0, 0}, 10)
	if err != nil {
		t.Fatalf("expected no error on empty store, got: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results from empty store, got %d", len(results))
	}
}

func TestAddAndSearch_SortOrder(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "embedding-test-*")
	defer os.RemoveAll(tmpDir)

	store, err := NewVectorStore(VectorStoreConfig{
		DBPath:     tmpDir + "/test.db",
		Dimensions: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Add entries with known similarities to query vector [1, 0, 0]
	entries := []struct {
		id        string
		embedding []float32
	}{
		{"low", []float32{0, 1, 0}},        // similarity ~ 0
		{"high", []float32{1, 0, 0}},       // similarity = 1
		{"medium", []float32{0.7, 0.7, 0}}, // similarity ~ 0.7
	}
	for _, e := range entries {
		if err := store.AddWithEmbedding(e.id, "content", e.embedding, nil); err != nil {
			t.Fatal(err)
		}
	}

	results, err := store.SearchByVector([]float32{1, 0, 0}, 10)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Verify descending similarity order
	for i := 1; i < len(results); i++ {
		if results[i].Similarity > results[i-1].Similarity {
			t.Errorf("results not sorted: [%d].Similarity=%f > [%d].Similarity=%f",
				i, results[i].Similarity, i-1, results[i-1].Similarity)
		}
	}

	if results[0].Entry.ID != "high" {
		t.Errorf("expected highest similarity first, got %s", results[0].Entry.ID)
	}
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	zero := []float32{0, 0, 0}
	nonzero := []float32{1, 2, 3}

	result := CosineSimilarity(zero, nonzero)
	if result != 0 {
		t.Errorf("expected 0 for zero vector, got %f", result)
	}

	result = CosineSimilarity(zero, zero)
	if result != 0 {
		t.Errorf("expected 0 for both zero vectors, got %f", result)
	}
}

func TestCosineSimilarity_MismatchedDimensions(t *testing.T) {
	a := []float32{1, 2}
	b := []float32{1, 2, 3, 4}

	result := CosineSimilarity(a, b)
	if result != 0 {
		t.Errorf("expected 0 for mismatched dimensions, got %f", result)
	}
}

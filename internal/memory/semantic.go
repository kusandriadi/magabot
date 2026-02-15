// Package memory provides semantic memory with embedding-based search.
package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kusa/magabot/internal/embedding"
)

// SemanticStore provides semantic memory with embedding-based similarity search.
type SemanticStore struct {
	mu       sync.RWMutex
	vectors  *embedding.VectorStore
	client   *embedding.Client
	userID   string
	dataDir  string
	logger   *slog.Logger
}

// SemanticConfig holds configuration for semantic memory.
type SemanticConfig struct {
	DataDir    string
	UserID     string
	Client     *embedding.Client
	Dimensions int
	Logger     *slog.Logger
}

// SemanticMemory represents a semantic memory entry.
type SemanticMemory struct {
	ID          string                 `json:"id"`
	Content     string                 `json:"content"`
	Type        string                 `json:"type"`        // fact, preference, event, note, context
	Source      string                 `json:"source"`      // chat, manual, auto
	Platform    string                 `json:"platform"`
	Tags        []string               `json:"tags,omitempty"`
	Importance  int                    `json:"importance"`  // 1-10
	AccessCount int                    `json:"access_count"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// NewSemanticStore creates a new semantic memory store.
func NewSemanticStore(cfg SemanticConfig) (*SemanticStore, error) {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Dimensions <= 0 {
		cfg.Dimensions = 1536
	}

	// Sanitize user ID for file path
	safeID := sanitizeFilename(cfg.UserID)
	if safeID == "" {
		return nil, fmt.Errorf("invalid user ID")
	}

	// Create data directory
	memDir := filepath.Join(cfg.DataDir, "semantic", safeID)
	if err := os.MkdirAll(memDir, 0700); err != nil {
		return nil, fmt.Errorf("create memory dir: %w", err)
	}

	// Create vector store
	vectors, err := embedding.NewVectorStore(embedding.VectorStoreConfig{
		DBPath:     filepath.Join(memDir, "vectors.db"),
		TableName:  "memories",
		Client:     cfg.Client,
		Dimensions: cfg.Dimensions,
		Logger:     cfg.Logger,
	})
	if err != nil {
		return nil, fmt.Errorf("create vector store: %w", err)
	}

	return &SemanticStore{
		vectors: vectors,
		client:  cfg.Client,
		userID:  cfg.UserID,
		dataDir: memDir,
		logger:  cfg.Logger,
	}, nil
}

// MaxContentLength is the maximum allowed content length for a memory entry.
const MaxContentLength = 100000 // 100KB

// Add stores a new semantic memory with automatic embedding generation.
func (s *SemanticStore) Add(ctx context.Context, mem *SemanticMemory) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate content length to prevent resource exhaustion
	if len(mem.Content) > MaxContentLength {
		return fmt.Errorf("content too long (max %d bytes)", MaxContentLength)
	}

	// Validate content is not empty
	if strings.TrimSpace(mem.Content) == "" {
		return fmt.Errorf("content cannot be empty")
	}

	if mem.ID == "" {
		mem.ID = uuid.New().String()[:12]
	}
	if mem.CreatedAt.IsZero() {
		mem.CreatedAt = time.Now()
	}
	mem.UpdatedAt = time.Now()

	if mem.Importance < 1 {
		mem.Importance = 5
	}
	if mem.Importance > 10 {
		mem.Importance = 10
	}

	// Convert memory to metadata for storage
	metadata := map[string]interface{}{
		"type":         mem.Type,
		"source":       mem.Source,
		"platform":     mem.Platform,
		"importance":   mem.Importance,
		"access_count": mem.AccessCount,
		"tags":         mem.Tags,
		"user_id":      s.userID,
		"created_at":   mem.CreatedAt.Unix(),
		"updated_at":   mem.UpdatedAt.Unix(),
	}
	for k, v := range mem.Metadata {
		metadata[k] = v
	}

	// Add to vector store (embedding will be generated automatically)
	return s.vectors.Add(ctx, mem.ID, mem.Content, metadata)
}

// Remember is a convenience method to add a memory from chat.
func (s *SemanticStore) Remember(ctx context.Context, content, platform, source string) (*SemanticMemory, error) {
	mem := &SemanticMemory{
		Content:    content,
		Type:       detectMemoryType(content),
		Source:     source,
		Platform:   platform,
		Tags:       extractTags(content),
		Importance: 5,
	}

	if err := s.Add(ctx, mem); err != nil {
		return nil, err
	}

	return mem, nil
}

// Search finds memories similar to the query using semantic search.
func (s *SemanticStore) Search(ctx context.Context, query string, limit int) ([]*SemanticMemory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}

	results, err := s.vectors.Search(ctx, query, limit)
	if err != nil {
		return nil, err
	}

	memories := make([]*SemanticMemory, 0, len(results))
	for _, r := range results {
		mem := entryToMemory(r.Entry)
		mem.AccessCount++
		memories = append(memories, mem)
	}

	return memories, nil
}

// SearchWithScore returns memories with their similarity scores.
func (s *SemanticStore) SearchWithScore(ctx context.Context, query string, limit int) ([]SemanticSearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}

	results, err := s.vectors.Search(ctx, query, limit)
	if err != nil {
		return nil, err
	}

	output := make([]SemanticSearchResult, 0, len(results))
	for _, r := range results {
		output = append(output, SemanticSearchResult{
			Memory:     entryToMemory(r.Entry),
			Similarity: r.Similarity,
		})
	}

	return output, nil
}

// SemanticSearchResult combines a memory with its similarity score.
type SemanticSearchResult struct {
	Memory     *SemanticMemory `json:"memory"`
	Similarity float32         `json:"similarity"`
}

// Get retrieves a memory by ID.
func (s *SemanticStore) Get(id string) (*SemanticMemory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, err := s.vectors.Get(id)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, nil
	}

	return entryToMemory(entry), nil
}

// Delete removes a memory by ID.
func (s *SemanticStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.vectors.Delete(id)
}

// Update modifies an existing memory.
func (s *SemanticStore) Update(ctx context.Context, mem *SemanticMemory) error {
	// Delete and re-add to regenerate embedding
	if err := s.Delete(mem.ID); err != nil {
		return err
	}
	return s.Add(ctx, mem)
}

// Clear removes all memories.
func (s *SemanticStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.vectors.Clear()
}

// Count returns the number of stored memories.
func (s *SemanticStore) Count() (int, error) {
	return s.vectors.Count()
}

// List returns all memories (paginated).
func (s *SemanticStore) List(offset, limit int) ([]*SemanticMemory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := s.vectors.List(offset, limit)
	if err != nil {
		return nil, err
	}

	memories := make([]*SemanticMemory, 0, len(entries))
	for _, entry := range entries {
		memories = append(memories, entryToMemory(entry))
	}

	return memories, nil
}

// GetContext builds a context string from relevant memories for LLM prompts.
func (s *SemanticStore) GetContext(ctx context.Context, query string, maxTokens int) (string, error) {
	memories, err := s.Search(ctx, query, 10)
	if err != nil {
		return "", err
	}

	if len(memories) == 0 {
		return "", nil
	}

	var result string
	totalLen := 0

	result += "Relevant memories about the user:\n"
	for _, mem := range memories {
		line := fmt.Sprintf("- [%s] %s\n", mem.Type, mem.Content)
		if totalLen+len(line) > maxTokens {
			break
		}
		result += line
		totalLen += len(line)
	}

	return result, nil
}

// Close closes the underlying database connection.
func (s *SemanticStore) Close() error {
	return s.vectors.Close()
}

// Stats returns memory statistics.
func (s *SemanticStore) Stats() (map[string]int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count, err := s.vectors.Count()
	if err != nil {
		return nil, err
	}

	// Get entries to count by type
	entries, err := s.vectors.List(0, count)
	if err != nil {
		return nil, err
	}

	stats := map[string]int{
		"total": count,
	}

	for _, entry := range entries {
		if entry.Metadata != nil {
			if t, ok := entry.Metadata["type"].(string); ok {
				stats[t]++
			}
		}
	}

	return stats, nil
}

// entryToMemory converts a vector store entry to a semantic memory.
func entryToMemory(entry *embedding.Entry) *SemanticMemory {
	mem := &SemanticMemory{
		ID:        entry.ID,
		Content:   entry.Content,
		Metadata:  make(map[string]interface{}),
		CreatedAt: entry.CreatedAt,
		UpdatedAt: entry.UpdatedAt,
	}

	if entry.Metadata != nil {
		if t, ok := entry.Metadata["type"].(string); ok {
			mem.Type = t
		}
		if s, ok := entry.Metadata["source"].(string); ok {
			mem.Source = s
		}
		if p, ok := entry.Metadata["platform"].(string); ok {
			mem.Platform = p
		}
		if i, ok := entry.Metadata["importance"].(float64); ok {
			mem.Importance = int(i)
		}
		if a, ok := entry.Metadata["access_count"].(float64); ok {
			mem.AccessCount = int(a)
		}
		if tags, ok := entry.Metadata["tags"].([]interface{}); ok {
			for _, t := range tags {
				if s, ok := t.(string); ok {
					mem.Tags = append(mem.Tags, s)
				}
			}
		}
		// Copy remaining metadata
		for k, v := range entry.Metadata {
			if k != "type" && k != "source" && k != "platform" && k != "importance" && k != "access_count" && k != "tags" && k != "user_id" && k != "created_at" && k != "updated_at" {
				mem.Metadata[k] = v
			}
		}
	}

	return mem
}

// sanitizeFilename removes unsafe characters from a filename.
// Provides defense-in-depth against path traversal and injection attacks.
func sanitizeFilename(name string) string {
	if name == "" {
		return ""
	}

	// Remove path separators, null bytes, and other dangerous characters
	for _, r := range []rune{'/', '\\', '\x00', ':', '*', '?', '"', '<', '>', '|'} {
		name = replaceRune(name, r, '_')
	}

	// Remove control characters (0x00-0x1F and 0x7F)
	var result []rune
	for _, r := range name {
		if r >= 0x20 && r != 0x7F {
			result = append(result, r)
		}
	}
	name = string(result)

	// Trim leading/trailing whitespace and dots (prevents ".." attacks after transformation)
	name = strings.Trim(name, " .\t\r\n")

	// Block reserved Windows names
	upperName := strings.ToUpper(name)
	reserved := []string{"CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4",
		"COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4",
		"LPT5", "LPT6", "LPT7", "LPT8", "LPT9"}
	for _, r := range reserved {
		if upperName == r || strings.HasPrefix(upperName, r+".") {
			name = "_" + name
			break
		}
	}

	// Limit length
	if len(name) > 100 {
		name = name[:100]
	}

	// If empty after sanitization, return a safe default
	if name == "" {
		return "_"
	}

	return name
}

func replaceRune(s string, old, new rune) string {
	result := []rune(s)
	for i, r := range result {
		if r == old {
			result[i] = new
		}
	}
	return string(result)
}

// extractTags extracts hashtags from content.
func extractTags(content string) []string {
	var tags []string
	words := splitWords(content)
	for _, word := range words {
		if len(word) > 1 && word[0] == '#' {
			tags = append(tags, word[1:])
		}
	}
	return tags
}

func splitWords(s string) []string {
	var words []string
	var current []rune
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if len(current) > 0 {
				words = append(words, string(current))
				current = nil
			}
		} else {
			current = append(current, r)
		}
	}
	if len(current) > 0 {
		words = append(words, string(current))
	}
	return words
}

// BulkImport imports multiple memories efficiently.
func (s *SemanticStore) BulkImport(ctx context.Context, memories []*SemanticMemory) error {
	for _, mem := range memories {
		if err := s.Add(ctx, mem); err != nil {
			s.logger.Warn("failed to import memory", "id", mem.ID, "error", err)
			// Continue with other memories
		}
	}
	return nil
}

// Export exports all memories to JSON.
func (s *SemanticStore) Export() ([]byte, error) {
	count, err := s.Count()
	if err != nil {
		return nil, err
	}

	memories, err := s.List(0, count)
	if err != nil {
		return nil, err
	}

	return json.MarshalIndent(memories, "", "  ")
}

// MigrateFromLegacy migrates memories from the legacy file-based store.
func (s *SemanticStore) MigrateFromLegacy(ctx context.Context, legacyStore *Store) error {
	memories := legacyStore.List("")

	for _, m := range memories {
		sem := &SemanticMemory{
			ID:          m.ID,
			Content:     m.Content,
			Type:        m.Type,
			Source:      m.Source,
			Platform:    m.Platform,
			Tags:        m.Keywords,
			Importance:  m.Importance,
			AccessCount: m.AccessCount,
			CreatedAt:   m.CreatedAt,
			UpdatedAt:   m.UpdatedAt,
		}

		if err := s.Add(ctx, sem); err != nil {
			s.logger.Warn("failed to migrate memory", "id", m.ID, "error", err)
		}
	}

	s.logger.Info("migrated memories from legacy store", "count", len(memories))
	return nil
}

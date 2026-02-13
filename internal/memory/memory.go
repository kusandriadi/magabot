// Package memory provides persistent memory and RAG capabilities
package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kusa/magabot/internal/util"
)

// Memory represents a single memory entry
type Memory struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`      // fact, preference, event, note
	Content   string    `json:"content"`   // The actual memory content
	Keywords  []string  `json:"keywords"`  // For search
	Source    string    `json:"source"`    // Where this came from (chat, manual, etc)
	UserID    string    `json:"user_id"`   // Owner of this memory
	Platform  string    `json:"platform"`  // Platform where created
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	AccessCount int     `json:"access_count"` // How often retrieved
	Importance  int     `json:"importance"`   // 1-10 scale
}

// Store manages persistent memory storage
type Store struct {
	mu       sync.RWMutex
	memories map[string]*Memory // id -> memory
	filePath string
	userID   string
}

// NewStore creates a new memory store for a user
func NewStore(dataDir, userID string) (*Store, error) {
	safeID := util.SanitizeFilename(userID)
	if safeID == "" {
		return nil, fmt.Errorf("invalid user ID")
	}
	filePath := filepath.Join(dataDir, "memory", fmt.Sprintf("%s.json", safeID))
	
	store := &Store{
		memories: make(map[string]*Memory),
		filePath: filePath,
		userID:   userID,
	}
	
	if err := store.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	
	return store, nil
}

// load reads memories from disk
func (s *Store) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}
	
	var memories []*Memory
	if err := json.Unmarshal(data, &memories); err != nil {
		return err
	}
	
	s.mu.Lock()
	defer s.mu.Unlock()
	
	for _, m := range memories {
		s.memories[m.ID] = m
	}
	
	return nil
}

// save writes memories to disk
func (s *Store) save() error {
	s.mu.RLock()
	memories := make([]*Memory, 0, len(s.memories))
	for _, m := range s.memories {
		memories = append(memories, m)
	}
	s.mu.RUnlock()
	
	data, err := json.MarshalIndent(memories, "", "  ")
	if err != nil {
		return err
	}
	
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(s.filePath), 0700); err != nil {
		return err
	}
	
	return os.WriteFile(s.filePath, data, 0600)
}

// Add stores a new memory
func (s *Store) Add(memType, content, source, platform string, keywords []string, importance int) (*Memory, error) {
	if importance < 1 {
		importance = 5
	}
	if importance > 10 {
		importance = 10
	}
	
	// Auto-extract keywords if not provided
	if len(keywords) == 0 {
		keywords = extractKeywords(content)
	}
	
	mem := &Memory{
		ID:         util.RandomID(16),
		Type:       memType,
		Content:    content,
		Keywords:   keywords,
		Source:     source,
		UserID:     s.userID,
		Platform:   platform,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Importance: importance,
	}
	
	s.mu.Lock()
	s.memories[mem.ID] = mem
	s.mu.Unlock()
	
	return mem, s.save()
}

// Remember is a convenience method to add a memory from chat
func (s *Store) Remember(content, platform string) (*Memory, error) {
	// Detect type from content
	memType := detectMemoryType(content)
	return s.Add(memType, content, "chat", platform, nil, 5)
}

// Search finds relevant memories based on query
func (s *Store) Search(query string, limit int) []*Memory {
	if limit <= 0 {
		limit = 5
	}

	queryWords := strings.Fields(strings.ToLower(query))

	s.mu.Lock()
	defer s.mu.Unlock()

	type scored struct {
		mem   *Memory
		score float64
	}

	var results []scored

	for _, mem := range s.memories {
		score := calculateRelevance(mem, queryWords)
		if score > 0 {
			results = append(results, scored{mem, score})
		}
	}

	// Sort by score (descending)
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	// Take top N
	memories := make([]*Memory, 0, limit)
	for i := 0; i < len(results) && i < limit; i++ {
		mem := results[i].mem
		mem.AccessCount++
		memories = append(memories, mem)
	}

	return memories
}

// GetContext retrieves relevant context for a conversation
func (s *Store) GetContext(query string, maxTokens int) string {
	memories := s.Search(query, 10)
	
	if len(memories) == 0 {
		return ""
	}
	
	var sb strings.Builder
	sb.WriteString("Relevant memories:\n")
	
	totalLen := 0
	for _, mem := range memories {
		line := fmt.Sprintf("- [%s] %s\n", mem.Type, mem.Content)
		if totalLen+len(line) > maxTokens {
			break
		}
		sb.WriteString(line)
		totalLen += len(line)
	}
	
	return sb.String()
}

// List returns all memories, optionally filtered by type
func (s *Store) List(memType string) []*Memory {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	memories := make([]*Memory, 0, len(s.memories))
	for _, m := range s.memories {
		if memType == "" || m.Type == memType {
			memories = append(memories, m)
		}
	}
	
	// Sort by created date (newest first)
	sort.Slice(memories, func(i, j int) bool {
		return memories[i].CreatedAt.After(memories[j].CreatedAt)
	})
	
	return memories
}

// Delete removes a memory by ID
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	delete(s.memories, id)
	s.mu.Unlock()
	return s.save()
}

// Clear removes all memories
func (s *Store) Clear() error {
	s.mu.Lock()
	s.memories = make(map[string]*Memory)
	s.mu.Unlock()
	return s.save()
}

// Stats returns memory statistics
func (s *Store) Stats() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	stats := map[string]int{
		"total": len(s.memories),
	}
	
	for _, m := range s.memories {
		stats[m.Type]++
	}
	
	return stats
}

// Helper functions

func extractKeywords(content string) []string {
	// Simple keyword extraction
	words := strings.Fields(strings.ToLower(content))
	keywords := make([]string, 0)
	
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"was": true, "were": true, "be": true, "been": true, "being": true,
		"have": true, "has": true, "had": true, "do": true, "does": true,
		"did": true, "will": true, "would": true, "could": true, "should": true,
		"i": true, "you": true, "he": true, "she": true, "it": true,
		"we": true, "they": true, "my": true, "your": true, "his": true,
		"her": true, "its": true, "our": true, "their": true,
		"yang": true, "dan": true, "di": true, "ke": true, "dari": true,
		"untuk": true, "dengan": true, "pada": true, "ini": true, "itu": true,
		"saya": true, "kamu": true, "dia": true, "kami": true, "mereka": true,
	}
	
	seen := make(map[string]bool)
	for _, word := range words {
		word = strings.Trim(word, ".,!?;:'\"")
		if len(word) > 2 && !stopWords[word] && !seen[word] {
			keywords = append(keywords, word)
			seen[word] = true
		}
	}
	
	return keywords
}

func detectMemoryType(content string) string {
	lower := strings.ToLower(content)
	
	if strings.Contains(lower, "suka") || strings.Contains(lower, "prefer") ||
		strings.Contains(lower, "favorite") || strings.Contains(lower, "favourit") {
		return "preference"
	}
	if strings.Contains(lower, "nama saya") || strings.Contains(lower, "my name") ||
		strings.Contains(lower, "i am") || strings.Contains(lower, "saya adalah") {
		return "fact"
	}
	if strings.Contains(lower, "kemarin") || strings.Contains(lower, "yesterday") ||
		strings.Contains(lower, "tadi") || strings.Contains(lower, "barusan") {
		return "event"
	}
	
	return "note"
}

func calculateRelevance(mem *Memory, queryWords []string) float64 {
	score := 0.0
	contentLower := strings.ToLower(mem.Content)
	
	for _, word := range queryWords {
		// Check content
		if strings.Contains(contentLower, word) {
			score += 1.0
		}
		// Check keywords (higher weight)
		for _, kw := range mem.Keywords {
			if strings.Contains(strings.ToLower(kw), word) {
				score += 2.0
			}
		}
	}
	
	// Boost by importance
	score *= float64(mem.Importance) / 5.0
	
	// Boost recent memories slightly
	age := time.Since(mem.CreatedAt).Hours()
	if age < 24 {
		score *= 1.2
	} else if age < 168 { // 1 week
		score *= 1.1
	}
	
	return score
}

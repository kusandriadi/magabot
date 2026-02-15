// Package embedding provides vector embeddings generation and similarity search
// for semantic memory. Supports OpenAI and Voyage AI compatible APIs.
package embedding

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/kusa/magabot/internal/util"
)

// Provider identifies the embedding API provider.
type Provider string

const (
	ProviderOpenAI   Provider = "openai"
	ProviderVoyage   Provider = "voyage"
	ProviderLocal    Provider = "local"    // Local embedding server (e.g., sentence-transformers)
	ProviderCohere   Provider = "cohere"
)

// defaultDimensions holds the default embedding dimensions for each model.
var defaultDimensions = map[string]int{
	"text-embedding-3-small":      1536,
	"text-embedding-3-large":      3072,
	"text-embedding-ada-002":      1536,
	"voyage-3":                    1024,
	"voyage-3-lite":               512,
	"voyage-code-3":               1024,
	"voyage-finance-2":            1024,
	"embed-english-v3.0":          1024,
	"embed-multilingual-v3.0":     1024,
}

// Config holds embedding service configuration.
type Config struct {
	Provider    Provider
	APIKey      string // #nosec G117 -- config field, not serialized to untrusted output
	Model       string
	BaseURL     string // Custom base URL for API
	Dimensions  int    // Output dimensions (for models that support it)
	Timeout     time.Duration
	MaxBatchSize int   // Max texts per batch request
	Logger      *slog.Logger
}

// Client generates embeddings using an API provider.
type Client struct {
	config Config
	client *http.Client
	logger *slog.Logger
}

// NewClient creates a new embedding client.
func NewClient(cfg Config) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxBatchSize <= 0 {
		cfg.MaxBatchSize = 100
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	// Set default base URLs
	if cfg.BaseURL == "" {
		switch cfg.Provider {
		case ProviderOpenAI:
			cfg.BaseURL = "https://api.openai.com/v1"
		case ProviderVoyage:
			cfg.BaseURL = "https://api.voyageai.com/v1"
		case ProviderCohere:
			cfg.BaseURL = "https://api.cohere.ai/v1"
		}
	}

	// Get default dimensions
	if cfg.Dimensions <= 0 {
		if dims, ok := defaultDimensions[cfg.Model]; ok {
			cfg.Dimensions = dims
		} else {
			cfg.Dimensions = 1536 // Fallback default
		}
	}

	return &Client{
		config: cfg,
		client: util.NewHTTPClient(cfg.Timeout),
		logger: cfg.Logger,
	}
}

// isAllowedURL validates that a URL is safe to connect to (SSRF prevention).
// For local provider, allows localhost connections.
// For cloud providers, blocks private/internal IP ranges.
func isAllowedURL(baseURL string, provider Provider) error {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Must be HTTP or HTTPS
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("unsupported URL scheme: %s", parsed.Scheme)
	}

	// For local provider, allow localhost
	if provider == ProviderLocal {
		return nil
	}

	// For cloud providers, block private networks
	host := parsed.Hostname()

	// Check for localhost variants
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return fmt.Errorf("cloud provider URL cannot point to localhost")
	}

	// Resolve hostname and check for private IPs
	ips, err := net.LookupIP(host)
	if err == nil {
		for _, ip := range ips {
			if isPrivateIP(ip) {
				return fmt.Errorf("cloud provider URL resolves to private IP: %s", ip)
			}
		}
	}

	return nil
}

// isPrivateIP checks if an IP address is in a private/reserved range.
func isPrivateIP(ip net.IP) bool {
	// Check for loopback
	if ip.IsLoopback() {
		return true
	}

	// Check for private ranges
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16", // Link-local
		"fc00::/7",       // IPv6 private
		"fe80::/10",      // IPv6 link-local
	}

	for _, cidr := range privateRanges {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}

	return false
}

// ValidateConfig validates the embedding client configuration.
func ValidateConfig(cfg Config) error {
	if cfg.BaseURL != "" {
		if err := isAllowedURL(cfg.BaseURL, cfg.Provider); err != nil {
			return fmt.Errorf("base URL validation failed: %w", err)
		}
	}

	// Validate API key is not empty for cloud providers
	if cfg.Provider != ProviderLocal && strings.TrimSpace(cfg.APIKey) == "" {
		return fmt.Errorf("API key required for provider: %s", cfg.Provider)
	}

	return nil
}

// Embedding represents a single embedding result.
type Embedding struct {
	Text      string    `json:"text"`
	Vector    []float32 `json:"vector"`
	Model     string    `json:"model,omitempty"`
	TokenCount int      `json:"token_count,omitempty"`
}

// Embed generates embeddings for a list of texts.
func (c *Client) Embed(ctx context.Context, texts []string) ([]Embedding, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	// Batch if needed
	results := make([]Embedding, 0, len(texts))

	for i := 0; i < len(texts); i += c.config.MaxBatchSize {
		end := i + c.config.MaxBatchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[i:end]

		embeddings, err := c.embedBatch(ctx, batch)
		if err != nil {
			return nil, fmt.Errorf("batch %d: %w", i/c.config.MaxBatchSize, err)
		}

		results = append(results, embeddings...)
	}

	return results, nil
}

// EmbedOne generates an embedding for a single text.
func (c *Client) EmbedOne(ctx context.Context, text string) (*Embedding, error) {
	results, err := c.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return &results[0], nil
}

// embedBatch performs the actual API call for a batch of texts.
func (c *Client) embedBatch(ctx context.Context, texts []string) ([]Embedding, error) {
	switch c.config.Provider {
	case ProviderOpenAI:
		return c.embedOpenAI(ctx, texts)
	case ProviderVoyage:
		return c.embedVoyage(ctx, texts)
	case ProviderCohere:
		return c.embedCohere(ctx, texts)
	case ProviderLocal:
		return c.embedLocal(ctx, texts)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", c.config.Provider)
	}
}

// openAIRequest is the request body for OpenAI embeddings API.
type openAIRequest struct {
	Input          []string `json:"input"`
	Model          string   `json:"model"`
	Dimensions     int      `json:"dimensions,omitempty"`
	EncodingFormat string   `json:"encoding_format,omitempty"`
}

// openAIResponse is the response from OpenAI embeddings API.
type openAIResponse struct {
	Data  []openAIEmbedding `json:"data"`
	Model string            `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

type openAIEmbedding struct {
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

func (c *Client) embedOpenAI(ctx context.Context, texts []string) ([]Embedding, error) {
	reqBody := openAIRequest{
		Input:          texts,
		Model:          c.config.Model,
		EncodingFormat: "float",
	}

	// Only set dimensions for models that support it
	if c.config.Model == "text-embedding-3-small" || c.config.Model == "text-embedding-3-large" {
		if c.config.Dimensions > 0 {
			reqBody.Dimensions = c.config.Dimensions
		}
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL+"/embeddings", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Limit response size to prevent OOM (10 MB max)
	const maxResponseSize = 10 * 1024 * 1024
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result openAIResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("API error: %s", result.Error.Message)
	}

	embeddings := make([]Embedding, len(result.Data))
	for _, d := range result.Data {
		if d.Index >= len(texts) {
			continue
		}
		embeddings[d.Index] = Embedding{
			Text:       texts[d.Index],
			Vector:     d.Embedding,
			Model:      result.Model,
			TokenCount: result.Usage.TotalTokens / len(texts), // Approximate per-text
		}
	}

	return embeddings, nil
}

// voyageRequest is the request body for Voyage AI embeddings API.
type voyageRequest struct {
	Input     []string `json:"input"`
	Model     string   `json:"model"`
	InputType string   `json:"input_type,omitempty"` // document or query
}

// voyageResponse is the response from Voyage AI embeddings API.
type voyageResponse struct {
	Data  []voyageEmbedding `json:"data"`
	Model string            `json:"model"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type voyageEmbedding struct {
	Embedding []float32 `json:"embedding"`
}

func (c *Client) embedVoyage(ctx context.Context, texts []string) ([]Embedding, error) {
	reqBody := voyageRequest{
		Input:     texts,
		Model:     c.config.Model,
		InputType: "document",
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL+"/embeddings", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Limit response size to prevent OOM (10 MB max)
	const maxResponseSize = 10 * 1024 * 1024
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result voyageResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("API error: %s", result.Error.Message)
	}

	embeddings := make([]Embedding, len(result.Data))
	for i, d := range result.Data {
		if i >= len(texts) {
			continue
		}
		embeddings[i] = Embedding{
			Text:       texts[i],
			Vector:     d.Embedding,
			Model:      result.Model,
			TokenCount: result.Usage.TotalTokens / len(texts),
		}
	}

	return embeddings, nil
}

// cohereRequest is the request body for Cohere embeddings API.
type cohereRequest struct {
	Texts     []string `json:"texts"`
	Model     string   `json:"model"`
	InputType string   `json:"input_type"` // search_document or search_query
}

// cohereResponse is the response from Cohere embeddings API.
type cohereResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
	Error      *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *Client) embedCohere(ctx context.Context, texts []string) ([]Embedding, error) {
	reqBody := cohereRequest{
		Texts:     texts,
		Model:     c.config.Model,
		InputType: "search_document",
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL+"/embed", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Limit response size to prevent OOM (10 MB max)
	const maxResponseSize = 10 * 1024 * 1024
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result cohereResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("API error: %s", result.Error.Message)
	}

	embeddings := make([]Embedding, len(result.Embeddings))
	for i, vec := range result.Embeddings {
		if i >= len(texts) {
			continue
		}
		embeddings[i] = Embedding{
			Text:   texts[i],
			Vector: vec,
			Model:  c.config.Model,
		}
	}

	return embeddings, nil
}

// localRequest is the request body for local embedding server.
type localRequest struct {
	Texts []string `json:"texts"`
}

// localResponse is the response from local embedding server.
type localResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
	Error      string      `json:"error,omitempty"`
}

func (c *Client) embedLocal(ctx context.Context, texts []string) ([]Embedding, error) {
	reqBody := localRequest{Texts: texts}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.BaseURL+"/embed", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Limit response size to prevent OOM (10 MB max)
	const maxResponseSize = 10 * 1024 * 1024
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result localResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if result.Error != "" {
		return nil, fmt.Errorf("local error: %s", result.Error)
	}

	embeddings := make([]Embedding, len(result.Embeddings))
	for i, vec := range result.Embeddings {
		if i >= len(texts) {
			continue
		}
		embeddings[i] = Embedding{
			Text:   texts[i],
			Vector: vec,
			Model:  c.config.Model,
		}
	}

	return embeddings, nil
}

// CosineSimilarity computes the cosine similarity between two vectors.
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return float32(dotProduct / (math.Sqrt(normA) * math.Sqrt(normB)))
}

// EuclideanDistance computes the Euclidean distance between two vectors.
func EuclideanDistance(a, b []float32) float32 {
	if len(a) != len(b) {
		return float32(math.MaxFloat32)
	}

	var sum float64
	for i := range a {
		diff := float64(a[i]) - float64(b[i])
		sum += diff * diff
	}

	return float32(math.Sqrt(sum))
}

// DotProduct computes the dot product of two vectors.
func DotProduct(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}

	var result float64
	for i := range a {
		result += float64(a[i]) * float64(b[i])
	}

	return float32(result)
}

// VectorStore provides persistent storage and retrieval of embeddings.
type VectorStore struct {
	mu         sync.RWMutex
	db         *sql.DB
	client     *Client
	tableName  string
	dimensions int
	logger     *slog.Logger
}

// VectorStoreConfig holds vector store configuration.
type VectorStoreConfig struct {
	DBPath     string
	TableName  string
	Client     *Client
	Dimensions int
	Logger     *slog.Logger
}

// NewVectorStore creates a new vector store backed by SQLite.
func NewVectorStore(cfg VectorStoreConfig) (*VectorStore, error) {
	if cfg.TableName == "" {
		cfg.TableName = "embeddings"
	}
	if cfg.Dimensions <= 0 {
		cfg.Dimensions = 1536
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0700); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	db, err := sql.Open("sqlite3", cfg.DBPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	store := &VectorStore{
		db:         db,
		client:     cfg.Client,
		tableName:  cfg.TableName,
		dimensions: cfg.Dimensions,
		logger:     cfg.Logger,
	}

	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}

	return store, nil
}

// initSchema creates the database tables if they don't exist.
func (s *VectorStore) initSchema() error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id TEXT PRIMARY KEY,
			content TEXT NOT NULL,
			embedding BLOB NOT NULL,
			metadata TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_%s_created ON %s(created_at);
	`, s.tableName, s.tableName, s.tableName)

	_, err := s.db.Exec(query)
	return err
}

// Entry represents a stored embedding entry.
type Entry struct {
	ID        string                 `json:"id"`
	Content   string                 `json:"content"`
	Embedding []float32              `json:"embedding,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// Add stores a new entry, generating the embedding if client is available.
func (s *VectorStore) Add(ctx context.Context, id, content string, metadata map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Generate embedding
	var embedding []float32
	if s.client != nil {
		emb, err := s.client.EmbedOne(ctx, content)
		if err != nil {
			return fmt.Errorf("generate embedding: %w", err)
		}
		embedding = emb.Vector
	}

	return s.addWithEmbedding(id, content, embedding, metadata)
}

// AddWithEmbedding stores an entry with a pre-computed embedding.
func (s *VectorStore) AddWithEmbedding(id, content string, embedding []float32, metadata map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addWithEmbedding(id, content, embedding, metadata)
}

func (s *VectorStore) addWithEmbedding(id, content string, embedding []float32, metadata map[string]interface{}) error {
	embData, err := json.Marshal(embedding)
	if err != nil {
		return fmt.Errorf("marshal embedding: %w", err)
	}

	metaData, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	query := fmt.Sprintf(`
		INSERT OR REPLACE INTO %s (id, content, embedding, metadata, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, s.tableName)

	_, err = s.db.Exec(query, id, content, embData, string(metaData))
	return err
}

// Get retrieves an entry by ID.
func (s *VectorStore) Get(id string) (*Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := fmt.Sprintf(`
		SELECT id, content, embedding, metadata, created_at, updated_at
		FROM %s WHERE id = ?
	`, s.tableName)

	var entry Entry
	var embData []byte
	var metaData string
	err := s.db.QueryRow(query, id).Scan(
		&entry.ID, &entry.Content, &embData, &metaData,
		&entry.CreatedAt, &entry.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(embData, &entry.Embedding); err != nil {
		return nil, fmt.Errorf("unmarshal embedding: %w", err)
	}

	if metaData != "" {
		if err := json.Unmarshal([]byte(metaData), &entry.Metadata); err != nil {
			s.logger.Warn("failed to unmarshal metadata", "id", id, "error", err)
		}
	}

	return &entry, nil
}

// Delete removes an entry by ID.
func (s *VectorStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := fmt.Sprintf("DELETE FROM %s WHERE id = ?", s.tableName)
	_, err := s.db.Exec(query, id)
	return err
}

// SearchResult represents a search result with similarity score.
type SearchResult struct {
	Entry      *Entry  `json:"entry"`
	Similarity float32 `json:"similarity"`
}

// Search finds similar entries to the query text.
func (s *VectorStore) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if s.client == nil {
		return nil, fmt.Errorf("no embedding client configured")
	}

	// Generate query embedding
	emb, err := s.client.EmbedOne(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("generate query embedding: %w", err)
	}

	return s.SearchByVector(emb.Vector, limit)
}

// MaxSearchEntries is the maximum number of entries to scan during search.
// This provides a safeguard against OOM for large datasets.
const MaxSearchEntries = 10000

// SearchByVector finds similar entries to a query vector.
func (s *VectorStore) SearchByVector(queryVector []float32, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Load entries with limit to prevent OOM
	// Note: For large datasets, consider using a specialized vector database
	query := fmt.Sprintf(`
		SELECT id, content, embedding, metadata, created_at, updated_at
		FROM %s ORDER BY created_at DESC LIMIT %d
	`, s.tableName, MaxSearchEntries)

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult

	for rows.Next() {
		var entry Entry
		var embData []byte
		var metaData string

		err := rows.Scan(&entry.ID, &entry.Content, &embData, &metaData,
			&entry.CreatedAt, &entry.UpdatedAt)
		if err != nil {
			continue
		}

		if err := json.Unmarshal(embData, &entry.Embedding); err != nil {
			continue
		}

		if metaData != "" {
_ = json.Unmarshal([]byte(metaData), &entry.Metadata)
		}

		similarity := CosineSimilarity(queryVector, entry.Embedding)

		results = append(results, SearchResult{
			Entry:      &entry,
			Similarity: similarity,
		})
	}

	// Sort by similarity (descending)
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Similarity > results[i].Similarity {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	// Limit results
	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// Count returns the number of stored entries.
func (s *VectorStore) Count() (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", s.tableName)
	var count int
	err := s.db.QueryRow(query).Scan(&count)
	return count, err
}

// Clear removes all entries.
func (s *VectorStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := fmt.Sprintf("DELETE FROM %s", s.tableName)
	_, err := s.db.Exec(query)
	return err
}

// Close closes the database connection.
func (s *VectorStore) Close() error {
	return s.db.Close()
}

// List returns all entries (paginated).
func (s *VectorStore) List(offset, limit int) ([]*Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	query := fmt.Sprintf(`
		SELECT id, content, embedding, metadata, created_at, updated_at
		FROM %s ORDER BY created_at DESC LIMIT ? OFFSET ?
	`, s.tableName)

	rows, err := s.db.Query(query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*Entry

	for rows.Next() {
		var entry Entry
		var embData []byte
		var metaData string

		err := rows.Scan(&entry.ID, &entry.Content, &embData, &metaData,
			&entry.CreatedAt, &entry.UpdatedAt)
		if err != nil {
			continue
		}

		if err := json.Unmarshal(embData, &entry.Embedding); err != nil {
			continue
		}

		if metaData != "" {
_ = json.Unmarshal([]byte(metaData), &entry.Metadata)
		}

		entries = append(entries, &entry)
	}

	return entries, nil
}

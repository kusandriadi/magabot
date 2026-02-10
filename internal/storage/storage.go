// Package storage handles persistent data storage with SQLite
package storage

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Store manages persistent storage
type Store struct {
	db *sql.DB
}

// Message represents a chat message
type Message struct {
	ID        int64
	Platform  string
	ChatID    string
	UserID    string
	Username  string
	Content   string    // Encrypted
	Timestamp time.Time
	Direction string    // "in" or "out"
}

// Session represents a platform session
type Session struct {
	ID        int64
	Platform  string
	Data      string    // Encrypted session data
	UpdatedAt time.Time
}

// New creates a new storage instance
func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}

	// SQLite only supports one writer; limit pool accordingly
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	// Set pragmas for security and performance
	pragmas := []string{
		"PRAGMA secure_delete = ON",
		"PRAGMA auto_vacuum = INCREMENTAL",
		"PRAGMA temp_store = MEMORY",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("set pragma: %w", err)
		}
	}

	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

// migrate runs database migrations
func (s *Store) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			platform TEXT NOT NULL,
			chat_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			username TEXT,
			content TEXT NOT NULL,
			timestamp DATETIME NOT NULL,
			direction TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_platform_chat ON messages(platform, chat_id)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_timestamp ON messages(timestamp)`,
		
		`CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			platform TEXT NOT NULL UNIQUE,
			data TEXT NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
		
		`CREATE TABLE IF NOT EXISTS config (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		
		`CREATE TABLE IF NOT EXISTS audit_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			platform TEXT,
			user_id TEXT,
			action TEXT NOT NULL,
			details TEXT
		)`,
	}

	for _, m := range migrations {
		if _, err := s.db.Exec(m); err != nil {
			return fmt.Errorf("migration: %w", err)
		}
	}

	return nil
}

// Close closes the database
func (s *Store) Close() error {
	return s.db.Close()
}

// SaveMessage saves a message
func (s *Store) SaveMessage(msg *Message) error {
	_, err := s.db.Exec(
		`INSERT INTO messages (platform, chat_id, user_id, username, content, timestamp, direction)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		msg.Platform, msg.ChatID, msg.UserID, msg.Username, msg.Content, msg.Timestamp, msg.Direction,
	)
	return err
}

// GetMessages retrieves messages for a chat
func (s *Store) GetMessages(platform, chatID string, limit int) ([]Message, error) {
	rows, err := s.db.Query(
		`SELECT id, platform, chat_id, user_id, username, content, timestamp, direction
		 FROM messages WHERE platform = ? AND chat_id = ?
		 ORDER BY timestamp DESC LIMIT ?`,
		platform, chatID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.Platform, &m.ChatID, &m.UserID, &m.Username, &m.Content, &m.Timestamp, &m.Direction); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}

	return messages, rows.Err()
}

// SaveSession saves a platform session
func (s *Store) SaveSession(platform, data string) error {
	_, err := s.db.Exec(
		`INSERT INTO sessions (platform, data, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(platform) DO UPDATE SET data = ?, updated_at = ?`,
		platform, data, time.Now(), data, time.Now(),
	)
	return err
}

// GetSession retrieves a platform session
func (s *Store) GetSession(platform string) (string, error) {
	var data string
	err := s.db.QueryRow(
		`SELECT data FROM sessions WHERE platform = ?`, platform,
	).Scan(&data)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return data, err
}

// DeleteSession deletes a platform session
func (s *Store) DeleteSession(platform string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE platform = ?`, platform)
	return err
}

// SetConfig saves a config value
func (s *Store) SetConfig(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO config (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(key) DO UPDATE SET value = ?, updated_at = CURRENT_TIMESTAMP`,
		key, value, value,
	)
	return err
}

// GetConfig retrieves a config value
func (s *Store) GetConfig(key string) (string, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM config WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// AuditLog records an audit event
func (s *Store) AuditLog(platform, userID, action, details string) error {
	_, err := s.db.Exec(
		`INSERT INTO audit_log (platform, user_id, action, details) VALUES (?, ?, ?, ?)`,
		platform, userID, action, details,
	)
	return err
}

// PurgeOldMessages deletes messages older than retention days
func (s *Store) PurgeOldMessages(retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		return 0, nil
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	result, err := s.db.Exec(`DELETE FROM messages WHERE timestamp < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// Stats returns storage statistics
func (s *Store) Stats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Message counts per platform
	rows, err := s.db.Query(`SELECT platform, COUNT(*) FROM messages GROUP BY platform`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	msgCounts := make(map[string]int64)
	for rows.Next() {
		var platform string
		var count int64
		if err := rows.Scan(&platform, &count); err != nil {
			return nil, err
		}
		msgCounts[platform] = count
	}
	stats["messages"] = msgCounts

	// Sessions
	var sessionCount int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&sessionCount); err != nil {
		return nil, fmt.Errorf("count sessions: %w", err)
	}
	stats["sessions"] = sessionCount

	// Database size
	var pageCount, pageSize int64
	if err := s.db.QueryRow(`PRAGMA page_count`).Scan(&pageCount); err != nil {
		return nil, fmt.Errorf("page_count: %w", err)
	}
	if err := s.db.QueryRow(`PRAGMA page_size`).Scan(&pageSize); err != nil {
		return nil, fmt.Errorf("page_size: %w", err)
	}
	stats["db_size_bytes"] = pageCount * pageSize

	return stats, nil
}

// Vacuum optimizes the database
func (s *Store) Vacuum() error {
	_, err := s.db.Exec(`VACUUM`)
	return err
}

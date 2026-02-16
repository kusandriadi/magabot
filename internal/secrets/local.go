// Local file-based secrets backend
package secrets

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// Local is a file-based secrets backend
type Local struct {
	path    string
	secrets map[string]string
	mu      sync.RWMutex
}

// LocalConfig for local backend
type LocalConfig struct {
	Path string `yaml:"path"` // Default: ~/.magabot/secrets.json
}

// NewLocal creates a new local secrets backend
func NewLocal(cfg *LocalConfig) (*Local, error) {
	path := cfg.Path
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".magabot", "secrets.json")
	}

	l := &Local{
		path:    path,
		secrets: make(map[string]string),
	}

	// Load existing secrets
	if err := l.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return l, nil
}

// Name returns the backend name
func (l *Local) Name() string {
	return "local"
}

// Get retrieves a secret
func (l *Local) Get(ctx context.Context, key string) (string, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	value, ok := l.secrets[key]
	if !ok {
		return "", ErrNotFound
	}
	return value, nil
}

// Set stores a secret
func (l *Local) Set(ctx context.Context, key, value string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.secrets[key] = value
	return l.save()
}

// Delete removes a secret
func (l *Local) Delete(ctx context.Context, key string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.secrets, key)
	return l.save()
}

// List returns all secret keys
func (l *Local) List(ctx context.Context) ([]string, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	keys := make([]string, 0, len(l.secrets))
	for k := range l.secrets {
		keys = append(keys, k)
	}
	return keys, nil
}

// Ping checks if the backend is available
func (l *Local) Ping(ctx context.Context) error {
	// Local is always available
	return nil
}

// load reads secrets from file
func (l *Local) load() error {
	data, err := os.ReadFile(l.path)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &l.secrets)
}

// save writes secrets to file atomically (write-to-temp-then-rename)
func (l *Local) save() error {
	// Ensure directory exists
	dir := filepath.Dir(l.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(l.secrets, "", "  ")
	if err != nil {
		return err
	}

	// Atomic write: write to temp file then rename to prevent partial writes
	tmp, err := os.CreateTemp(dir, ".secrets-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	// Set restrictive permissions on Unix; Windows uses ACLs instead.
	if runtime.GOOS != "windows" {
		if err := os.Chmod(tmpName, 0600); err != nil {
			os.Remove(tmpName)
			return err
		}
	}

	return os.Rename(tmpName, l.path)
}

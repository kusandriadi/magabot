// Chain backend tries multiple backends in sequence
package secrets

import (
	"context"
	"fmt"
	"strings"
)

// Chain is a backend that tries multiple backends in sequence
type Chain struct {
	backends []Backend
}

// NewChain creates a new chain backend
func NewChain(backends ...Backend) *Chain {
	return &Chain{
		backends: backends,
	}
}

// Name returns the backend name
func (c *Chain) Name() string {
	names := make([]string, len(c.backends))
	for i, b := range c.backends {
		names[i] = b.Name()
	}
	return fmt.Sprintf("chain[%s]", strings.Join(names, ","))
}

// Get retrieves a secret from the first backend that has it
func (c *Chain) Get(ctx context.Context, key string) (string, error) {
	for _, backend := range c.backends {
		value, err := backend.Get(ctx, key)
		if err == nil {
			return value, nil
		}
		// Continue to next backend if not found
		if err != ErrNotFound {
			// Log other errors but continue
			continue
		}
	}
	return "", ErrNotFound
}

// Set stores a secret in the first backend
func (c *Chain) Set(ctx context.Context, key, value string) error {
	if len(c.backends) == 0 {
		return ErrBackendError
	}
	return c.backends[0].Set(ctx, key, value)
}

// Delete removes a secret from the first backend
func (c *Chain) Delete(ctx context.Context, key string) error {
	if len(c.backends) == 0 {
		return ErrBackendError
	}
	return c.backends[0].Delete(ctx, key)
}

// List returns all secret keys from the first backend
func (c *Chain) List(ctx context.Context) ([]string, error) {
	if len(c.backends) == 0 {
		return nil, ErrBackendError
	}
	return c.backends[0].List(ctx)
}

// Ping checks if at least one backend is available
func (c *Chain) Ping(ctx context.Context) error {
	for _, backend := range c.backends {
		if err := backend.Ping(ctx); err == nil {
			return nil
		}
	}
	return ErrBackendError
}

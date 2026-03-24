// Package platform provides shared types for chat platform implementations.
package platform

import (
	"sync"

	"github.com/kusa/magabot/internal/router"
)

// Base contains the message-handler plumbing shared by every platform.
// Embed it in your Bot/Server struct to get thread-safe SetHandler/GetHandler.
type Base struct {
	handler   router.MessageHandler
	handlerMu sync.RWMutex
}

// SetHandler sets the message handler (thread-safe).
func (b *Base) SetHandler(h router.MessageHandler) {
	b.handlerMu.Lock()
	b.handler = h
	b.handlerMu.Unlock()
}

// GetHandler returns the current message handler (thread-safe).
func (b *Base) GetHandler() router.MessageHandler {
	b.handlerMu.RLock()
	h := b.handler
	b.handlerMu.RUnlock()
	return h
}

package platform

// MessageHandler is the function signature for message handlers
type MessageHandler func(msg *Message) (string, error)

// Message represents a platform-agnostic message
type Message struct {
	Platform  string      // telegram, discord, slack, whatsapp, lark, webhook
	ChatID    string      // Channel/chat identifier
	MessageID string      // Message identifier
	UserID    string      // Sender identifier
	Username  string      // Sender display name
	Text      string      // Message text content
	IsGroup   bool        // Whether message is from a group
	GuildID   string      // Discord guild ID (if applicable)
	ReplyToID string      // ID of message being replied to
	Raw       interface{} // Original platform-specific message
}

// Adapter is the interface all platform adapters must implement
type Adapter interface {
	// Start begins listening for messages
	Start(handler MessageHandler) error
	
	// Stop shuts down the adapter
	Stop() error
	
	// Send sends a message to a chat
	Send(chatID, message string) error
	
	// Name returns the platform name
	Name() string
	
	// IsRunning returns whether the adapter is active
	IsRunning() bool
}

// ReplyAdapter extends Adapter with reply capability
type ReplyAdapter interface {
	Adapter
	Reply(chatID, messageID, message string) error
}

// Manager manages multiple platform adapters
type Manager struct {
	adapters map[string]Adapter
	handler  MessageHandler
}

// NewManager creates a new platform manager
func NewManager() *Manager {
	return &Manager{
		adapters: make(map[string]Adapter),
	}
}

// Register adds an adapter to the manager
func (m *Manager) Register(adapter Adapter) {
	m.adapters[adapter.Name()] = adapter
}

// SetHandler sets the global message handler
func (m *Manager) SetHandler(handler MessageHandler) {
	m.handler = handler
}

// StartAll starts all registered adapters
func (m *Manager) StartAll() error {
	for name, adapter := range m.adapters {
		if err := adapter.Start(m.handler); err != nil {
			return &PlatformError{Platform: name, Err: err}
		}
	}
	return nil
}

// StopAll stops all registered adapters
func (m *Manager) StopAll() {
	for _, adapter := range m.adapters {
		adapter.Stop()
	}
}

// Get returns an adapter by name
func (m *Manager) Get(name string) Adapter {
	return m.adapters[name]
}

// Send sends a message via a specific platform
func (m *Manager) Send(platform, chatID, message string) error {
	adapter := m.adapters[platform]
	if adapter == nil {
		return &PlatformError{Platform: platform, Err: ErrPlatformNotFound}
	}
	return adapter.Send(chatID, message)
}

// Broadcast sends a message to all specified targets
func (m *Manager) Broadcast(targets []Target, message string) []error {
	var errors []error
	for _, t := range targets {
		if err := m.Send(t.Platform, t.ChatID, message); err != nil {
			errors = append(errors, err)
		}
	}
	return errors
}

// Target represents a messaging destination
type Target struct {
	Platform string `json:"platform" yaml:"platform"`
	ChatID   string `json:"chat_id" yaml:"chat_id"`
	Name     string `json:"name" yaml:"name"` // Friendly name
}

// PlatformError represents a platform-specific error
type PlatformError struct {
	Platform string
	Err      error
}

func (e *PlatformError) Error() string {
	return e.Platform + ": " + e.Err.Error()
}

// Common errors
var (
	ErrPlatformNotFound = &PlatformError{Err: nil}
)

func init() {
	ErrPlatformNotFound.Err = nil // Will be set to actual error message
}

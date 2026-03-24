// Package whatsapp provides WhatsApp integration via whatsmeow (multi-device)
package whatsapp

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	"github.com/kusa/magabot/internal/router"
)

// Bot represents a WhatsApp bot using whatsmeow
type Bot struct {
	client    *whatsmeow.Client
	container *sqlstore.Container
	handler   router.MessageHandler
	handlerMu sync.RWMutex
	logger    *slog.Logger
	dbPath    string
	done      chan struct{}
	mu        sync.RWMutex // protects client
	wg        sync.WaitGroup
}

// Config for WhatsApp bot
type Config struct {
	DBPath string // Path to SQLite database for whatsmeow session
	Logger *slog.Logger
}

// New creates a new WhatsApp bot
func New(cfg *Config) (*Bot, error) {
	dbPath := cfg.DBPath
	if dbPath == "" {
		home, _ := os.UserHomeDir()
		dbPath = filepath.Join(home, ".magabot", "whatsapp.db")
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	dbURI := fmt.Sprintf("file:%s?_foreign_keys=on", dbPath)
	container, err := sqlstore.New(context.Background(), "sqlite3", dbURI, waLog.Noop)
	if err != nil {
		return nil, fmt.Errorf("create whatsmeow store: %w", err)
	}

	return &Bot{
		container: container,
		logger:    cfg.Logger,
		dbPath:    dbPath,
		done:      make(chan struct{}),
	}, nil
}

// Name returns the platform name
func (b *Bot) Name() string {
	return "whatsapp"
}

// Start starts the WhatsApp client
func (b *Bot) Start(ctx context.Context) error {
	deviceStore, err := b.container.GetFirstDevice(ctx)
	if err != nil {
		return fmt.Errorf("get device: %w", err)
	}

	client := whatsmeow.NewClient(deviceStore, waLog.Noop)
	client.AddEventHandler(b.eventHandler)
	client.EnableAutoReconnect = true

	b.mu.Lock()
	b.client = client
	b.mu.Unlock()

	if client.Store.ID == nil {
		// New device — need QR code pairing
		qrChan, err := client.GetQRChannel(ctx)
		if err != nil {
			return fmt.Errorf("get QR channel: %w", err)
		}

		if err := client.Connect(); err != nil {
			return fmt.Errorf("connect: %w", err)
		}

		b.wg.Add(1)
		go func() {
			defer b.wg.Done()
			for evt := range qrChan {
				switch evt.Event {
				case "code":
					b.logger.Info("scan QR code to link WhatsApp device")
					qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stderr)
				case "success":
					b.logger.Info("WhatsApp pairing successful")
				case "timeout":
					b.logger.Warn("QR code expired, reconnect to get a new one")
				}
			}
		}()
	} else {
		// Already paired
		if err := client.Connect(); err != nil {
			return fmt.Errorf("connect: %w", err)
		}
	}

	b.logger.Info("whatsapp client started", "db", b.dbPath)
	return nil
}

// Stop stops the WhatsApp client
func (b *Bot) Stop() error {
	close(b.done)

	b.mu.RLock()
	client := b.client
	b.mu.RUnlock()

	if client != nil {
		client.Disconnect()
	}

	b.wg.Wait()
	return nil
}

// Send sends a text message to a WhatsApp chat
func (b *Bot) Send(chatID, message string) error {
	b.mu.RLock()
	client := b.client
	b.mu.RUnlock()

	if client == nil || !client.IsConnected() {
		return fmt.Errorf("WhatsApp not connected")
	}

	jid, err := types.ParseJID(chatID)
	if err != nil {
		return fmt.Errorf("invalid chat ID %q: %w", chatID, err)
	}

	_, err = client.SendMessage(context.Background(), jid, &waE2E.Message{
		Conversation: proto.String(message),
	})
	return err
}

// SetHandler sets the message handler
func (b *Bot) SetHandler(h router.MessageHandler) {
	b.handlerMu.Lock()
	b.handler = h
	b.handlerMu.Unlock()
}

// IsConnected returns connection status
func (b *Bot) IsConnected() bool {
	b.mu.RLock()
	client := b.client
	b.mu.RUnlock()
	return client != nil && client.IsConnected()
}

// eventHandler dispatches whatsmeow events
func (b *Bot) eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		b.handleMessage(v)
	case *events.Connected:
		b.logger.Info("connected to WhatsApp")
	case *events.Disconnected:
		b.logger.Info("disconnected from WhatsApp (auto-reconnect enabled)")
	case *events.LoggedOut:
		b.logger.Warn("logged out from WhatsApp — re-run setup to pair again")
	}
}

// handleMessage processes an incoming WhatsApp message
func (b *Bot) handleMessage(evt *events.Message) {
	b.handlerMu.RLock()
	handler := b.handler
	b.handlerMu.RUnlock()

	if handler == nil {
		return
	}

	// Skip own messages
	if evt.Info.IsFromMe {
		return
	}

	// Skip status broadcasts
	if evt.Info.Chat.Server == "broadcast" {
		return
	}

	content := extractContent(evt)
	if content == "" {
		return
	}

	chatID := evt.Info.Chat.String()
	userID := evt.Info.Sender.String()

	msg := &router.Message{
		Platform:  "whatsapp",
		ChatID:    chatID,
		UserID:    userID,
		Username:  evt.Info.PushName,
		Text:      content,
		Timestamp: evt.Info.Timestamp,
	}

	// Extract reply context if this message quotes another
	if rc := extractReplyContext(evt, b); rc != nil {
		msg.ReplyTo = rc
	}

	ctx := context.Background()

	// Send typing indicator
	b.mu.RLock()
	client := b.client
	b.mu.RUnlock()
	if client != nil && client.IsConnected() {
		_ = client.SendChatPresence(ctx, evt.Info.Chat, types.ChatPresenceComposing, types.ChatPresenceMediaText)
	}

	// Set up streaming callback — send new messages progressively (no editing)
	var (
		lastSend    time.Time
		lastSentLen int // length of text already sent in previous messages
		streamed    bool
	)
	const streamSendInterval = 3 * time.Second // throttle between sends

	jid := evt.Info.Chat
	msg.StreamCallback = func(text string) {
		if client == nil || !client.IsConnected() {
			return
		}
		now := time.Now()

		// Throttle sends
		if now.Sub(lastSend) < streamSendInterval {
			return
		}

		// Only send the new portion since last send
		currentText := text[lastSentLen:]
		if currentText == "" {
			return
		}

		if _, err := client.SendMessage(ctx, jid, &waE2E.Message{
			Conversation: proto.String(currentText),
		}); err != nil {
			b.logger.Debug("stream: send failed", "error", err)
			return
		}
		lastSentLen = len(text)
		lastSend = now
		streamed = true
	}

	response, err := handler(ctx, msg)
	if err != nil {
		b.logger.Debug("handler error", "error", err)
		return
	}

	// Clear typing indicator
	if client != nil && client.IsConnected() {
		_ = client.SendChatPresence(ctx, evt.Info.Chat, types.ChatPresencePaused, types.ChatPresenceMediaText)
	}

	if response == "" {
		return
	}

	// Send the remaining text not yet delivered during streaming
	finalText := response
	if lastSentLen > 0 && lastSentLen < len(response) {
		finalText = response[lastSentLen:]
	} else if lastSentLen >= len(response) {
		return // everything was already sent during streaming
	}

	if streamed {
		// Send remainder as new message
		if client != nil && client.IsConnected() {
			if _, err := client.SendMessage(ctx, jid, &waE2E.Message{
				Conversation: proto.String(finalText),
			}); err != nil {
				b.logger.Error("stream: send final failed", "error", err)
			}
		}
	} else {
		// No streaming (command, agent, etc.) — send as before
		if err := b.Send(chatID, response); err != nil {
			b.logger.Error("send reply failed", "error", err)
		}
	}
}

// extractContent extracts text content from a WhatsApp message
func extractContent(evt *events.Message) string {
	msg := evt.Message
	if msg == nil {
		return ""
	}

	// Text message
	if msg.GetConversation() != "" {
		return msg.GetConversation()
	}

	// Extended text (reply, link preview)
	if ext := msg.GetExtendedTextMessage(); ext != nil && ext.GetText() != "" {
		return ext.GetText()
	}

	// Image with caption
	if img := msg.GetImageMessage(); img != nil {
		if caption := img.GetCaption(); caption != "" {
			return "[Image] " + caption
		}
		return "[Image]"
	}

	// Video with caption
	if vid := msg.GetVideoMessage(); vid != nil {
		if caption := vid.GetCaption(); caption != "" {
			return "[Video] " + caption
		}
		return "[Video]"
	}

	// Document with caption
	if doc := msg.GetDocumentMessage(); doc != nil {
		if caption := doc.GetCaption(); caption != "" {
			return "[Document] " + caption
		}
		return "[Document]"
	}

	// Voice/Audio message
	if msg.GetAudioMessage() != nil {
		return "[Voice Message]"
	}

	// Sticker
	if msg.GetStickerMessage() != nil {
		return "[Sticker]"
	}

	// Location
	if loc := msg.GetLocationMessage(); loc != nil {
		return fmt.Sprintf("[Location: %.6f, %.6f]", loc.GetDegreesLatitude(), loc.GetDegreesLongitude())
	}

	// Contact
	if msg.GetContactMessage() != nil {
		return "[Contact]"
	}

	return ""
}

// extractReplyContext extracts the quoted message context from a WhatsApp reply
func extractReplyContext(evt *events.Message, b *Bot) *router.ReplyContext {
	msg := evt.Message
	if msg == nil {
		return nil
	}

	// Get ContextInfo from whichever message type has it
	var ci *waE2E.ContextInfo
	if ext := msg.GetExtendedTextMessage(); ext != nil {
		ci = ext.GetContextInfo()
	} else if img := msg.GetImageMessage(); img != nil {
		ci = img.GetContextInfo()
	} else if vid := msg.GetVideoMessage(); vid != nil {
		ci = vid.GetContextInfo()
	} else if doc := msg.GetDocumentMessage(); doc != nil {
		ci = doc.GetContextInfo()
	} else if aud := msg.GetAudioMessage(); aud != nil {
		ci = aud.GetContextInfo()
	}

	if ci == nil || ci.GetQuotedMessage() == nil {
		return nil
	}

	// Extract text from the quoted message
	quoted := ci.GetQuotedMessage()
	var quotedText string
	if quoted.GetConversation() != "" {
		quotedText = quoted.GetConversation()
	} else if ext := quoted.GetExtendedTextMessage(); ext != nil {
		quotedText = ext.GetText()
	} else if img := quoted.GetImageMessage(); img != nil && img.GetCaption() != "" {
		quotedText = "[Image] " + img.GetCaption()
	} else if vid := quoted.GetVideoMessage(); vid != nil && vid.GetCaption() != "" {
		quotedText = "[Video] " + vid.GetCaption()
	}

	if quotedText == "" {
		return nil
	}

	// Determine who sent the quoted message
	participant := ci.GetParticipant()
	var username string
	var isBot bool

	b.mu.RLock()
	client := b.client
	b.mu.RUnlock()

	if client != nil && participant != "" {
		jid, err := types.ParseJID(participant)
		if err == nil && jid.User == client.Store.ID.User {
			isBot = true
			username = "bot"
		}
	}

	return &router.ReplyContext{
		Text:     quotedText,
		Username: username,
		IsBot:    isBot,
	}
}

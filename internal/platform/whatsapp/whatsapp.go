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
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	"github.com/kusa/magabot/internal/platform"
	"github.com/kusa/magabot/internal/router"
	"github.com/kusa/magabot/internal/util"
)

const maxVoiceDownloadSize = 20 * 1024 * 1024 // 20 MB

// Bot represents a WhatsApp bot using whatsmeow
type Bot struct {
	platform.Base
	client        *whatsmeow.Client
	container     *sqlstore.Container
	logger        *slog.Logger
	dataDir       string // platform-specific data dir (e.g. data/platform/whatsapp)
	downloadsDir  string // where downloaded voice files are saved
	onPairFailure func()
	done          chan struct{}
	mu            sync.RWMutex // protects client
	wg            sync.WaitGroup
}

// Config for WhatsApp bot
type Config struct {
	DataDir       string // Platform data directory (DB + QR file live here)
	DownloadsDir  string // Directory for downloaded voice files
	OnPairFailure func() // Called when QR pairing fails after all retries
	Logger        *slog.Logger
}

// New creates a new WhatsApp bot
func New(cfg *Config) (*Bot, error) {
	dataDir := cfg.DataDir
	if dataDir == "" {
		home, _ := os.UserHomeDir()
		dataDir = filepath.Join(home, ".magabot", "data", "platform", "whatsapp")
	}

	// Ensure directory exists
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	dbPath := filepath.Join(dataDir, "whatsapp.db")
	dbURI := fmt.Sprintf("file:%s?_foreign_keys=on", dbPath)
	container, err := sqlstore.New(context.Background(), "sqlite3", dbURI, waLog.Noop)
	if err != nil {
		return nil, fmt.Errorf("create whatsmeow store: %w", err)
	}

	downloadsDir := cfg.DownloadsDir
	if downloadsDir == "" {
		downloadsDir = filepath.Join(dataDir, "downloads")
	}

	return &Bot{
		container:     container,
		logger:        cfg.Logger,
		dataDir:       dataDir,
		downloadsDir:  downloadsDir,
		onPairFailure: cfg.OnPairFailure,
		done:          make(chan struct{}),
	}, nil
}

// getClient returns a snapshot of the current WhatsApp client (thread-safe).
func (b *Bot) getClient() *whatsmeow.Client {
	b.mu.RLock()
	c := b.client
	b.mu.RUnlock()
	return c
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
		// New device — need QR code pairing (with auto-retry on expiry)
		b.wg.Add(1)
		go b.qrLoop(ctx)
	} else {
		// Already paired
		if err := client.Connect(); err != nil {
			return fmt.Errorf("connect: %w", err)
		}
	}

	b.logger.Info("whatsapp client started", "data_dir", b.dataDir)
	return nil
}

// qrLoop handles QR pairing with automatic retry on expiry (max 3 attempts).
func (b *Bot) qrLoop(ctx context.Context) {
	defer b.wg.Done()
	qrFile := filepath.Join(b.dataDir, "qr.txt")
	const maxRetries = 3

	for attempt := 0; attempt < maxRetries; attempt++ {
		client := b.getClient()

		qrChan, err := client.GetQRChannel(ctx)
		if err != nil {
			b.logger.Error("get QR channel failed", "error", err)
			return
		}

		if err := client.Connect(); err != nil {
			b.logger.Error("connect for QR pairing failed", "error", err)
			return
		}

		retry := false
		for evt := range qrChan {
			switch evt.Event {
			case "code":
				b.logger.Info("scan QR code to link WhatsApp device, run 'magabot qr' to display")
				_ = os.WriteFile(qrFile, []byte(evt.Code), 0600)
			case "success":
				b.logger.Info("WhatsApp pairing successful")
				_ = os.Remove(qrFile)
				return
			case "timeout":
				remaining := maxRetries - attempt - 1
				if remaining > 0 {
					b.logger.Info("QR code expired, generating new one...", "retries_left", remaining)
				}
				_ = os.Remove(qrFile)
				retry = true
			}
		}

		if !retry {
			return
		}

		// Disconnect before reconnecting for a fresh QR channel
		client.Disconnect()

		// Check if shutdown was requested
		select {
		case <-b.done:
			return
		case <-time.After(2 * time.Second):
		}
	}

	// All retries exhausted — disable WhatsApp
	b.logger.Warn("WhatsApp QR pairing failed after 3 attempts, disabling platform — run 'magabot setup platform' to re-enable")
	client := b.getClient()
	if client != nil {
		client.Disconnect()
	}
	if b.onPairFailure != nil {
		b.onPairFailure()
	}
}

// Stop stops the WhatsApp client
func (b *Bot) Stop() error {
	close(b.done)

	if client := b.getClient(); client != nil {
		client.Disconnect()
	}

	b.wg.Wait()
	return nil
}

// Send sends a text message to a WhatsApp chat
func (b *Bot) Send(chatID, message string) error {
	client := b.getClient()
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

// SendVoice uploads and sends an OGG Opus audio as a WhatsApp PTT voice message.
func (b *Bot) SendVoice(chatID string, audio []byte) error {
	client := b.getClient()
	if client == nil || !client.IsConnected() {
		return fmt.Errorf("WhatsApp not connected")
	}

	jid, err := types.ParseJID(chatID)
	if err != nil {
		return fmt.Errorf("invalid chat ID %q: %w", chatID, err)
	}

	uploaded, err := client.Upload(context.Background(), audio, whatsmeow.MediaAudio)
	if err != nil {
		return fmt.Errorf("upload audio: %w", err)
	}

	// Flat waveform (64 amplitude samples, 0-100)
	waveform := make([]byte, 64)
	for i := range waveform {
		waveform[i] = 50
	}

	_, err = client.SendMessage(context.Background(), jid, &waE2E.Message{
		AudioMessage: &waE2E.AudioMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(audio))),
			Mimetype:      proto.String("audio/ogg; codecs=opus"),
			PTT:           proto.Bool(true),
			Waveform:      waveform,
		},
	})
	return err
}

// SetHandler is provided by platform.Base.

// IsConnected returns connection status
func (b *Bot) IsConnected() bool {
	client := b.getClient()
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
	handler := b.GetHandler()
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

	// Handle voice/audio: download and save to disk so daemon can transcribe it
	var media []string
	client := b.getClient()
	if evt.Message != nil && evt.Message.GetAudioMessage() != nil {
		if content == "" {
			content = "[Voice Message]"
		}
		if client != nil {
			audioData, err := client.Download(context.Background(), evt.Message.GetAudioMessage())
			if err != nil {
				b.logger.Warn("download voice failed", "error", err)
			} else if len(audioData) > maxVoiceDownloadSize {
				b.logger.Warn("voice file too large, skipping", "size", len(audioData))
			} else if len(audioData) > 0 {
				if path, err := b.saveVoice(audioData); err != nil {
					b.logger.Warn("save voice failed", "error", err)
				} else {
					media = append(media, path)
				}
			}
		}
	}

	if content == "" {
		return
	}

	chatID := evt.Info.Chat.String()
	// ToNonAD strips the device suffix (e.g. "628...:5@s.whatsapp.net" → "628...@s.whatsapp.net")
	// so it matches the JID stored in config and correctly identifies DMs vs groups.
	userID := evt.Info.Sender.ToNonAD().String()

	msg := &router.Message{
		Platform:  "whatsapp",
		ChatID:    chatID,
		UserID:    userID,
		Username:  evt.Info.PushName,
		Text:      content,
		Media:     media,
		Timestamp: evt.Info.Timestamp,
	}

	// Extract reply context if this message quotes another
	if rc := extractReplyContext(evt, b); rc != nil {
		msg.ReplyTo = rc
	}

	ctx := context.Background()

	// Mark incoming message as read
	if client != nil && client.IsConnected() {
		_ = client.MarkRead(ctx, []types.MessageID{evt.Info.ID}, time.Now(), evt.Info.Chat, evt.Info.Sender)
	}

	// Send typing indicator
	if client != nil && client.IsConnected() {
		_ = client.SendChatPresence(ctx, evt.Info.Chat, types.ChatPresenceComposing, types.ChatPresenceMediaText)
	}

	// Set up streaming callback — send new messages progressively (no editing)
	st := util.NewStreamTracker(3 * time.Second)

	jid := evt.Info.Chat
	msg.StreamCallback = func(text string) {
		if client == nil || !client.IsConnected() {
			return
		}

		newPortion, ok := st.ShouldSend(text)
		if !ok {
			return
		}

		if _, err := client.SendMessage(ctx, jid, &waE2E.Message{
			Conversation: proto.String(platform.SanitizeText("whatsapp", newPortion)),
		}); err != nil {
			b.logger.Debug("stream: send failed", "error", err)
			return
		}
		st.MarkSent(len(text))
	}

	response, err := handler(ctx, msg)
	if err != nil {
		b.logger.Warn("handler error", "error", err)
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
	finalText, shouldSend := st.FinalText(response)
	if !shouldSend {
		return
	}
	finalText = platform.SanitizeText("whatsapp", finalText)

	if st.Streamed() {
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
		if err := b.Send(chatID, finalText); err != nil {
			b.logger.Error("send reply failed", "error", err)
		}
	}
}

// saveVoice writes downloaded audio bytes to the downloads directory.
func (b *Bot) saveVoice(data []byte) (string, error) {
	if err := os.MkdirAll(b.downloadsDir, 0700); err != nil {
		return "", fmt.Errorf("create downloads dir: %w", err)
	}
	filename := fmt.Sprintf("voice_%d.ogg", time.Now().UnixNano())
	path := filepath.Join(b.downloadsDir, filename)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return "", fmt.Errorf("write voice: %w", err)
	}
	return path, nil
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

	client := b.getClient()
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

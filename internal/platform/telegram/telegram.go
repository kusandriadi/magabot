// Package telegram provides Telegram bot integration
package telegram

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/kusa/magabot/internal/platform"
	"github.com/kusa/magabot/internal/router"
	"github.com/kusa/magabot/internal/security"
	"github.com/kusa/magabot/internal/util"
)

// Bot represents a Telegram bot
type Bot struct {
	platform.Base
	api          *gotgbot.Bot
	logger       *slog.Logger
	downloadsDir string
	username     string
	done         chan struct{}
	wg           sync.WaitGroup
}

// Config for Telegram bot
type Config struct {
	Token        string
	DownloadsDir string // Directory to save downloaded media
	Logger       *slog.Logger
}

// New creates a new Telegram bot
func New(cfg *Config) (*Bot, error) {
	api, err := gotgbot.NewBot(cfg.Token, &gotgbot.BotOpts{
		BotClient: &gotgbot.BaseBotClient{
			Client:             http.Client{},
			DefaultRequestOpts: &gotgbot.RequestOpts{Timeout: 70 * time.Second},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create bot: %w", err)
	}

	downloadsDir := cfg.DownloadsDir
	if downloadsDir != "" {
		if err := os.MkdirAll(downloadsDir, 0700); err != nil {
			return nil, fmt.Errorf("failed to create downloads directory: %w", err)
		}
	}

	return &Bot{
		api:          api,
		logger:       cfg.Logger,
		downloadsDir: downloadsDir,
		done:         make(chan struct{}),
	}, nil
}

// Name returns the platform name
func (b *Bot) Name() string {
	return "telegram"
}

// Start starts listening for updates (long polling)
func (b *Bot) Start(ctx context.Context) error {
	info, err := b.api.GetMe(nil)
	if err != nil {
		return fmt.Errorf("get bot info: %w", err)
	}
	b.username = info.Username

	b.wg.Add(1)
	go b.pollUpdates(ctx)

	b.logger.Info("telegram bot started", "username", b.username)
	return nil
}

// Stop stops the bot
func (b *Bot) Stop() error {
	close(b.done)
	b.wg.Wait()
	return nil
}

// Send sends a message. chatID may be "groupID:threadID" for Forum Topics.
func (b *Bot) Send(chatID, message string) error {
	groupID, threadID := parseChatID(chatID)
	if groupID == 0 {
		return fmt.Errorf("invalid chat ID: %s", chatID)
	}

	opts := &gotgbot.SendMessageOpts{ParseMode: "Markdown"}
	if threadID != 0 {
		opts.MessageThreadId = threadID
	}

	_, err := b.api.SendMessage(groupID, message, opts)
	return err
}

// SendVoice sends an OGG Opus audio as a Telegram voice message.
func (b *Bot) SendVoice(chatID string, audio []byte) error {
	groupID, threadID := parseChatID(chatID)
	if groupID == 0 {
		return fmt.Errorf("invalid chat ID: %s", chatID)
	}
	opts := &gotgbot.SendVoiceOpts{}
	if threadID != 0 {
		opts.MessageThreadId = threadID
	}
	_, err := b.api.SendVoice(groupID, gotgbot.InputFileByReader("voice.ogg", bytes.NewReader(audio)), opts)
	return err
}

// SetHandler is provided by platform.Base.

// pollUpdates runs the long-polling loop
func (b *Bot) pollUpdates(ctx context.Context) {
	defer b.wg.Done()

	var offset int64
	for {
		select {
		case <-b.done:
			return
		case <-ctx.Done():
			return
		default:
		}

		updates, err := b.api.GetUpdatesWithContext(ctx, &gotgbot.GetUpdatesOpts{
			Offset:  offset,
			Timeout: 60,
		})
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			select {
			case <-b.done:
				return
			default:
			}
			b.logger.Warn("get updates failed", "error", err)
			time.Sleep(time.Second)
			continue
		}

		for i := range updates {
			offset = updates[i].UpdateId + 1
			if updates[i].Message != nil {
				go b.handleUpdate(ctx, updates[i].Message)
			}
		}
	}
}

// handleUpdate handles a single incoming message
func (b *Bot) handleUpdate(ctx context.Context, msg *gotgbot.Message) {
	text := msg.Text
	var media []string

	// Handle photo messages
	if len(msg.Photo) > 0 {
		// Get the largest photo (last in array)
		photo := msg.Photo[len(msg.Photo)-1]
		if path, err := b.downloadFile(photo.FileId, "photo"); err == nil {
			media = append(media, path)
		} else {
			b.logger.Warn("download photo failed", "error", err)
		}
		if msg.Caption != "" {
			text = msg.Caption
		} else if text == "" {
			text = "[Photo]"
		}
	}

	// Handle voice messages
	if msg.Voice != nil {
		if path, err := b.downloadFile(msg.Voice.FileId, "voice"); err == nil {
			media = append(media, path)
		} else {
			b.logger.Warn("download voice failed", "error", err)
		}
		if msg.Caption != "" {
			text = msg.Caption
		} else if text == "" {
			text = "[Voice Message]"
		}
	}

	// Handle audio messages
	if msg.Audio != nil {
		if text == "" {
			text = "[Audio Message]"
		}
	}

	// Handle document messages
	if msg.Document != nil {
		if path, err := b.downloadFile(msg.Document.FileId, "doc"); err == nil {
			media = append(media, path)
		} else {
			b.logger.Warn("download document failed", "error", err)
		}
		if msg.Caption != "" {
			text = msg.Caption
		} else if text == "" {
			text = fmt.Sprintf("[Document: %s]", msg.Document.FileName)
		}
	}

	// Skip if no text content at all
	if text == "" {
		return
	}

	// For Forum Topics (supergroups with topics enabled), include thread ID in
	// ChatID so each topic gets its own isolated session.
	chatID := fmt.Sprintf("%d", msg.Chat.Id)
	threadID := msg.MessageThreadId
	if msg.IsTopicMessage && threadID != 0 {
		chatID = fmt.Sprintf("%d:%d", msg.Chat.Id, threadID)
	}

	routerMsg := &router.Message{
		Platform:  "telegram",
		ChatID:    chatID,
		UserID:    fmt.Sprintf("%d", msg.From.Id),
		Username:  msg.From.Username,
		Text:      text,
		Media:     media,
		Timestamp: time.Unix(msg.Date, 0),
		Raw:       msg,
	}

	// Extract reply context if this message is a reply.
	// Prefer quoted text (user-selected portion) over full message text.
	if msg.ReplyToMessage != nil {
		replyText := msg.ReplyToMessage.Text
		if replyText == "" {
			replyText = msg.ReplyToMessage.Caption
		}
		// In newer Telegram clients, the user may select a specific portion to quote.
		if msg.Quote != nil && msg.Quote.Text != "" {
			replyText = msg.Quote.Text
		}
		b.logger.Debug("reply detected",
			"chat_id", chatID,
			"reply_msg_id", msg.ReplyToMessage.MessageId,
			"reply_text_len", len(replyText),
			"has_quote", msg.Quote != nil,
			"reply_from_nil", msg.ReplyToMessage.From == nil,
		)
		if replyText != "" {
			var replyUser string
			var isBot bool
			if msg.ReplyToMessage.From != nil {
				replyUser = msg.ReplyToMessage.From.Username
				if replyUser == "" {
					replyUser = msg.ReplyToMessage.From.FirstName
				}
				isBot = msg.ReplyToMessage.From.IsBot
			}
			routerMsg.ReplyTo = &router.ReplyContext{
				Text:     replyText,
				Username: replyUser,
				IsBot:    isBot,
			}
		}
	} else if msg.ExternalReply != nil {
		// Reply to a message from a linked channel or different chat.
		// Full text is unavailable; use the quoted portion if the user selected one.
		b.logger.Debug("external reply detected", "chat_id", chatID, "has_quote", msg.Quote != nil)
		replyText := ""
		if msg.Quote != nil {
			replyText = msg.Quote.Text
		}
		origin := msg.ExternalReply.Origin.MergeMessageOrigin()
		replyUser := origin.SenderUserName
		var isBot bool
		if origin.SenderUser != nil {
			replyUser = origin.SenderUser.Username
			if replyUser == "" {
				replyUser = origin.SenderUser.FirstName
			}
			isBot = origin.SenderUser.IsBot
		} else if origin.Chat != nil {
			replyUser = origin.Chat.Title
		} else if origin.SenderChat != nil {
			replyUser = origin.SenderChat.Title
		}
		if replyText != "" || replyUser != "" {
			routerMsg.ReplyTo = &router.ReplyContext{
				Text:     replyText,
				Username: replyUser,
				IsBot:    isBot,
			}
		}
	}

	handler := b.GetHandler()
	if handler == nil {
		return
	}

	// Set up streaming callback — send new messages progressively (no editing)
	st := util.NewStreamTracker(2 * time.Second)

	routerMsg.StreamCallback = func(text string) {
		newPortion, ok := st.ShouldSend(text)
		if !ok {
			return
		}

		for i, chunk := range platform.SplitMessage(platform.SanitizeText("telegram", newPortion), telegramMaxLen) {
			opts := &gotgbot.SendMessageOpts{}
			if st.IsFirstChunk() && i == 0 {
				opts.ReplyParameters = &gotgbot.ReplyParameters{MessageId: msg.MessageId}
			}
			if threadID != 0 {
				opts.MessageThreadId = threadID
			}
			if _, err := b.api.SendMessage(msg.Chat.Id, chunk, opts); err != nil {
				b.logger.Debug("stream: send failed", "error", err)
				return
			}
		}
		// Re-send typing since SendMessage clears it
		typingOpts := &gotgbot.SendChatActionOpts{}
		if threadID != 0 {
			typingOpts.MessageThreadId = threadID
		}
		_, _ = b.api.SendChatAction(msg.Chat.Id, "typing", typingOpts)
		st.MarkSent(len(text))
	}

	// Start periodic typing indicator (Telegram typing expires after ~5s)
	typingDone := make(chan struct{})
	go func() {
		opts := &gotgbot.SendChatActionOpts{}
		if threadID != 0 {
			opts.MessageThreadId = threadID
		}
		if _, err := b.api.SendChatAction(msg.Chat.Id, "typing", opts); err != nil {
			b.logger.Debug("send typing failed", "error", err)
		}
		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-typingDone:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := b.api.SendChatAction(msg.Chat.Id, "typing", opts); err != nil {
					b.logger.Debug("send typing failed", "error", err)
				}
			}
		}
	}()

	response, err := handler(ctx, routerMsg)
	close(typingDone)
	if err != nil {
		b.logger.Warn("handler error", "error", err)
		// Notify user for non-auth errors so the bot doesn't go silent
		if err != security.ErrNotAuthorized && err != security.ErrAccountLocked {
			errOpts := &gotgbot.SendMessageOpts{ReplyParameters: &gotgbot.ReplyParameters{MessageId: msg.MessageId}}
			if threadID != 0 {
				errOpts.MessageThreadId = threadID
			}
			if _, sendErr := b.api.SendMessage(msg.Chat.Id, "⚠️ "+err.Error(), errOpts); sendErr != nil {
				b.logger.Debug("send error msg failed", "error", sendErr)
			}
		}
		return
	}

	if response == "" {
		return
	}

	// Send the remaining text not yet delivered during streaming
	finalText, shouldSend := st.FinalText(response)
	if !shouldSend {
		return
	}
	finalText = platform.SanitizeText("telegram", finalText)

	opts := &gotgbot.SendMessageOpts{ParseMode: "Markdown"}
	if !st.Streamed() {
		opts.ReplyParameters = &gotgbot.ReplyParameters{MessageId: msg.MessageId}
	}
	if threadID != 0 {
		opts.MessageThreadId = threadID
	}
	for _, chunk := range platform.SplitMessage(finalText, telegramMaxLen) {
		chunkOpts := *opts
		if _, err := b.api.SendMessage(msg.Chat.Id, chunk, &chunkOpts); err != nil {
			// Markdown parse may fail on LLM output — retry without parse mode
			chunkOpts.ParseMode = ""
			if _, err2 := b.api.SendMessage(msg.Chat.Id, chunk, &chunkOpts); err2 != nil {
				b.logger.Error("send failed (even without parse mode)", "original_error", err, "retry_error", err2)
				break
			}
		}
	}
}

// telegramMaxLen is Telegram's maximum message length in characters.
const telegramMaxLen = 4096

// parseChatID parses "groupID" or "groupID:threadID" format.
func parseChatID(chatID string) (groupID int64, threadID int64) {
	fmt.Sscanf(chatID, "%d:%d", &groupID, &threadID) //nolint:errcheck
	if groupID == 0 {
		fmt.Sscanf(chatID, "%d", &groupID) //nolint:errcheck
	}
	return
}

// maxDownloadSize is the maximum file size for Telegram downloads (20MB matches Telegram's limit)
const maxDownloadSize = 20 * 1024 * 1024

// allowedMediaExts is the whitelist of allowed media file extensions
var allowedMediaExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".bmp": true,
	".mp4": true, ".webm": true, ".mov": true,
	".ogg": true, ".oga": true, ".mp3": true, ".m4a": true, ".wav": true,
	".pdf": true, ".txt": true, ".md": true, ".csv": true, ".json": true,
	".docx": true, ".xlsx": true, ".pptx": true,
}

// downloadFile downloads a file from Telegram and saves it locally
func (b *Bot) downloadFile(fileID, prefix string) (string, error) {
	if b.downloadsDir == "" {
		return "", fmt.Errorf("downloads dir not configured")
	}

	file, err := b.api.GetFile(fileID, nil)
	if err != nil {
		return "", fmt.Errorf("get file: %w", err)
	}

	// Build download URL (token is embedded — use dedicated client, never log this URL)
	fileURL := file.URL(b.api, nil)

	client := util.NewHTTPClient(60 * time.Second)
	resp, err := client.Get(fileURL)
	if err != nil {
		return "", fmt.Errorf("download file: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	// Validate and sanitize file extension
	ext := filepath.Ext(file.FilePath)
	if ext == "" {
		ext = ".jpg"
	}
	if !allowedMediaExts[ext] {
		ext = ".bin"
	}

	filename := fmt.Sprintf("%s_%d%s", prefix, time.Now().UnixNano(), ext)
	localPath := filepath.Join(b.downloadsDir, filename)

	// Create file with restricted permissions (0600 = owner read/write only)
	out, err := os.OpenFile(localPath, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	defer func() { _ = out.Close() }()

	// Limit download size to prevent disk exhaustion
	limited := io.LimitReader(resp.Body, maxDownloadSize)
	if _, err := io.Copy(out, limited); err != nil {
		_ = os.Remove(localPath)
		return "", fmt.Errorf("save file: %w", err)
	}

	return localPath, nil
}

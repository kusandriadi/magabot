// Package telegram provides Telegram bot integration
package telegram

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/kusa/magabot/internal/router"
)

// Bot represents a Telegram bot
type Bot struct {
	api          *tgbotapi.BotAPI
	handler      router.MessageHandler
	handlerMu    sync.RWMutex
	logger       *slog.Logger
	downloadsDir string
	updates      tgbotapi.UpdatesChannel
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
	api, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("create bot: %w", err)
	}

	// Security: disable debug mode
	api.Debug = false

	downloadsDir := cfg.DownloadsDir
	if downloadsDir != "" {
		os.MkdirAll(downloadsDir, 0700)
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
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	b.updates = b.api.GetUpdatesChan(u)

	b.wg.Add(1)
	go b.processUpdates(ctx)

	b.logger.Info("telegram bot started", "username", b.api.Self.UserName)
	return nil
}

// Stop stops the bot
func (b *Bot) Stop() error {
	close(b.done)
	b.api.StopReceivingUpdates()
	b.wg.Wait()
	return nil
}

// Send sends a message
func (b *Bot) Send(chatID, message string) error {
	var chatIDInt int64
	if _, err := fmt.Sscanf(chatID, "%d", &chatIDInt); err != nil {
		return fmt.Errorf("invalid chat ID: %s", chatID)
	}

	msg := tgbotapi.NewMessage(chatIDInt, message)
	msg.ParseMode = "Markdown"
	
	_, err := b.api.Send(msg)
	return err
}

// SetHandler sets the message handler
func (b *Bot) SetHandler(h router.MessageHandler) {
	b.handlerMu.Lock()
	b.handler = h
	b.handlerMu.Unlock()
}

// processUpdates processes incoming updates
func (b *Bot) processUpdates(ctx context.Context) {
	defer b.wg.Done()

	for {
		select {
		case <-b.done:
			return
		case <-ctx.Done():
			return
		case update := <-b.updates:
			if update.Message == nil {
				continue
			}
			b.handleUpdate(ctx, &update)
		}
	}
}

// handleUpdate handles a single update
func (b *Bot) handleUpdate(ctx context.Context, update *tgbotapi.Update) {
	msg := update.Message
	if msg == nil {
		return
	}

	text := msg.Text
	var media []string

	// Handle photo messages
	if msg.Photo != nil && len(msg.Photo) > 0 {
		// Get the largest photo (last in array)
		photo := msg.Photo[len(msg.Photo)-1]
		if path, err := b.downloadFile(photo.FileID, "photo"); err == nil {
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
		if text == "" {
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

	routerMsg := &router.Message{
		Platform:  "telegram",
		ChatID:    fmt.Sprintf("%d", msg.Chat.ID),
		UserID:    fmt.Sprintf("%d", msg.From.ID),
		Username:  msg.From.UserName,
		Text:      text,
		Media:     media,
		Timestamp: time.Unix(int64(msg.Date), 0),
		Raw:       update,
	}

	b.handlerMu.RLock()
	handler := b.handler
	b.handlerMu.RUnlock()

	if handler == nil {
		return
	}

	response, err := handler(ctx, routerMsg)
	if err != nil {
		b.logger.Warn("handler error", "error", err)
		return
	}

	if response == "" {
		return
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, response)
	reply.ParseMode = "Markdown"
	reply.ReplyToMessageID = msg.MessageID

	if _, err := b.api.Send(reply); err != nil {
		b.logger.Error("send failed", "error", err)
	}
}

// maxDownloadSize is the maximum file size for Telegram downloads (20MB matches Telegram's limit)
const maxDownloadSize = 20 * 1024 * 1024

// allowedMediaExts is the whitelist of allowed media file extensions
var allowedMediaExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".bmp": true,
	".mp4": true, ".webm": true, ".mov": true,
	".ogg": true, ".oga": true, ".mp3": true, ".m4a": true, ".wav": true,
	".pdf": true, ".txt": true,
}

// downloadFile downloads a file from Telegram and saves it locally
func (b *Bot) downloadFile(fileID, prefix string) (string, error) {
	if b.downloadsDir == "" {
		return "", fmt.Errorf("downloads dir not configured")
	}

	file, err := b.api.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		return "", fmt.Errorf("get file: %w", err)
	}

	// Build download URL (token is embedded by Telegram API — use dedicated client, never log this URL)
	fileURL := file.Link(b.api.Token)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(fileURL)
	if err != nil {
		return "", fmt.Errorf("download file: %w", err)
	}
	defer resp.Body.Close()

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
	defer out.Close()

	// Limit download size to prevent disk exhaustion
	limited := io.LimitReader(resp.Body, maxDownloadSize)
	if _, err := io.Copy(out, limited); err != nil {
		os.Remove(localPath)
		return "", fmt.Errorf("save file: %w", err)
	}

	return localPath, nil
}

// GetBotInfo returns bot information
func (b *Bot) GetBotInfo() *tgbotapi.User {
	return &b.api.Self
}

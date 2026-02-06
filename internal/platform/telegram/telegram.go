// Package telegram provides Telegram bot integration
package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/kusa/magabot/internal/router"
)

// Bot represents a Telegram bot
type Bot struct {
	api     *tgbotapi.BotAPI
	handler router.MessageHandler
	logger  *slog.Logger
	updates tgbotapi.UpdatesChannel
	done    chan struct{}
	wg      sync.WaitGroup
}

// Config for Telegram bot
type Config struct {
	Token  string
	Logger *slog.Logger
}

// New creates a new Telegram bot
func New(cfg *Config) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		return nil, fmt.Errorf("create bot: %w", err)
	}

	// Security: disable debug mode
	api.Debug = false

	return &Bot{
		api:    api,
		logger: cfg.Logger,
		done:   make(chan struct{}),
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
	b.handler = h
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

	routerMsg := &router.Message{
		Platform:  "telegram",
		ChatID:    fmt.Sprintf("%d", msg.Chat.ID),
		UserID:    fmt.Sprintf("%d", msg.From.ID),
		Username:  msg.From.UserName,
		Text:      msg.Text,
		Timestamp: time.Unix(int64(msg.Date), 0),
		Raw:       update,
	}

	if b.handler == nil {
		return
	}

	response, err := b.handler(ctx, routerMsg)
	if err != nil {
		b.logger.Debug("handler error", "error", err)
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

// GetBotInfo returns bot information
func (b *Bot) GetBotInfo() *tgbotapi.User {
	return &b.api.Self
}

// Package slack provides Slack bot integration using Socket Mode
package slack

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
	"github.com/kusa/magabot/internal/router"
)

// Bot represents a Slack bot
type Bot struct {
	api       *slack.Client
	socket    *socketmode.Client
	handler   router.MessageHandler
	handlerMu sync.RWMutex
	logger    *slog.Logger
	done      chan struct{}
	wg        sync.WaitGroup
}

// Config for Slack bot
type Config struct {
	BotToken string
	AppToken string
	Logger   *slog.Logger
}

// New creates a new Slack bot
func New(cfg *Config) (*Bot, error) {
	api := slack.New(
		cfg.BotToken,
		slack.OptionAppLevelToken(cfg.AppToken),
	)

	socket := socketmode.New(api)

	return &Bot{
		api:    api,
		socket: socket,
		logger: cfg.Logger,
		done:   make(chan struct{}),
	}, nil
}

// Name returns the platform name
func (b *Bot) Name() string {
	return "slack"
}

// Start starts the socket mode client
func (b *Bot) Start(ctx context.Context) error {
	b.wg.Add(1)
	go b.processEvents(ctx)

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		if err := b.socket.Run(); err != nil {
			b.logger.Error("socket mode error", "error", err)
		}
	}()

	b.logger.Info("slack bot started")
	return nil
}

// Stop stops the bot
func (b *Bot) Stop() error {
	close(b.done)
	b.wg.Wait()
	return nil
}

// Send sends a message
func (b *Bot) Send(chatID, message string) error {
	_, _, err := b.api.PostMessage(chatID, slack.MsgOptionText(message, false))
	return err
}

// SetHandler sets the message handler
func (b *Bot) SetHandler(h router.MessageHandler) {
	b.handlerMu.Lock()
	b.handler = h
	b.handlerMu.Unlock()
}

// processEvents processes socket mode events
func (b *Bot) processEvents(ctx context.Context) {
	defer b.wg.Done()

	for {
		select {
		case <-b.done:
			return
		case <-ctx.Done():
			return
		case evt := <-b.socket.Events:
			b.handleEvent(ctx, evt)
		}
	}
}

// handleEvent handles a socket mode event
func (b *Bot) handleEvent(ctx context.Context, evt socketmode.Event) {
	switch evt.Type {
	case socketmode.EventTypeEventsAPI:
		eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
		if !ok {
			return
		}

		b.socket.Ack(*evt.Request)

		switch eventsAPIEvent.Type {
		case slackevents.CallbackEvent:
			innerEvent := eventsAPIEvent.InnerEvent
			switch ev := innerEvent.Data.(type) {
			case *slackevents.MessageEvent:
				if ev.BotID != "" {
					return
				}
				b.handleMessage(ctx, ev)
			}
		}

	case socketmode.EventTypeSlashCommand:
		cmd, ok := evt.Data.(slack.SlashCommand)
		if !ok {
			return
		}
		b.socket.Ack(*evt.Request)
		b.handleSlashCommand(ctx, &cmd)
	}
}

// handleMessage handles an incoming message
func (b *Bot) handleMessage(ctx context.Context, ev *slackevents.MessageEvent) {
	b.handlerMu.RLock()
	handler := b.handler
	b.handlerMu.RUnlock()

	if handler == nil {
		return
	}

	msg := &router.Message{
		Platform:  "slack",
		ChatID:    ev.Channel,
		UserID:    ev.User,
		Text:      ev.Text,
		Timestamp: parseSlackTimestamp(ev.TimeStamp),
		Raw:       ev,
	}

	user, err := b.api.GetUserInfo(ev.User)
	if err == nil {
		msg.Username = user.Name
	}

	response, err := handler(ctx, msg)
	if err != nil {
		b.logger.Debug("handler error", "error", err)
		return
	}

	if response == "" {
		return
	}

	if _, _, err := b.api.PostMessage(ev.Channel,
		slack.MsgOptionText(response, false),
		slack.MsgOptionTS(ev.TimeStamp),
	); err != nil {
		b.logger.Error("send message failed", "channel", ev.Channel, "error", err)
	}
}

// handleSlashCommand handles a slash command
func (b *Bot) handleSlashCommand(ctx context.Context, cmd *slack.SlashCommand) {
	b.handlerMu.RLock()
	handler := b.handler
	b.handlerMu.RUnlock()

	if handler == nil {
		return
	}

	msg := &router.Message{
		Platform:  "slack",
		ChatID:    cmd.ChannelID,
		UserID:    cmd.UserID,
		Username:  cmd.UserName,
		Text:      fmt.Sprintf("/%s %s", cmd.Command, cmd.Text),
		Timestamp: time.Now(),
		Raw:       cmd,
	}

	response, err := handler(ctx, msg)
	if err != nil {
		b.logger.Debug("handler error", "error", err)
		return
	}

	if response != "" {
		if _, _, err := b.api.PostMessage(cmd.ChannelID, slack.MsgOptionText(response, false)); err != nil {
			b.logger.Error("send slash response failed", "channel", cmd.ChannelID, "error", err)
		}
	}
}

func parseSlackTimestamp(ts string) time.Time {
	var sec, nsec int64
	_, _ = fmt.Sscanf(ts, "%d.%d", &sec, &nsec)
	return time.Unix(sec, nsec*1000)
}

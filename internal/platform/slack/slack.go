// Package slack provides Slack bot integration using Socket Mode
package slack

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/kusa/magabot/internal/platform"
	"github.com/kusa/magabot/internal/router"
	"github.com/kusa/magabot/internal/util"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

// Bot represents a Slack bot
type Bot struct {
	platform.Base
	api    *slack.Client
	socket *socketmode.Client
	logger *slog.Logger
	done   chan struct{}
	wg     sync.WaitGroup
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

// SendVoice is not supported on Slack; it's a no-op.
func (b *Bot) SendVoice(_ string, _ []byte) error { return nil }

// SetHandler is provided by platform.Base.

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

// slackMaxLen is the max message length for splitting long responses.
const slackMaxLen = 4096

func (b *Bot) handleMessage(ctx context.Context, ev *slackevents.MessageEvent) {
	handler := b.GetHandler()
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

	// Extract reply context if this message is in a thread
	if ev.ThreadTimeStamp != "" && ev.ThreadTimeStamp != ev.TimeStamp {
		msgs, _, _, replyErr := b.api.GetConversationReplies(&slack.GetConversationRepliesParameters{
			ChannelID: ev.Channel,
			Timestamp: ev.ThreadTimeStamp,
			Limit:     1,
			Inclusive: true,
		})
		if replyErr == nil && len(msgs) > 0 {
			parent := msgs[0]
			var parentUser string
			var isBot bool
			if parent.BotID != "" {
				parentUser = "bot"
				isBot = true
			} else if parent.User != "" {
				if u, uErr := b.api.GetUserInfo(parent.User); uErr == nil {
					parentUser = u.Name
				}
			}
			msg.ReplyTo = &router.ReplyContext{
				Text:     parent.Text,
				Username: parentUser,
				IsBot:    isBot,
			}
		}
	}

	// Add typing indicator reaction (Slack doesn't support bot typing events)
	typingRef := slack.ItemRef{
		Channel:   ev.Channel,
		Timestamp: ev.TimeStamp,
	}
	_ = b.api.AddReaction("hourglass_flowing_sand", typingRef)
	defer func() {
		_ = b.api.RemoveReaction("hourglass_flowing_sand", typingRef)
	}()

	// Set up streaming callback — send new messages progressively (no editing)
	st := util.NewStreamTracker(2 * time.Second)

	msg.StreamCallback = func(text string) {
		newPortion, ok := st.ShouldSend(text)
		if !ok {
			return
		}

		for _, chunk := range platform.SplitMessage(platform.SanitizeText("slack", newPortion), slackMaxLen) {
			if _, _, err := b.api.PostMessage(ev.Channel,
				slack.MsgOptionText(chunk, false),
				slack.MsgOptionTS(ev.TimeStamp),
			); err != nil {
				b.logger.Debug("stream: send failed", "error", err)
				return
			}
		}
		st.MarkSent(len(text))
	}

	response, err := handler(ctx, msg)
	if err != nil {
		b.logger.Debug("handler error", "error", err)
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
	finalText = platform.SanitizeText("slack", finalText)

	for _, chunk := range platform.SplitMessage(finalText, slackMaxLen) {
		if _, _, err := b.api.PostMessage(ev.Channel,
			slack.MsgOptionText(chunk, false),
			slack.MsgOptionTS(ev.TimeStamp),
		); err != nil {
			b.logger.Error("send chunk failed", "channel", ev.Channel, "error", err)
		}
	}
}

// handleSlashCommand handles a slash command
func (b *Bot) handleSlashCommand(ctx context.Context, cmd *slack.SlashCommand) {
	handler := b.GetHandler()
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
		if _, _, err := b.api.PostMessage(cmd.ChannelID, slack.MsgOptionText(platform.SanitizeText("slack", response), false)); err != nil {
			b.logger.Error("send slash response failed", "channel", cmd.ChannelID, "error", err)
		}
	}
}

func parseSlackTimestamp(ts string) time.Time {
	var sec, nsec int64
	_, _ = fmt.Sscanf(ts, "%d.%d", &sec, &nsec)
	return time.Unix(sec, nsec*1000)
}

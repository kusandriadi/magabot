// Package platform provides chat platform adapters
package platform

import (
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
)

// DiscordAdapter implements Discord bot using discordgo
type DiscordAdapter struct {
	token       string
	session     *discordgo.Session
	handler     MessageHandler
	allowedIDs  map[string]bool // user IDs or guild IDs
	mu          sync.RWMutex
	running     bool
	botID       string
	prefix      string // command prefix, default "!"
}

// DiscordConfig holds Discord configuration
type DiscordConfig struct {
	Token      string   `yaml:"token" json:"token"`
	AllowedIDs []string `yaml:"allowed_ids" json:"allowed_ids"` // user or guild IDs
	Prefix     string   `yaml:"prefix" json:"prefix"`           // command prefix
}

// NewDiscordAdapter creates a new Discord adapter
func NewDiscordAdapter(config DiscordConfig) (*DiscordAdapter, error) {
	if config.Token == "" {
		return nil, fmt.Errorf("discord token is required")
	}

	prefix := config.Prefix
	if prefix == "" {
		prefix = "!" // default prefix
	}

	allowed := make(map[string]bool)
	for _, id := range config.AllowedIDs {
		allowed[id] = true
	}

	return &DiscordAdapter{
		token:      config.Token,
		allowedIDs: allowed,
		prefix:     prefix,
	}, nil
}

// Start begins listening for Discord messages
func (d *DiscordAdapter) Start(handler MessageHandler) error {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return fmt.Errorf("discord adapter already running")
	}
	d.handler = handler
	d.mu.Unlock()

	// Create Discord session
	session, err := discordgo.New("Bot " + d.token)
	if err != nil {
		return fmt.Errorf("failed to create discord session: %w", err)
	}

	d.session = session

	// Set intents
	session.Identify.Intents = discordgo.IntentsGuildMessages |
		discordgo.IntentsDirectMessages |
		discordgo.IntentsMessageContent

	// Register message handler
	session.AddHandler(d.onMessage)
	session.AddHandler(d.onReady)

	// Open connection
	if err := session.Open(); err != nil {
		return fmt.Errorf("failed to open discord connection: %w", err)
	}

	d.mu.Lock()
	d.running = true
	d.mu.Unlock()

	log.Println("[DISCORD] Bot started")
	return nil
}

// Stop closes the Discord connection
func (d *DiscordAdapter) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.running {
		return nil
	}

	if d.session != nil {
		d.session.Close()
	}

	d.running = false
	log.Println("[DISCORD] Bot stopped")
	return nil
}

// Send sends a message to a Discord channel
func (d *DiscordAdapter) Send(channelID, message string) error {
	if d.session == nil {
		return fmt.Errorf("discord session not initialized")
	}

	_, err := d.session.ChannelMessageSend(channelID, message)
	return err
}

// SendEmbed sends an embed message
func (d *DiscordAdapter) SendEmbed(channelID string, embed *discordgo.MessageEmbed) error {
	if d.session == nil {
		return fmt.Errorf("discord session not initialized")
	}

	_, err := d.session.ChannelMessageSendEmbed(channelID, embed)
	return err
}

// Reply replies to a specific message
func (d *DiscordAdapter) Reply(channelID, messageID, message string) error {
	if d.session == nil {
		return fmt.Errorf("discord session not initialized")
	}

	_, err := d.session.ChannelMessageSendReply(channelID, message, &discordgo.MessageReference{
		MessageID: messageID,
		ChannelID: channelID,
	})
	return err
}

// onReady handles the ready event
func (d *DiscordAdapter) onReady(s *discordgo.Session, r *discordgo.Ready) {
	d.mu.Lock()
	d.botID = r.User.ID
	d.mu.Unlock()

	log.Printf("[DISCORD] Logged in as %s#%s", r.User.Username, r.User.Discriminator)

	// Set status
	s.UpdateGameStatus(0, "Ready to help!")
}

// onMessage handles incoming messages
func (d *DiscordAdapter) onMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore own messages
	if m.Author.ID == d.botID {
		return
	}

	// Ignore bots
	if m.Author.Bot {
		return
	}

	// Check authorization
	if !d.isAuthorized(m) {
		return
	}

	// Check if message is for us (mention or prefix or DM)
	content := m.Content
	isDM := m.GuildID == ""
	isMention := d.isMentioned(m)
	hasPrefix := strings.HasPrefix(content, d.prefix)

	if !isDM && !isMention && !hasPrefix {
		return
	}

	// Clean up message
	if hasPrefix {
		content = strings.TrimPrefix(content, d.prefix)
	}
	if isMention {
		// Remove mention from content
		content = d.removeMention(content)
	}
	content = strings.TrimSpace(content)

	if content == "" {
		return
	}

	// Create message for handler
	msg := &Message{
		Platform:  "discord",
		ChatID:    m.ChannelID,
		MessageID: m.ID,
		UserID:    m.Author.ID,
		Username:  m.Author.Username,
		Text:      content,
		IsGroup:   !isDM,
		GuildID:   m.GuildID,
		Raw:       m,
	}

	// Handle message
	if d.handler != nil {
		go d.handleMessage(msg)
	}
}

// handleMessage processes message and sends response
func (d *DiscordAdapter) handleMessage(msg *Message) {
	response, err := d.handler(msg)
	if err != nil {
		log.Printf("[DISCORD] Handler error: %v", err)
		d.Send(msg.ChatID, fmt.Sprintf("❌ Error: %v", err))
		return
	}

	if response != "" {
		d.Send(msg.ChatID, response)
	}
}

// isAuthorized checks if message sender is allowed
func (d *DiscordAdapter) isAuthorized(m *discordgo.MessageCreate) bool {
	// If no restrictions, allow all
	if len(d.allowedIDs) == 0 {
		return true
	}

	// Check user ID
	if d.allowedIDs[m.Author.ID] {
		return true
	}

	// Check guild ID
	if m.GuildID != "" && d.allowedIDs[m.GuildID] {
		return true
	}

	return false
}

// isMentioned checks if bot was mentioned
func (d *DiscordAdapter) isMentioned(m *discordgo.MessageCreate) bool {
	for _, mention := range m.Mentions {
		if mention.ID == d.botID {
			return true
		}
	}
	return false
}

// removeMention removes bot mention from message
func (d *DiscordAdapter) removeMention(content string) string {
	// Remove <@BOT_ID> or <@!BOT_ID>
	content = strings.ReplaceAll(content, "<@"+d.botID+">", "")
	content = strings.ReplaceAll(content, "<@!"+d.botID+">", "")
	return strings.TrimSpace(content)
}

// Name returns the platform name
func (d *DiscordAdapter) Name() string {
	return "discord"
}

// IsRunning returns whether the adapter is running
func (d *DiscordAdapter) IsRunning() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.running
}

// GetSession returns the Discord session for advanced operations
func (d *DiscordAdapter) GetSession() *discordgo.Session {
	return d.session
}

package cron

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// NotifierConfig holds notification channel credentials
type NotifierConfig struct {
	TelegramToken   string `json:"telegram_token,omitempty"`
	SlackToken      string `json:"slack_token,omitempty"`
	DiscordToken    string `json:"discord_token,omitempty"`    // Bot token for Discord
	DiscordWebhook  string `json:"discord_webhook,omitempty"`  // Webhook URL fallback
	WhatsAppAPIURL  string `json:"whatsapp_api_url,omitempty"`
	WhatsAppAPIKey  string `json:"whatsapp_api_key,omitempty"`
	LarkAppID       string `json:"lark_app_id,omitempty"`
	LarkAppSecret   string `json:"lark_app_secret,omitempty"`
	larkToken       string // cached access token
	larkTokenExpiry int64  // unix timestamp
}

// Notifier sends messages to various channels
type Notifier struct {
	config     NotifierConfig
	httpClient *http.Client
}

// NewNotifier creates a new notifier
func NewNotifier(config NotifierConfig) *Notifier {
	return &Notifier{
		config: config,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Send dispatches a message to the specified channel
func (n *Notifier) Send(ctx context.Context, ch NotifyChannel, message string) error {
	switch strings.ToLower(ch.Type) {
	case "telegram", "tg":
		return n.sendTelegram(ctx, ch.Target, message)
	case "whatsapp", "wa":
		return n.sendWhatsApp(ctx, ch.Target, message)
	case "slack":
		return n.sendSlack(ctx, ch.Target, message)
	case "discord":
		return n.sendDiscord(ctx, ch.Target, message)
	case "lark", "feishu":
		return n.sendLark(ctx, ch.Target, message)
	case "webhook":
		return n.sendWebhook(ctx, ch.Target, message)
	default:
		return fmt.Errorf("unsupported channel type: %s", ch.Type)
	}
}

// sendTelegram sends a message via Telegram Bot API
func (n *Notifier) sendTelegram(ctx context.Context, chatID, message string) error {
	if n.config.TelegramToken == "" {
		return fmt.Errorf("telegram token not configured")
	}
	
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", n.config.TelegramToken)
	
	payload := map[string]interface{}{
		"chat_id":    chatID,
		"text":       message,
		"parse_mode": "Markdown",
	}
	
	return n.postJSON(ctx, apiURL, payload)
}

// sendWhatsApp sends a message via WhatsApp Business API or custom API
func (n *Notifier) sendWhatsApp(ctx context.Context, phone, message string) error {
	if n.config.WhatsAppAPIURL == "" {
		return fmt.Errorf("whatsapp API URL not configured")
	}
	
	// Clean phone number
	phone = strings.TrimPrefix(phone, "+")
	phone = strings.ReplaceAll(phone, " ", "")
	phone = strings.ReplaceAll(phone, "-", "")
	
	payload := map[string]interface{}{
		"phone":   phone,
		"message": message,
	}
	
	req, err := http.NewRequestWithContext(ctx, "POST", n.config.WhatsAppAPIURL, nil)
	if err != nil {
		return err
	}
	
	body, _ := json.Marshal(payload)
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	
	if n.config.WhatsAppAPIKey != "" {
		req.Header.Set("Authorization", "Bearer "+n.config.WhatsAppAPIKey)
	}
	
	resp, err := n.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("whatsapp request failed: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("whatsapp error %d: %s", resp.StatusCode, string(body))
	}
	
	return nil
}

// sendSlack sends a message via Slack API
func (n *Notifier) sendSlack(ctx context.Context, channel, message string) error {
	if n.config.SlackToken == "" {
		return fmt.Errorf("slack token not configured")
	}
	
	apiURL := "https://slack.com/api/chat.postMessage"
	
	payload := map[string]interface{}{
		"channel": channel,
		"text":    message,
	}
	
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+n.config.SlackToken)
	
	resp, err := n.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("slack request failed: %w", err)
	}
	defer resp.Body.Close()
	
	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("slack response parse error: %w", err)
	}
	
	if !result.OK {
		return fmt.Errorf("slack error: %s", result.Error)
	}
	
	return nil
}

// sendDiscord sends a message via Discord (bot token or webhook)
func (n *Notifier) sendDiscord(ctx context.Context, target, message string) error {
	// If target is a webhook URL
	if strings.HasPrefix(target, "http") {
		return n.sendDiscordWebhook(ctx, target, message)
	}

	// If we have a default webhook, use it
	if n.config.DiscordWebhook != "" && !strings.Contains(target, "/") {
		return n.sendDiscordWebhook(ctx, n.config.DiscordWebhook, message)
	}

	// Otherwise, try to use bot token to send to channel ID
	if n.config.DiscordToken != "" {
		return n.sendDiscordBot(ctx, target, message)
	}

	return fmt.Errorf("discord: no token or webhook configured")
}

// sendDiscordWebhook sends via Discord webhook
func (n *Notifier) sendDiscordWebhook(ctx context.Context, webhookURL, message string) error {
	payload := map[string]interface{}{
		"content": message,
	}
	return n.postJSON(ctx, webhookURL, payload)
}

// sendDiscordBot sends via Discord bot token
func (n *Notifier) sendDiscordBot(ctx context.Context, channelID, message string) error {
	apiURL := fmt.Sprintf("https://discord.com/api/v10/channels/%s/messages", channelID)
	
	payload := map[string]interface{}{
		"content": message,
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bot "+n.config.DiscordToken)

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("discord request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("discord error %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// sendLark sends a message via Lark (Feishu) API
func (n *Notifier) sendLark(ctx context.Context, chatID, message string) error {
	if n.config.LarkAppID == "" || n.config.LarkAppSecret == "" {
		return fmt.Errorf("lark app_id and app_secret not configured")
	}

	// Get or refresh access token
	token, err := n.getLarkToken(ctx)
	if err != nil {
		return fmt.Errorf("failed to get lark token: %w", err)
	}

	// Send message
	apiURL := "https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=chat_id"
	
	content, _ := json.Marshal(map[string]string{"text": message})
	payload := map[string]interface{}{
		"receive_id": chatID,
		"msg_type":   "text",
		"content":    string(content),
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("lark request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("lark error %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// getLarkToken gets or refreshes the Lark access token
func (n *Notifier) getLarkToken(ctx context.Context) (string, error) {
	// Check if we have a valid cached token
	now := time.Now().Unix()
	if n.config.larkToken != "" && n.config.larkTokenExpiry > now {
		return n.config.larkToken, nil
	}

	// Get new token
	tokenURL := "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal"
	payload := map[string]string{
		"app_id":     n.config.LarkAppID,
		"app_secret": n.config.LarkAppSecret,
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
		Expire            int    `json:"expire"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if result.Code != 0 {
		return "", fmt.Errorf("lark token error: %s", result.Msg)
	}

	// Cache token (expire 1 minute early for safety)
	n.config.larkToken = result.TenantAccessToken
	n.config.larkTokenExpiry = now + int64(result.Expire) - 60

	return n.config.larkToken, nil
}

// sendWebhook sends a POST request to a custom webhook
func (n *Notifier) sendWebhook(ctx context.Context, webhookURL, message string) error {
	// Validate URL
	if _, err := url.Parse(webhookURL); err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}
	
	payload := map[string]interface{}{
		"message":   message,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	
	return n.postJSON(ctx, webhookURL, payload)
}

// postJSON sends a JSON POST request
func (n *Notifier) postJSON(ctx context.Context, url string, payload interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}
	
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := n.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	
	return nil
}

// TestChannel sends a test message to verify configuration
func (n *Notifier) TestChannel(ctx context.Context, ch NotifyChannel) error {
	testMsg := fmt.Sprintf("🤖 Magabot test notification\nChannel: %s\nTarget: %s\nTime: %s",
		ch.Type, ch.Name, time.Now().Format("2006-01-02 15:04:05"))
	
	return n.Send(ctx, ch, testMsg)
}

// UpdateConfig updates the notifier configuration
func (n *Notifier) UpdateConfig(config NotifierConfig) {
	n.config = config
}

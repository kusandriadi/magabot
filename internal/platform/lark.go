package platform

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

// LarkAdapter implements Lark (Feishu) bot
type LarkAdapter struct {
	appID          string
	appSecret      string
	verifyToken    string
	encryptKey     string
	handler        MessageHandler
	allowedIDs     map[string]bool
	mu             sync.RWMutex
	running        bool
	server         *http.Server
	accessToken    string
	tokenExpiresAt time.Time
	listenAddr     string
}

// LarkConfig holds Lark configuration
type LarkConfig struct {
	AppID       string   `yaml:"app_id" json:"app_id"`
	AppSecret   string   `yaml:"app_secret" json:"app_secret"`
	VerifyToken string   `yaml:"verify_token" json:"verify_token"` // Event verification token
	EncryptKey  string   `yaml:"encrypt_key" json:"encrypt_key"`   // Optional encryption key
	ListenAddr  string   `yaml:"listen_addr" json:"listen_addr"`   // Webhook listen address
	AllowedIDs  []string `yaml:"allowed_ids" json:"allowed_ids"`   // Allowed user/chat IDs
}

// Lark API endpoints
const (
	larkAPIBase      = "https://open.feishu.cn/open-apis"
	larkTokenURL     = larkAPIBase + "/auth/v3/tenant_access_token/internal"
	larkSendMsgURL   = larkAPIBase + "/im/v1/messages"
	larkReplyMsgURL  = larkAPIBase + "/im/v1/messages/%s/reply"
)

// NewLarkAdapter creates a new Lark adapter
func NewLarkAdapter(config LarkConfig) (*LarkAdapter, error) {
	if config.AppID == "" || config.AppSecret == "" {
		return nil, fmt.Errorf("lark app_id and app_secret are required")
	}

	listenAddr := config.ListenAddr
	if listenAddr == "" {
		listenAddr = ":9000"
	}

	allowed := make(map[string]bool)
	for _, id := range config.AllowedIDs {
		allowed[id] = true
	}

	return &LarkAdapter{
		appID:       config.AppID,
		appSecret:   config.AppSecret,
		verifyToken: config.VerifyToken,
		encryptKey:  config.EncryptKey,
		allowedIDs:  allowed,
		listenAddr:  listenAddr,
	}, nil
}

// Start begins the Lark webhook server
func (l *LarkAdapter) Start(handler MessageHandler) error {
	l.mu.Lock()
	if l.running {
		l.mu.Unlock()
		return fmt.Errorf("lark adapter already running")
	}
	l.handler = handler
	l.mu.Unlock()

	// Get initial access token
	if err := l.refreshToken(); err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	// Start token refresh goroutine
	go l.tokenRefreshLoop()

	// Setup HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook/lark", l.handleWebhook)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	l.server = &http.Server{
		Addr:    l.listenAddr,
		Handler: mux,
	}

	l.mu.Lock()
	l.running = true
	l.mu.Unlock()

	log.Printf("[LARK] Webhook server starting on %s", l.listenAddr)

	go func() {
		if err := l.server.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("[LARK] Server error: %v", err)
		}
	}()

	return nil
}

// Stop shuts down the Lark adapter
func (l *LarkAdapter) Stop() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.running {
		return nil
	}

	if l.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		l.server.Shutdown(ctx)
	}

	l.running = false
	log.Println("[LARK] Bot stopped")
	return nil
}

// LarkEvent represents incoming Lark event
type LarkEvent struct {
	Schema    string `json:"schema"`
	Header    LarkEventHeader `json:"header"`
	Event     json.RawMessage `json:"event"`
	Challenge string `json:"challenge"` // URL verification
	Token     string `json:"token"`     // Verification token
	Type      string `json:"type"`      // Event type
}

type LarkEventHeader struct {
	EventID    string `json:"event_id"`
	EventType  string `json:"event_type"`
	CreateTime string `json:"create_time"`
	Token      string `json:"token"`
	AppID      string `json:"app_id"`
	TenantKey  string `json:"tenant_key"`
}

// LarkMessageEvent represents a message event
type LarkMessageEvent struct {
	Sender  LarkSender  `json:"sender"`
	Message LarkMessage `json:"message"`
}

type LarkSender struct {
	SenderID   LarkSenderID `json:"sender_id"`
	SenderType string       `json:"sender_type"`
	TenantKey  string       `json:"tenant_key"`
}

type LarkSenderID struct {
	UnionID string `json:"union_id"`
	UserID  string `json:"user_id"`
	OpenID  string `json:"open_id"`
}

type LarkMessage struct {
	MessageID   string `json:"message_id"`
	RootID      string `json:"root_id"`
	ParentID    string `json:"parent_id"`
	CreateTime  string `json:"create_time"`
	ChatID      string `json:"chat_id"`
	ChatType    string `json:"chat_type"` // p2p or group
	MessageType string `json:"message_type"`
	Content     string `json:"content"`
	Mentions    []LarkMention `json:"mentions"`
}

type LarkMention struct {
	Key    string       `json:"key"`
	ID     LarkSenderID `json:"id"`
	Name   string       `json:"name"`
	TenantKey string    `json:"tenant_key"`
}

// handleWebhook processes incoming Lark events
func (l *LarkAdapter) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var event LarkEvent
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Handle URL verification challenge
	if event.Challenge != "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"challenge": event.Challenge})
		return
	}

	// Verify token
	token := event.Token
	if token == "" {
		token = event.Header.Token
	}
	if l.verifyToken != "" && token != l.verifyToken {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	// Handle message events
	eventType := event.Type
	if eventType == "" {
		eventType = event.Header.EventType
	}

	if eventType == "im.message.receive_v1" {
		go l.handleMessageEvent(event.Event)
	}

	w.WriteHeader(http.StatusOK)
}

// handleMessageEvent processes message events
func (l *LarkAdapter) handleMessageEvent(eventData json.RawMessage) {
	var msgEvent LarkMessageEvent
	if err := json.Unmarshal(eventData, &msgEvent); err != nil {
		log.Printf("[LARK] Failed to parse message event: %v", err)
		return
	}

	// Check authorization
	userID := msgEvent.Sender.SenderID.OpenID
	chatID := msgEvent.Message.ChatID
	if len(l.allowedIDs) > 0 && !l.allowedIDs[userID] && !l.allowedIDs[chatID] {
		log.Printf("[LARK] Unauthorized message from %s", userID)
		return
	}

	// Parse message content
	var content struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(msgEvent.Message.Content), &content); err != nil {
		log.Printf("[LARK] Failed to parse message content: %v", err)
		return
	}

	if content.Text == "" {
		return
	}

	// Create message for handler
	msg := &Message{
		Platform:  "lark",
		ChatID:    chatID,
		MessageID: msgEvent.Message.MessageID,
		UserID:    userID,
		Username:  "", // Lark doesn't provide username in event
		Text:      content.Text,
		IsGroup:   msgEvent.Message.ChatType == "group",
		Raw:       msgEvent,
	}

	// Handle message
	if l.handler != nil {
		response, err := l.handler(msg)
		if err != nil {
			log.Printf("[LARK] Handler error: %v", err)
			l.Reply(chatID, msgEvent.Message.MessageID, fmt.Sprintf("❌ Error: %v", err))
			return
		}
		if response != "" {
			l.Reply(chatID, msgEvent.Message.MessageID, response)
		}
	}
}

// Send sends a message to a Lark chat
func (l *LarkAdapter) Send(chatID, message string) error {
	return l.sendMessage(chatID, "", message)
}

// Reply replies to a specific message
func (l *LarkAdapter) Reply(chatID, messageID, message string) error {
	return l.sendMessage(chatID, messageID, message)
}

// sendMessage sends or replies to a message
func (l *LarkAdapter) sendMessage(chatID, replyToID, message string) error {
	l.mu.RLock()
	token := l.accessToken
	l.mu.RUnlock()

	if token == "" {
		return fmt.Errorf("no access token available")
	}

	content, _ := json.Marshal(map[string]string{"text": message})

	var url string
	var payload map[string]interface{}

	if replyToID != "" {
		// Reply to message
		url = fmt.Sprintf(larkReplyMsgURL, replyToID)
		payload = map[string]interface{}{
			"msg_type": "text",
			"content":  string(content),
		}
	} else {
		// Send new message
		url = larkSendMsgURL + "?receive_id_type=chat_id"
		payload = map[string]interface{}{
			"receive_id": chatID,
			"msg_type":   "text",
			"content":    string(content),
		}
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("lark API error %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// refreshToken gets a new tenant access token
func (l *LarkAdapter) refreshToken() error {
	payload := map[string]string{
		"app_id":     l.appID,
		"app_secret": l.appSecret,
	}

	body, _ := json.Marshal(payload)
	resp, err := http.Post(larkTokenURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
		Expire            int    `json:"expire"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if result.Code != 0 {
		return fmt.Errorf("lark token error: %s", result.Msg)
	}

	l.mu.Lock()
	l.accessToken = result.TenantAccessToken
	l.tokenExpiresAt = time.Now().Add(time.Duration(result.Expire-60) * time.Second) // Refresh 1 min early
	l.mu.Unlock()

	log.Println("[LARK] Access token refreshed")
	return nil
}

// tokenRefreshLoop periodically refreshes the access token
func (l *LarkAdapter) tokenRefreshLoop() {
	for {
		l.mu.RLock()
		running := l.running
		expiresAt := l.tokenExpiresAt
		l.mu.RUnlock()

		if !running {
			return
		}

		// Sleep until token needs refresh
		sleepDuration := time.Until(expiresAt)
		if sleepDuration < time.Minute {
			sleepDuration = time.Minute
		}

		time.Sleep(sleepDuration)

		if err := l.refreshToken(); err != nil {
			log.Printf("[LARK] Token refresh failed: %v", err)
		}
	}
}

// Name returns the platform name
func (l *LarkAdapter) Name() string {
	return "lark"
}

// IsRunning returns whether the adapter is running
func (l *LarkAdapter) IsRunning() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.running
}

// VerifySignature verifies Lark event signature (for encrypted events)
func (l *LarkAdapter) VerifySignature(timestamp, nonce, body, signature string) bool {
	if l.encryptKey == "" {
		return true
	}

	content := timestamp + nonce + l.encryptKey + body
	hash := sha256.Sum256([]byte(content))
	expected := fmt.Sprintf("%x", hash)

	return expected == signature
}

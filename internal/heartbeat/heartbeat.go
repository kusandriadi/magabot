// Package heartbeat provides periodic check service
package heartbeat

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// CheckFunc is a function that performs a check and returns alert message if any
type CheckFunc func(ctx context.Context) (alert string, err error)

// NotifyFunc is called when there's something to notify
type NotifyFunc func(platform, chatID, message string) error

// Check represents a periodic check
type Check struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Interval    time.Duration `json:"interval"` // How often to run
	Enabled     bool          `json:"enabled"`
	LastRun     time.Time     `json:"last_run"`
	LastResult  string        `json:"last_result"` // ok, alert, error
	LastMessage string        `json:"last_message"`
	RunCount    int64         `json:"run_count"`
	AlertCount  int64         `json:"alert_count"`
	Func        CheckFunc     `json:"-"` // The actual check function
}

// Target represents where to send notifications
type Target struct {
	Platform string `json:"platform"`
	ChatID   string `json:"chat_id"`
}

// Service manages heartbeat checks
type Service struct {
	mu       sync.RWMutex
	checks   map[string]*Check
	targets  []Target
	notify   NotifyFunc
	interval time.Duration // Base heartbeat interval
	running  bool
	stopCh   chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
}

// Config holds heartbeat configuration
type Config struct {
	Enabled  bool          `yaml:"enabled"`
	Interval time.Duration `yaml:"interval"` // Base interval (e.g., 30m)
	Targets  []Target      `yaml:"targets"`  // Where to send alerts
}

// NewService creates a new heartbeat service
func NewService(interval time.Duration, notify NotifyFunc) *Service {
	if interval < time.Minute {
		interval = 30 * time.Minute
	}

	return &Service{
		checks:   make(map[string]*Check),
		interval: interval,
		notify:   notify,
		stopCh:   make(chan struct{}),
	}
}

// AddCheck registers a new check
func (s *Service) AddCheck(name, description string, interval time.Duration, fn CheckFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if interval < time.Minute {
		interval = s.interval
	}

	s.checks[name] = &Check{
		Name:        name,
		Description: description,
		Interval:    interval,
		Enabled:     true,
		Func:        fn,
	}
}

// RemoveCheck removes a check
func (s *Service) RemoveCheck(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.checks, name)
}

// EnableCheck enables a check
func (s *Service) EnableCheck(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if check, ok := s.checks[name]; ok {
		check.Enabled = true
	}
}

// DisableCheck disables a check
func (s *Service) DisableCheck(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if check, ok := s.checks[name]; ok {
		check.Enabled = false
	}
}

// AddTarget adds a notification target
func (s *Service) AddTarget(platform, chatID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.targets = append(s.targets, Target{Platform: platform, ChatID: chatID})
}

// SetTargets sets all notification targets
func (s *Service) SetTargets(targets []Target) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.targets = targets
}

// Start begins the heartbeat service
func (s *Service) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.mu.Unlock()

	log.Printf("[HEARTBEAT] Service started (interval: %v)", s.interval)

	go s.loop()
}

// Stop halts the heartbeat service
func (s *Service) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.cancel()
	s.mu.Unlock()

	log.Println("[HEARTBEAT] Service stopped")
}

// loop runs the heartbeat loop
func (s *Service) loop() {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// Run immediately on start
	s.runChecks()

	for {
		select {
		case <-ticker.C:
			s.runChecks()
		case <-s.ctx.Done():
			return
		}
	}
}

// runChecks executes all due checks
func (s *Service) runChecks() {
	s.mu.RLock()
	checks := make([]*Check, 0, len(s.checks))
	for _, check := range s.checks {
		if check.Enabled && s.isDue(check) {
			checks = append(checks, check)
		}
	}
	s.mu.RUnlock()

	if len(checks) == 0 {
		return
	}

	log.Printf("[HEARTBEAT] Running %d checks", len(checks))

	var alerts []string

	for _, check := range checks {
		alert, err := s.runCheck(check)
		if err != nil {
			log.Printf("[HEARTBEAT] Check %s error: %v", check.Name, err)
		}
		if alert != "" {
			alerts = append(alerts, fmt.Sprintf("🔔 %s: %s", check.Name, alert))
		}
	}

	// Send consolidated alerts
	if len(alerts) > 0 {
		s.sendAlerts(alerts)
	}
}

// isDue checks if a check should run
func (s *Service) isDue(check *Check) bool {
	if check.LastRun.IsZero() {
		return true
	}
	return time.Since(check.LastRun) >= check.Interval
}

// runCheck executes a single check
func (s *Service) runCheck(check *Check) (string, error) {
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	alert, err := check.Func(ctx)

	s.mu.Lock()
	check.LastRun = time.Now()
	check.RunCount++
	if err != nil {
		check.LastResult = "error"
		check.LastMessage = err.Error()
	} else if alert != "" {
		check.LastResult = "alert"
		check.LastMessage = alert
		check.AlertCount++
	} else {
		check.LastResult = "ok"
		check.LastMessage = ""
	}
	s.mu.Unlock()

	return alert, err
}

// sendAlerts sends alerts to all targets
func (s *Service) sendAlerts(alerts []string) {
	if s.notify == nil {
		return
	}

	message := "💓 *Heartbeat Alert*\n\n" + joinAlerts(alerts)

	s.mu.RLock()
	targets := s.targets
	s.mu.RUnlock()

	for _, target := range targets {
		if err := s.notify(target.Platform, target.ChatID, message); err != nil {
			log.Printf("[HEARTBEAT] Failed to notify %s/%s: %v", target.Platform, target.ChatID, err)
		}
	}
}

// Status returns the current status of all checks
func (s *Service) Status() map[string]*Check {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*Check)
	for name, check := range s.checks {
		// Copy to avoid race
		c := *check
		result[name] = &c
	}
	return result
}

// RunNow triggers all checks immediately
func (s *Service) RunNow() []string {
	s.mu.RLock()
	checks := make([]*Check, 0, len(s.checks))
	for _, check := range s.checks {
		if check.Enabled {
			checks = append(checks, check)
		}
	}
	s.mu.RUnlock()

	var alerts []string
	for _, check := range checks {
		alert, err := s.runCheck(check)
		if err != nil {
			alerts = append(alerts, fmt.Sprintf("❌ %s: %v", check.Name, err))
		} else if alert != "" {
			alerts = append(alerts, fmt.Sprintf("🔔 %s: %s", check.Name, alert))
		} else {
			alerts = append(alerts, fmt.Sprintf("✅ %s: OK", check.Name))
		}
	}

	return alerts
}

func joinAlerts(alerts []string) string {
	result := ""
	for i, alert := range alerts {
		if i > 0 {
			result += "\n"
		}
		result += alert
	}
	return result
}

// Built-in check creators

// NewTimeCheck creates a check that alerts at specific times
func NewTimeCheck(hours []int) CheckFunc {
	return func(ctx context.Context) (string, error) {
		hour := time.Now().Hour()
		for _, h := range hours {
			if hour == h {
				return fmt.Sprintf("It's %d:00", hour), nil
			}
		}
		return "", nil
	}
}

// NewHTTPCheck creates a check that pings a URL
func NewHTTPCheck(url string, expectStatus int) CheckFunc {
	return func(ctx context.Context) (string, error) {
		// Implementation would use http.Get with context
		// For now, return empty (no alert)
		return "", nil
	}
}

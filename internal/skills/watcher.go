// Package skills - File watcher for auto-reload
package skills

import (
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Watcher monitors skills directory for changes and triggers reload
type Watcher struct {
	manager    *Manager
	logger     *slog.Logger
	stopCh     chan struct{}
	wg         sync.WaitGroup
	interval   time.Duration
	lastMod    map[string]time.Time
	mu         sync.Mutex
}

// NewWatcher creates a new skills watcher
func NewWatcher(manager *Manager, logger *slog.Logger) *Watcher {
	return &Watcher{
		manager:  manager,
		logger:   logger,
		stopCh:   make(chan struct{}),
		interval: 5 * time.Second, // Check every 5 seconds
		lastMod:  make(map[string]time.Time),
	}
}

// Start begins watching for changes
func (w *Watcher) Start() {
	w.wg.Add(1)
	go w.watchLoop()
	w.logger.Info("skills watcher started", "dir", w.manager.skillsDir, "interval", w.interval)
}

// Stop stops the watcher
func (w *Watcher) Stop() {
	close(w.stopCh)
	w.wg.Wait()
	w.logger.Info("skills watcher stopped")
}

func (w *Watcher) watchLoop() {
	defer w.wg.Done()

	// Initial scan to record modification times
	w.scanAndRecord()

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			if w.hasChanges() {
				w.logger.Info("skills directory changed, reloading...")
				if err := w.manager.LoadAll(); err != nil {
					w.logger.Error("failed to reload skills", "error", err)
				} else {
					w.logger.Info("skills reloaded", "count", len(w.manager.skills))
				}
				w.scanAndRecord() // Update modification times
			}
		}
	}
}

// scanAndRecord scans the skills directory and records modification times
func (w *Watcher) scanAndRecord() {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.lastMod = make(map[string]time.Time)

	filepath.Walk(w.manager.skillsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		// Only watch skill.yaml files and directories
		if info.Name() == "skill.yaml" || info.Name() == "SKILL.yaml" || info.IsDir() {
			w.lastMod[path] = info.ModTime()
		}

		return nil
	})
}

// hasChanges checks if any files have been modified, added, or removed
func (w *Watcher) hasChanges() bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	current := make(map[string]time.Time)
	
	filepath.Walk(w.manager.skillsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.Name() == "skill.yaml" || info.Name() == "SKILL.yaml" || info.IsDir() {
			current[path] = info.ModTime()
		}

		return nil
	})

	// Check for modifications or new files
	for path, modTime := range current {
		lastMod, exists := w.lastMod[path]
		if !exists {
			// New file
			return true
		}
		if !modTime.Equal(lastMod) {
			// Modified file
			return true
		}
	}

	// Check for deleted files
	for path := range w.lastMod {
		if _, exists := current[path]; !exists {
			return true
		}
	}

	return false
}

// SetInterval sets the polling interval
func (w *Watcher) SetInterval(d time.Duration) {
	w.interval = d
}

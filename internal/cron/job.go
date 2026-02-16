// Package cron provides scheduled job management with multi-channel notifications
package cron

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// NotifyChannel represents a notification destination
type NotifyChannel struct {
	Type   string `json:"type"`   // telegram, whatsapp, slack, discord, webhook
	Target string `json:"target"` // chat_id, phone, channel, webhook_url
	Name   string `json:"name"`   // friendly name for display
}

// Job represents a scheduled cron job
type Job struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Schedule    string          `json:"schedule"` // cron expression (e.g., "0 9 * * 1-5")
	Message     string          `json:"message"`  // message to send
	Channels    []NotifyChannel `json:"channels"` // where to send
	Enabled     bool            `json:"enabled"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	LastRunAt   *time.Time      `json:"last_run_at,omitempty"`
	LastError   string          `json:"last_error,omitempty"`
	RunCount    int64           `json:"run_count"`
}

// JobStore manages persistent storage of cron jobs
type JobStore struct {
	mu       sync.RWMutex
	jobs     map[string]*Job
	filePath string
}

// NewJobStore creates a new job store
func NewJobStore(dataDir string) (*JobStore, error) {
	filePath := filepath.Join(dataDir, "cron_jobs.json")

	store := &JobStore{
		jobs:     make(map[string]*Job),
		filePath: filePath,
	}

	// Load existing jobs
	if err := store.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load jobs: %w", err)
	}

	return store, nil
}

// load reads jobs from disk
func (s *JobStore) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}

	var jobs []*Job
	if err := json.Unmarshal(data, &jobs); err != nil {
		return fmt.Errorf("failed to parse jobs: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.jobs = make(map[string]*Job)
	for _, job := range jobs {
		s.jobs[job.ID] = job
	}

	return nil
}

// save writes jobs to disk
func (s *JobStore) save() error {
	s.mu.RLock()
	jobs := make([]*Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobs = append(jobs, job)
	}
	s.mu.RUnlock()

	data, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal jobs: %w", err)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(s.filePath), 0700); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write atomically
	tmpFile := s.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write jobs: %w", err)
	}

	if err := os.Rename(tmpFile, s.filePath); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to rename jobs file: %w", err)
	}

	return nil
}

// Create adds a new job
func (s *JobStore) Create(job *Job) error {
	if job.ID == "" {
		job.ID = uuid.New().String()[:8] // short ID for usability
	}
	job.CreatedAt = time.Now()
	job.UpdatedAt = time.Now()

	s.mu.Lock()
	s.jobs[job.ID] = job
	s.mu.Unlock()

	return s.save()
}

// Update modifies an existing job
func (s *JobStore) Update(job *Job) error {
	s.mu.Lock()
	if _, exists := s.jobs[job.ID]; !exists {
		s.mu.Unlock()
		return fmt.Errorf("job not found: %s", job.ID)
	}
	job.UpdatedAt = time.Now()
	s.jobs[job.ID] = job
	s.mu.Unlock()

	return s.save()
}

// Delete removes a job
func (s *JobStore) Delete(id string) error {
	s.mu.Lock()
	if _, exists := s.jobs[id]; !exists {
		s.mu.Unlock()
		return fmt.Errorf("job not found: %s", id)
	}
	delete(s.jobs, id)
	s.mu.Unlock()

	return s.save()
}

// Get retrieves a job by ID
func (s *JobStore) Get(id string) (*Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	job, exists := s.jobs[id]
	if !exists {
		return nil, fmt.Errorf("job not found: %s", id)
	}

	// Return a copy
	jobCopy := *job
	return &jobCopy, nil
}

// List returns all jobs
func (s *JobStore) List() []*Job {
	s.mu.RLock()
	defer s.mu.RUnlock()

	jobs := make([]*Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobCopy := *job
		jobs = append(jobs, &jobCopy)
	}

	return jobs
}

// ListEnabled returns only enabled jobs
func (s *JobStore) ListEnabled() []*Job {
	s.mu.RLock()
	defer s.mu.RUnlock()

	jobs := make([]*Job, 0)
	for _, job := range s.jobs {
		if job.Enabled {
			jobCopy := *job
			jobs = append(jobs, &jobCopy)
		}
	}

	return jobs
}

// SetEnabled enables or disables a job
func (s *JobStore) SetEnabled(id string, enabled bool) error {
	s.mu.Lock()
	job, exists := s.jobs[id]
	if !exists {
		s.mu.Unlock()
		return fmt.Errorf("job not found: %s", id)
	}
	job.Enabled = enabled
	job.UpdatedAt = time.Now()
	s.mu.Unlock()

	return s.save()
}

// RecordRun updates the job after execution
func (s *JobStore) RecordRun(id string, err error) error {
	s.mu.Lock()
	job, exists := s.jobs[id]
	if !exists {
		s.mu.Unlock()
		return fmt.Errorf("job not found: %s", id)
	}

	now := time.Now()
	job.LastRunAt = &now
	job.RunCount++
	if err != nil {
		job.LastError = err.Error()
	} else {
		job.LastError = ""
	}
	s.mu.Unlock()

	return s.save()
}

// ValidateSchedule checks if a cron expression is valid
func ValidateSchedule(schedule string) error {
	// Use cron parser to validate
	// Common patterns:
	// "* * * * *"     - every minute
	// "0 9 * * 1-5"   - 9am weekdays
	// "0 */2 * * *"   - every 2 hours
	// "@hourly"       - every hour
	// "@daily"        - every day at midnight

	// Basic validation - more thorough validation in scheduler
	if schedule == "" {
		return fmt.Errorf("schedule cannot be empty")
	}
	return nil
}

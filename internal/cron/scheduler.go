package cron

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/robfig/cron/v3"
)

// Scheduler manages cron job execution
type Scheduler struct {
	mu       sync.RWMutex
	cron     *cron.Cron
	store    *JobStore
	notifier *Notifier
	entryIDs map[string]cron.EntryID // job ID -> cron entry ID
	running  bool
}

// NewScheduler creates a new scheduler
func NewScheduler(store *JobStore, notifier *Notifier) *Scheduler {
	return &Scheduler{
		cron:     cron.New(cron.WithSeconds()), // support seconds
		store:    store,
		notifier: notifier,
		entryIDs: make(map[string]cron.EntryID),
	}
}

// Start begins the scheduler
func (s *Scheduler) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil
	}

	// Load all enabled jobs
	jobs := s.store.ListEnabled()
	for _, job := range jobs {
		if err := s.scheduleJob(job); err != nil {
			log.Printf("[CRON] Failed to schedule job %s: %v", job.ID, err)
		}
	}

	s.cron.Start()
	s.running = true

	log.Printf("[CRON] Scheduler started with %d jobs", len(jobs))
	return nil
}

// Stop halts the scheduler
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	ctx := s.cron.Stop()
	<-ctx.Done()
	s.running = false

	log.Println("[CRON] Scheduler stopped")
}

// scheduleJob adds a job to the cron scheduler (must hold lock)
func (s *Scheduler) scheduleJob(job *Job) error {
	// Create the job function
	jobFunc := s.createJobFunc(job.ID)

	// Parse schedule - support both 5-field and 6-field (with seconds)
	schedule := job.Schedule

	// Add to cron
	entryID, err := s.cron.AddFunc(schedule, jobFunc)
	if err != nil {
		// Try with "0 " prefix for 5-field cron expressions
		entryID, err = s.cron.AddFunc("0 "+schedule, jobFunc)
		if err != nil {
			return fmt.Errorf("invalid schedule: %w", err)
		}
	}

	s.entryIDs[job.ID] = entryID
	log.Printf("[CRON] Scheduled job %s (%s): %s", job.ID, job.Name, schedule)

	return nil
}

// createJobFunc returns the function to execute for a job
func (s *Scheduler) createJobFunc(jobID string) func() {
	return func() {
		// Get current job state
		job, err := s.store.Get(jobID)
		if err != nil {
			log.Printf("[CRON] Job %s not found: %v", jobID, err)
			return
		}

		if !job.Enabled {
			log.Printf("[CRON] Job %s is disabled, skipping", jobID)
			return
		}

		log.Printf("[CRON] Running job %s (%s)", job.ID, job.Name)

		// Send notifications to all channels
		var lastErr error
		for _, ch := range job.Channels {
			if err := s.notifier.Send(context.Background(), ch, job.Message); err != nil {
				log.Printf("[CRON] Failed to send to %s/%s: %v", ch.Type, ch.Target, err)
				lastErr = err
			}
		}

		// Record the run
		_ = s.store.RecordRun(jobID, lastErr)
	}
}

// AddJob creates and schedules a new job
func (s *Scheduler) AddJob(job *Job) error {
	// Create in store first
	if err := s.store.Create(job); err != nil {
		return err
	}

	// Schedule if enabled and running
	if job.Enabled {
		s.mu.Lock()
		defer s.mu.Unlock()

		if s.running {
			if err := s.scheduleJob(job); err != nil {
				// Rollback
				_ = s.store.Delete(job.ID)
				return err
			}
		}
	}

	return nil
}

// UpdateJob modifies an existing job
func (s *Scheduler) UpdateJob(job *Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove old schedule if exists
	if entryID, exists := s.entryIDs[job.ID]; exists {
		s.cron.Remove(entryID)
		delete(s.entryIDs, job.ID)
	}

	// Update in store
	if err := s.store.Update(job); err != nil {
		return err
	}

	// Reschedule if enabled and running
	if job.Enabled && s.running {
		if err := s.scheduleJob(job); err != nil {
			return err
		}
	}

	return nil
}

// DeleteJob removes a job
func (s *Scheduler) DeleteJob(id string) error {
	s.mu.Lock()

	// Remove from cron
	if entryID, exists := s.entryIDs[id]; exists {
		s.cron.Remove(entryID)
		delete(s.entryIDs, id)
	}

	s.mu.Unlock()

	// Delete from store
	return s.store.Delete(id)
}

// EnableJob enables a job
func (s *Scheduler) EnableJob(id string) error {
	if err := s.store.SetEnabled(id, true); err != nil {
		return err
	}

	job, err := s.store.Get(id)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return s.scheduleJob(job)
	}

	return nil
}

// DisableJob disables a job
func (s *Scheduler) DisableJob(id string) error {
	s.mu.Lock()

	// Remove from cron
	if entryID, exists := s.entryIDs[id]; exists {
		s.cron.Remove(entryID)
		delete(s.entryIDs, id)
	}

	s.mu.Unlock()

	return s.store.SetEnabled(id, false)
}

// RunNow executes a job immediately
func (s *Scheduler) RunNow(id string) error {
	job, err := s.store.Get(id)
	if err != nil {
		return err
	}

	log.Printf("[CRON] Manual run job %s (%s)", job.ID, job.Name)

	// Send notifications
	var lastErr error
	for _, ch := range job.Channels {
		if err := s.notifier.Send(context.Background(), ch, job.Message); err != nil {
			log.Printf("[CRON] Failed to send to %s/%s: %v", ch.Type, ch.Target, err)
			lastErr = err
		}
	}

	// Record the run
	_ = s.store.RecordRun(id, lastErr)

	return lastErr
}

// GetJob retrieves a job
func (s *Scheduler) GetJob(id string) (*Job, error) {
	return s.store.Get(id)
}

// ListJobs returns all jobs
func (s *Scheduler) ListJobs() []*Job {
	return s.store.List()
}

// Status returns scheduler status
func (s *Scheduler) Status() (bool, int, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	total := len(s.store.List())
	active := len(s.entryIDs)

	return s.running, total, active
}

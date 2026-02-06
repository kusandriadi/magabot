package cron

import (
	"os"
	"testing"
	"time"
)

func TestJobStore(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "magabot-cron-test-*")
	defer os.RemoveAll(tmpDir)

	store, err := NewJobStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// Test Create
	job := &Job{
		Name:     "Test Job",
		Schedule: "0 9 * * *",
		Message:  "Hello World",
		Channels: []NotifyChannel{
			{Type: "telegram", Target: "123456", Name: "Test"},
		},
		Enabled: true,
	}

	err = store.Create(job)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if job.ID == "" {
		t.Error("Job should have an ID after creation")
	}

	// Test Get
	retrieved, err := store.Get(job.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved.Name != "Test Job" {
		t.Errorf("Expected name 'Test Job', got %s", retrieved.Name)
	}

	// Test List
	jobs := store.List()
	if len(jobs) != 1 {
		t.Errorf("Expected 1 job, got %d", len(jobs))
	}

	// Test ListEnabled
	enabledJobs := store.ListEnabled()
	if len(enabledJobs) != 1 {
		t.Errorf("Expected 1 enabled job, got %d", len(enabledJobs))
	}

	// Test SetEnabled
	err = store.SetEnabled(job.ID, false)
	if err != nil {
		t.Fatalf("SetEnabled failed: %v", err)
	}

	enabledJobs = store.ListEnabled()
	if len(enabledJobs) != 0 {
		t.Errorf("Expected 0 enabled jobs, got %d", len(enabledJobs))
	}

	// Test Update
	retrieved.Message = "Updated Message"
	err = store.Update(retrieved)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	updated, _ := store.Get(job.ID)
	if updated.Message != "Updated Message" {
		t.Errorf("Expected 'Updated Message', got %s", updated.Message)
	}

	// Test RecordRun
	err = store.RecordRun(job.ID, nil)
	if err != nil {
		t.Fatalf("RecordRun failed: %v", err)
	}

	recorded, _ := store.Get(job.ID)
	if recorded.LastRunAt == nil {
		t.Error("LastRunAt should be set")
	}
	if recorded.RunCount != 1 {
		t.Errorf("RunCount should be 1, got %d", recorded.RunCount)
	}

	// Test Delete
	err = store.Delete(job.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	jobs = store.List()
	if len(jobs) != 0 {
		t.Errorf("Expected 0 jobs after delete, got %d", len(jobs))
	}
}

func TestJobPersistence(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "magabot-cron-test-*")
	defer os.RemoveAll(tmpDir)

	// Create and save job
	store1, _ := NewJobStore(tmpDir)
	job := &Job{
		Name:     "Persistent Job",
		Schedule: "0 10 * * *",
		Message:  "Persisted",
		Enabled:  true,
	}
	store1.Create(job)
	jobID := job.ID

	// Create new store (simulates restart)
	store2, err := NewJobStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create second store: %v", err)
	}

	// Verify job persisted
	loaded, err := store2.Get(jobID)
	if err != nil {
		t.Fatalf("Failed to load job: %v", err)
	}

	if loaded.Name != "Persistent Job" {
		t.Errorf("Expected 'Persistent Job', got %s", loaded.Name)
	}
}

func TestValidateSchedule(t *testing.T) {
	tests := []struct {
		schedule string
		valid    bool
	}{
		{"0 9 * * *", true},
		{"@hourly", true},
		{"@daily", true},
		{"", false},
	}

	for _, tt := range tests {
		err := ValidateSchedule(tt.schedule)
		if tt.valid && err != nil {
			t.Errorf("Schedule %q should be valid, got error: %v", tt.schedule, err)
		}
		if !tt.valid && err == nil {
			t.Errorf("Schedule %q should be invalid", tt.schedule)
		}
	}
}

func TestJobTimestamps(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "magabot-cron-test-*")
	defer os.RemoveAll(tmpDir)

	store, _ := NewJobStore(tmpDir)

	before := time.Now()
	job := &Job{
		Name:     "Timestamp Test",
		Schedule: "0 9 * * *",
		Enabled:  true,
	}
	store.Create(job)
	after := time.Now()

	if job.CreatedAt.Before(before) || job.CreatedAt.After(after) {
		t.Error("CreatedAt should be between before and after")
	}

	if job.UpdatedAt.Before(before) || job.UpdatedAt.After(after) {
		t.Error("UpdatedAt should be between before and after")
	}
}

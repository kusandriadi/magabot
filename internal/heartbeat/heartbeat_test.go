package heartbeat_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kusa/magabot/internal/heartbeat"
)

func noopNotify(platform, chatID, message string) error {
	return nil
}

func TestNewService_DefaultInterval(t *testing.T) {
	// Interval below 1 minute should default to 30m.
	svc := heartbeat.NewService(10*time.Second, noopNotify)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	// We cannot directly inspect the interval from outside, but we verify
	// the service was created without error and is functional.
	status := svc.Status()
	if status == nil {
		t.Error("Status should return non-nil map")
	}
	if len(status) != 0 {
		t.Errorf("expected 0 checks initially, got %d", len(status))
	}
}

func TestNewService_CustomInterval(t *testing.T) {
	svc := heartbeat.NewService(5*time.Minute, noopNotify)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestNewService_NilNotify(t *testing.T) {
	// Should not panic with nil NotifyFunc.
	svc := heartbeat.NewService(5*time.Minute, nil)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestAddRemoveCheck(t *testing.T) {
	svc := heartbeat.NewService(5*time.Minute, noopNotify)

	svc.AddCheck("disk", "Check disk space", 5*time.Minute,
		func(ctx context.Context) (string, error) { return "", nil })

	status := svc.Status()
	if _, ok := status["disk"]; !ok {
		t.Fatal("expected 'disk' check in status after AddCheck")
	}
	if status["disk"].Name != "disk" {
		t.Errorf("expected check name 'disk', got %q", status["disk"].Name)
	}
	if status["disk"].Description != "Check disk space" {
		t.Errorf("expected description 'Check disk space', got %q", status["disk"].Description)
	}
	if !status["disk"].Enabled {
		t.Error("expected check to be enabled by default")
	}

	svc.RemoveCheck("disk")

	status = svc.Status()
	if _, ok := status["disk"]; ok {
		t.Error("expected 'disk' check to be removed")
	}
}

func TestRemoveCheck_Nonexistent(t *testing.T) {
	svc := heartbeat.NewService(5*time.Minute, noopNotify)
	// Should not panic.
	svc.RemoveCheck("nonexistent")
}

func TestEnableDisableCheck(t *testing.T) {
	svc := heartbeat.NewService(5*time.Minute, noopNotify)

	svc.AddCheck("mem", "Memory check", 5*time.Minute,
		func(ctx context.Context) (string, error) { return "", nil })

	// Initially enabled.
	status := svc.Status()
	if !status["mem"].Enabled {
		t.Error("expected check to be enabled initially")
	}

	svc.DisableCheck("mem")
	status = svc.Status()
	if status["mem"].Enabled {
		t.Error("expected check to be disabled after DisableCheck")
	}

	svc.EnableCheck("mem")
	status = svc.Status()
	if !status["mem"].Enabled {
		t.Error("expected check to be re-enabled after EnableCheck")
	}
}

func TestEnableDisableCheck_Nonexistent(t *testing.T) {
	svc := heartbeat.NewService(5*time.Minute, noopNotify)
	// Should not panic when enabling/disabling nonexistent checks.
	svc.EnableCheck("ghost")
	svc.DisableCheck("ghost")
}

func TestRunNow_NoChecks(t *testing.T) {
	svc := heartbeat.NewService(5*time.Minute, noopNotify)

	// Start so that ctx is set, then run checks.
	svc.Start()
	defer svc.Stop()

	results := svc.RunNow()
	if len(results) != 0 {
		t.Errorf("expected empty results with no checks, got %v", results)
	}
}

func TestRunNow_OKCheck(t *testing.T) {
	svc := heartbeat.NewService(5*time.Minute, noopNotify)

	svc.AddCheck("healthy", "Always OK", 5*time.Minute,
		func(ctx context.Context) (string, error) { return "", nil })

	svc.Start()
	defer svc.Stop()

	results := svc.RunNow()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !strings.Contains(results[0], "OK") {
		t.Errorf("expected result to contain 'OK', got %q", results[0])
	}
	if !strings.Contains(results[0], "healthy") {
		t.Errorf("expected result to contain check name 'healthy', got %q", results[0])
	}
}

func TestRunNow_AlertCheck(t *testing.T) {
	svc := heartbeat.NewService(5*time.Minute, noopNotify)

	svc.AddCheck("disk", "Disk check", 5*time.Minute,
		func(ctx context.Context) (string, error) { return "disk full", nil })

	svc.Start()
	defer svc.Stop()

	results := svc.RunNow()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !strings.Contains(results[0], "disk full") {
		t.Errorf("expected result to contain 'disk full', got %q", results[0])
	}
}

func TestRunNow_ErrorCheck(t *testing.T) {
	svc := heartbeat.NewService(5*time.Minute, noopNotify)

	svc.AddCheck("broken", "Broken check", 5*time.Minute,
		func(ctx context.Context) (string, error) {
			return "", fmt.Errorf("connection refused")
		})

	svc.Start()
	defer svc.Stop()

	results := svc.RunNow()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !strings.Contains(results[0], "connection refused") {
		t.Errorf("expected result to contain error message, got %q", results[0])
	}
}

func TestRunNow_DisabledCheckSkipped(t *testing.T) {
	svc := heartbeat.NewService(5*time.Minute, noopNotify)

	svc.AddCheck("disabled-check", "Disabled", 5*time.Minute,
		func(ctx context.Context) (string, error) { return "should not appear", nil })
	svc.DisableCheck("disabled-check")

	svc.Start()
	defer svc.Stop()

	results := svc.RunNow()
	if len(results) != 0 {
		t.Errorf("expected 0 results for disabled check, got %d: %v", len(results), results)
	}
}

func TestRunNow_MultipleChecks(t *testing.T) {
	svc := heartbeat.NewService(5*time.Minute, noopNotify)

	svc.AddCheck("check-a", "A", 5*time.Minute,
		func(ctx context.Context) (string, error) { return "", nil })
	svc.AddCheck("check-b", "B", 5*time.Minute,
		func(ctx context.Context) (string, error) { return "alert-b", nil })

	svc.Start()
	defer svc.Stop()

	results := svc.RunNow()
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestStartStop(t *testing.T) {
	svc := heartbeat.NewService(5*time.Minute, noopNotify)

	svc.Start()

	// Allow the goroutine to start.
	time.Sleep(50 * time.Millisecond)

	// Calling Start again should be safe (idempotent).
	svc.Start()

	svc.Stop()

	// Calling Stop again should be safe (idempotent).
	svc.Stop()
}

func TestStatus_ReturnsCopy(t *testing.T) {
	svc := heartbeat.NewService(5*time.Minute, noopNotify)

	svc.AddCheck("original", "Original check", 5*time.Minute,
		func(ctx context.Context) (string, error) { return "", nil })

	status := svc.Status()
	if _, ok := status["original"]; !ok {
		t.Fatal("expected 'original' check in status")
	}

	// Modify the returned map.
	delete(status, "original")
	status["injected"] = nil

	// Verify original is unaffected.
	status2 := svc.Status()
	if _, ok := status2["original"]; !ok {
		t.Error("modifying returned map should not affect service; 'original' missing")
	}
	if _, ok := status2["injected"]; ok {
		t.Error("injected key should not appear in service status")
	}
}

func TestAddTarget(t *testing.T) {
	svc := heartbeat.NewService(5*time.Minute, noopNotify)

	svc.AddTarget("telegram", "12345")
	svc.AddTarget("slack", "C001")

	// We cannot directly inspect targets from outside, but we ensure no panic.
}

func TestSetTargets(t *testing.T) {
	svc := heartbeat.NewService(5*time.Minute, noopNotify)

	targets := []heartbeat.Target{
		{Platform: "telegram", ChatID: "111"},
		{Platform: "slack", ChatID: "222"},
	}
	svc.SetTargets(targets)

	// Overwrite with empty.
	svc.SetTargets(nil)
}

func TestAddCheck_ShortIntervalDefaultsToServiceInterval(t *testing.T) {
	svc := heartbeat.NewService(5*time.Minute, noopNotify)

	// Interval below 1 minute should default to the service interval.
	svc.AddCheck("short", "Short interval", 10*time.Second,
		func(ctx context.Context) (string, error) { return "", nil })

	status := svc.Status()
	check, ok := status["short"]
	if !ok {
		t.Fatal("expected 'short' check")
	}
	if check.Interval < time.Minute {
		t.Errorf("expected interval >= 1 minute (defaulted), got %v", check.Interval)
	}
}

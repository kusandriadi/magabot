package cron

import (
	"testing"
	"time"
)

func TestParseScheduleCron(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{"simple cron", "* * * * *", false},
		{"specific time", "30 9 * * *", false},
		{"weekdays", "0 9 * * 1-5", false},
		{"with seconds", "0 30 9 * * *", false},
		{"ranges", "0-30 9-17 * * *", false},
		{"steps", "*/5 * * * *", false},
		{"complex", "0,30 9-17/2 * JAN-JUN MON-FRI", false},
		{"too few fields", "* * *", true},
		{"too many fields", "* * * * * * *", true},
		{"invalid range", "60 * * * *", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseSchedule(tt.expr, "")
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSchedule(%q) error = %v, wantErr %v", tt.expr, err, tt.wantErr)
			}
		})
	}
}

func TestParseSchedulePredefined(t *testing.T) {
	tests := []struct {
		name string
		expr string
	}{
		{"hourly", "@hourly"},
		{"daily", "@daily"},
		{"midnight", "@midnight"},
		{"weekly", "@weekly"},
		{"monthly", "@monthly"},
		{"yearly", "@yearly"},
		{"annually", "@annually"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := ParseSchedule(tt.expr, "")
			if err != nil {
				t.Fatalf("ParseSchedule(%q) error = %v", tt.expr, err)
			}
			if s.Type != ScheduleCron {
				t.Errorf("expected type cron, got %s", s.Type)
			}
		})
	}
}

func TestParseScheduleEvery(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		expected time.Duration
		wantErr  bool
	}{
		{"5 minutes", "every 5m", 5 * time.Minute, false},
		{"2 hours", "every 2h", 2 * time.Hour, false},
		{"30 seconds", "every 30s", 30 * time.Second, false},
		{"complex", "every 1h30m", 90 * time.Minute, false},
		{"minute shortcut", "every minute", time.Minute, false},
		{"hour shortcut", "every hour", time.Hour, false},
		{"day shortcut", "every day", 24 * time.Hour, false},
		{"invalid", "every abc", 0, true},
		{"too short", "every 100ms", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := ParseSchedule(tt.expr, "")
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSchedule(%q) error = %v, wantErr %v", tt.expr, err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if s.Type != ScheduleEvery {
					t.Errorf("expected type every, got %s", s.Type)
				}
				if s.Interval != tt.expected {
					t.Errorf("expected interval %v, got %v", tt.expected, s.Interval)
				}
			}
		})
	}
}

func TestParseScheduleAt(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{"RFC3339", "at 2024-02-14T15:30:00Z", false},
		{"datetime", "at 2024-02-14 15:30:00", false},
		{"date only", "at 2024-02-14", false},
		{"time only", "at 15:30", false},
		{"time with seconds", "at 15:30:45", false},
		{"invalid", "at not-a-time", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := ParseSchedule(tt.expr, "")
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSchedule(%q) error = %v, wantErr %v", tt.expr, err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if s.Type != ScheduleAt {
					t.Errorf("expected type at, got %s", s.Type)
				}
				if s.AtTime == nil {
					t.Error("AtTime should be set")
				}
			}
		})
	}
}

func TestScheduleWithTimezone(t *testing.T) {
	// Parse a schedule in Jakarta timezone
	s, err := ParseSchedule("0 9 * * *", "Asia/Jakarta")
	if err != nil {
		t.Fatalf("ParseSchedule error = %v", err)
	}

	if s.Timezone != "Asia/Jakarta" {
		t.Errorf("expected timezone Asia/Jakarta, got %s", s.Timezone)
	}

	// Test invalid timezone
	_, err = ParseSchedule("0 9 * * *", "Invalid/Timezone")
	if err == nil {
		t.Error("expected error for invalid timezone")
	}
}

func TestScheduleNext(t *testing.T) {
	// Test cron schedule
	s, _ := ParseSchedule("0 9 * * *", "UTC")
	now := time.Date(2024, 2, 14, 8, 0, 0, 0, time.UTC)
	next := s.Next(now)

	expected := time.Date(2024, 2, 14, 9, 0, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("expected next at %v, got %v", expected, next)
	}

	// Test every schedule
	s, _ = ParseSchedule("every 1h", "")
	next = s.Next(now)
	expected = now.Add(time.Hour)
	if !next.Equal(expected) {
		t.Errorf("expected next at %v, got %v", expected, next)
	}
}

func TestScheduleNextCronComplex(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		now      time.Time
		expected time.Time
	}{
		{
			name:     "next minute",
			expr:     "* * * * *",
			now:      time.Date(2024, 2, 14, 8, 30, 0, 0, time.UTC),
			expected: time.Date(2024, 2, 14, 8, 31, 0, 0, time.UTC), // Next minute (5-field cron)
		},
		{
			name:     "specific minute",
			expr:     "30 * * * *",
			now:      time.Date(2024, 2, 14, 8, 0, 0, 0, time.UTC),
			expected: time.Date(2024, 2, 14, 8, 30, 0, 0, time.UTC),
		},
		{
			name:     "next hour",
			expr:     "0 * * * *",
			now:      time.Date(2024, 2, 14, 8, 30, 0, 0, time.UTC),
			expected: time.Date(2024, 2, 14, 9, 0, 0, 0, time.UTC),
		},
		{
			name:     "weekday skip",
			expr:     "0 9 * * 1", // Monday only
			now:      time.Date(2024, 2, 14, 10, 0, 0, 0, time.UTC), // Wednesday
			expected: time.Date(2024, 2, 19, 9, 0, 0, 0, time.UTC),  // Next Monday
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := ParseSchedule(tt.expr, "UTC")
			if err != nil {
				t.Fatalf("ParseSchedule error = %v", err)
			}
			next := s.Next(tt.now)
			if !next.Equal(tt.expected) {
				t.Errorf("expected next at %v, got %v", tt.expected, next)
			}
		})
	}
}

func TestScheduleIsOneShot(t *testing.T) {
	// Cron is not one-shot
	s, err := ParseSchedule("0 9 * * *", "")
	if err != nil {
		t.Fatalf("failed to parse cron: %v", err)
	}
	if s.IsOneShot() {
		t.Error("cron schedule should not be one-shot")
	}

	// Every is not one-shot
	s, err = ParseSchedule("every 5m", "")
	if err != nil {
		t.Fatalf("failed to parse every: %v", err)
	}
	if s.IsOneShot() {
		t.Error("every schedule should not be one-shot")
	}

	// At is one-shot (use future date)
	s, err = ParseSchedule("at 2030-12-31T23:59:59Z", "")
	if err != nil {
		t.Fatalf("failed to parse at: %v", err)
	}
	if !s.IsOneShot() {
		t.Error("at schedule should be one-shot")
	}
}

func TestParseField(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		min, max int
		expected []int
		wantErr  bool
	}{
		{"wildcard", "*", 0, 5, []int{0, 1, 2, 3, 4, 5}, false},
		{"single", "3", 0, 5, []int{3}, false},
		{"range", "1-3", 0, 5, []int{1, 2, 3}, false},
		{"step", "*/2", 0, 5, []int{0, 2, 4}, false},
		{"list", "1,3,5", 0, 5, []int{1, 3, 5}, false},
		{"complex", "1-3,5", 0, 5, []int{1, 2, 3, 5}, false},
		{"range step", "0-10/3", 0, 10, []int{0, 3, 6, 9}, false},
		{"out of range", "10", 0, 5, nil, true},
		{"invalid", "abc", 0, 5, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field, err := parseField(tt.expr, tt.min, tt.max)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseField(%q) error = %v, wantErr %v", tt.expr, err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if !equalSlices(field.Values, tt.expected) {
					t.Errorf("parseField(%q) = %v, want %v", tt.expr, field.Values, tt.expected)
				}
			}
		})
	}
}

func TestReplaceNames(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"JAN", "1"},
		{"DEC", "12"},
		{"SUN", "0"},
		{"SAT", "6"},
		{"JAN-MAR", "1-3"},
		{"MON-FRI", "1-5"},
		{"jan", "1"}, // case insensitive
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := replaceNames(tt.input)
			if result != tt.expected {
				t.Errorf("replaceNames(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsValidTimezone(t *testing.T) {
	tests := []struct {
		tz    string
		valid bool
	}{
		{"", true},
		{"UTC", true},
		{"Asia/Jakarta", true},
		{"America/New_York", true},
		{"Invalid/Zone", false},
		{"ABC", false},
	}

	for _, tt := range tests {
		t.Run(tt.tz, func(t *testing.T) {
			if IsValidTimezone(tt.tz) != tt.valid {
				t.Errorf("IsValidTimezone(%q) = %v, want %v", tt.tz, !tt.valid, tt.valid)
			}
		})
	}
}

func TestResolveTimezone(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"WIB", "Asia/Jakarta"},
		{"JST", "Asia/Tokyo"},
		{"UTC", "UTC"},
		{"America/New_York", "America/New_York"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ResolveTimezone(tt.input)
			if result != tt.expected {
				t.Errorf("ResolveTimezone(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{"5m", 5 * time.Minute, false},
		{"2h", 2 * time.Hour, false},
		{"30s", 30 * time.Second, false},
		{"1d", 24 * time.Hour, false},
		{"1w", 7 * 24 * time.Hour, false},
		{"1h30m", 90 * time.Minute, false},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("ParseDuration(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestScheduleString(t *testing.T) {
	tests := []struct {
		expr     string
		expected string
	}{
		{"every 5m", "every 5m0s"},
		{"@daily", "cron @daily"},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			s, err := ParseSchedule(tt.expr, "")
			if err != nil {
				t.Fatalf("ParseSchedule error = %v", err)
			}
			result := s.String()
			if result != tt.expected {
				t.Errorf("String() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestScheduleInfo(t *testing.T) {
	s, _ := ParseSchedule("every 1h", "")
	info := s.Info()

	if info.Type != ScheduleEvery {
		t.Errorf("expected type every, got %s", info.Type)
	}
	if info.NextRun == nil {
		t.Error("NextRun should be set")
	}
}

func equalSlices(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

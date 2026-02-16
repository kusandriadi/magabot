// Package cron provides enhanced scheduling with timezone support and multiple schedule types.
package cron

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ScheduleType identifies the type of schedule.
type ScheduleType string

const (
	// ScheduleCron is a standard cron expression (minute hour day month weekday)
	ScheduleCron ScheduleType = "cron"
	// ScheduleEvery is an interval-based schedule (e.g., "every 5m", "every 2h")
	ScheduleEvery ScheduleType = "every"
	// ScheduleAt is a one-shot schedule at a specific time
	ScheduleAt ScheduleType = "at"
)

// Schedule represents a parsed schedule configuration.
type Schedule struct {
	Type       ScheduleType `json:"type"`
	Expression string       `json:"expression"` // Original expression
	Timezone   string       `json:"timezone"`   // IANA timezone (e.g., "Asia/Jakarta")
	location   *time.Location

	// For cron schedules
	Minute     Field `json:"minute,omitempty"`
	Hour       Field `json:"hour,omitempty"`
	DayOfMonth Field `json:"day_of_month,omitempty"`
	Month      Field `json:"month,omitempty"`
	DayOfWeek  Field `json:"day_of_week,omitempty"`
	Second     Field `json:"second,omitempty"` // Optional 6-field cron

	// For "every" schedules
	Interval time.Duration `json:"interval,omitempty"`

	// For "at" schedules
	AtTime *time.Time `json:"at_time,omitempty"`
}

// Field represents a cron field with its allowed values.
type Field struct {
	Expression string `json:"expression"`
	Values     []int  `json:"-"` // Expanded values
}

// ParseSchedule parses a schedule expression with optional timezone.
// Formats:
//   - Cron: "0 9 * * 1-5" or "0 0 9 * * 1-5" (with seconds)
//   - Every: "every 5m", "every 2h30m", "every 1d"
//   - At: "at 2024-02-14T15:30:00", "at 15:30" (today)
//   - Predefined: "@hourly", "@daily", "@weekly", "@monthly"
func ParseSchedule(expr string, timezone string) (*Schedule, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("empty schedule expression")
	}

	// Load timezone
	var loc *time.Location
	var err error
	if timezone != "" {
		loc, err = time.LoadLocation(timezone)
		if err != nil {
			return nil, fmt.Errorf("invalid timezone %q: %w", timezone, err)
		}
	} else {
		loc = time.Local
	}

	schedule := &Schedule{
		Expression: expr,
		Timezone:   timezone,
		location:   loc,
	}

	// Check for predefined schedules
	if strings.HasPrefix(expr, "@") {
		return parsePredefined(expr, schedule)
	}

	// Check for "every" schedule
	if strings.HasPrefix(strings.ToLower(expr), "every ") {
		return parseEvery(expr, schedule)
	}

	// Check for "at" schedule
	if strings.HasPrefix(strings.ToLower(expr), "at ") {
		return parseAt(expr, schedule)
	}

	// Otherwise, parse as cron expression
	return parseCron(expr, schedule)
}

// parsePredefined handles @hourly, @daily, etc.
func parsePredefined(expr string, schedule *Schedule) (*Schedule, error) {
	schedule.Type = ScheduleCron

	switch strings.ToLower(expr) {
	case "@yearly", "@annually":
		schedule.Second = Field{Expression: "0", Values: []int{0}}
		schedule.Minute = Field{Expression: "0", Values: []int{0}}
		schedule.Hour = Field{Expression: "0", Values: []int{0}}
		schedule.DayOfMonth = Field{Expression: "1", Values: []int{1}}
		schedule.Month = Field{Expression: "1", Values: []int{1}}
		schedule.DayOfWeek = Field{Expression: "*", Values: expandRange(0, 6)}
	case "@monthly":
		schedule.Second = Field{Expression: "0", Values: []int{0}}
		schedule.Minute = Field{Expression: "0", Values: []int{0}}
		schedule.Hour = Field{Expression: "0", Values: []int{0}}
		schedule.DayOfMonth = Field{Expression: "1", Values: []int{1}}
		schedule.Month = Field{Expression: "*", Values: expandRange(1, 12)}
		schedule.DayOfWeek = Field{Expression: "*", Values: expandRange(0, 6)}
	case "@weekly":
		schedule.Second = Field{Expression: "0", Values: []int{0}}
		schedule.Minute = Field{Expression: "0", Values: []int{0}}
		schedule.Hour = Field{Expression: "0", Values: []int{0}}
		schedule.DayOfMonth = Field{Expression: "*", Values: expandRange(1, 31)}
		schedule.Month = Field{Expression: "*", Values: expandRange(1, 12)}
		schedule.DayOfWeek = Field{Expression: "0", Values: []int{0}} // Sunday
	case "@daily", "@midnight":
		schedule.Second = Field{Expression: "0", Values: []int{0}}
		schedule.Minute = Field{Expression: "0", Values: []int{0}}
		schedule.Hour = Field{Expression: "0", Values: []int{0}}
		schedule.DayOfMonth = Field{Expression: "*", Values: expandRange(1, 31)}
		schedule.Month = Field{Expression: "*", Values: expandRange(1, 12)}
		schedule.DayOfWeek = Field{Expression: "*", Values: expandRange(0, 6)}
	case "@hourly":
		schedule.Second = Field{Expression: "0", Values: []int{0}}
		schedule.Minute = Field{Expression: "0", Values: []int{0}}
		schedule.Hour = Field{Expression: "*", Values: expandRange(0, 23)}
		schedule.DayOfMonth = Field{Expression: "*", Values: expandRange(1, 31)}
		schedule.Month = Field{Expression: "*", Values: expandRange(1, 12)}
		schedule.DayOfWeek = Field{Expression: "*", Values: expandRange(0, 6)}
	default:
		return nil, fmt.Errorf("unknown predefined schedule: %s", expr)
	}

	return schedule, nil
}

// parseEvery handles "every 5m", "every 2h30m", etc.
func parseEvery(expr string, schedule *Schedule) (*Schedule, error) {
	schedule.Type = ScheduleEvery

	durationStr := strings.TrimPrefix(strings.ToLower(expr), "every ")
	durationStr = strings.TrimSpace(durationStr)

	// Handle common shortcuts
	switch durationStr {
	case "minute":
		durationStr = "1m"
	case "hour":
		durationStr = "1h"
	case "day":
		durationStr = "24h"
	}

	// Parse duration
	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		return nil, fmt.Errorf("invalid interval %q: %w", durationStr, err)
	}

	if duration < time.Second {
		return nil, fmt.Errorf("interval must be at least 1 second")
	}

	schedule.Interval = duration
	return schedule, nil
}

// parseAt handles "at 2024-02-14T15:30:00" or "at 15:30"
func parseAt(expr string, schedule *Schedule) (*Schedule, error) {
	schedule.Type = ScheduleAt

	// Remove "at " prefix case-insensitively but preserve time string case
	lowerExpr := strings.ToLower(expr)
	var timeStr string
	if strings.HasPrefix(lowerExpr, "at ") {
		timeStr = strings.TrimSpace(expr[3:])
	} else {
		timeStr = strings.TrimSpace(expr)
	}

	var t time.Time
	var err error

	// Try various formats
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04",
		"2006-01-02",
		"15:04:05",
		"15:04",
	}

	for _, format := range formats {
		t, err = time.ParseInLocation(format, timeStr, schedule.location)
		if err == nil {
			break
		}
	}

	if err != nil {
		return nil, fmt.Errorf("invalid time %q: %w", timeStr, err)
	}

	// If only time was given (no date), assume today or tomorrow
	now := time.Now().In(schedule.location)
	if t.Year() == 0 {
		t = time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), t.Second(), 0, schedule.location)
		// If the time has already passed today, schedule for tomorrow
		if t.Before(now) {
			t = t.Add(24 * time.Hour)
		}
	}

	schedule.AtTime = &t
	return schedule, nil
}

// parseCron handles standard cron expressions.
func parseCron(expr string, schedule *Schedule) (*Schedule, error) {
	schedule.Type = ScheduleCron

	fields := strings.Fields(expr)
	if len(fields) < 5 || len(fields) > 6 {
		return nil, fmt.Errorf("cron expression must have 5 or 6 fields, got %d", len(fields))
	}

	var offset int
	if len(fields) == 6 {
		// 6-field: second minute hour day month weekday
		second, err := parseField(fields[0], 0, 59)
		if err != nil {
			return nil, fmt.Errorf("invalid second field: %w", err)
		}
		schedule.Second = second
		offset = 1
	} else {
		// 5-field: minute hour day month weekday (second = 0)
		schedule.Second = Field{Expression: "0", Values: []int{0}}
	}

	minute, err := parseField(fields[offset], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("invalid minute field: %w", err)
	}
	schedule.Minute = minute

	hour, err := parseField(fields[offset+1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("invalid hour field: %w", err)
	}
	schedule.Hour = hour

	dom, err := parseField(fields[offset+2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("invalid day-of-month field: %w", err)
	}
	schedule.DayOfMonth = dom

	month, err := parseField(fields[offset+3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("invalid month field: %w", err)
	}
	schedule.Month = month

	dow, err := parseField(fields[offset+4], 0, 6)
	if err != nil {
		return nil, fmt.Errorf("invalid day-of-week field: %w", err)
	}
	schedule.DayOfWeek = dow

	return schedule, nil
}

// parseField parses a single cron field.
func parseField(expr string, min, max int) (Field, error) {
	field := Field{Expression: expr}

	// Handle names (e.g., JAN-DEC, SUN-SAT)
	expr = replaceNames(expr)

	// Split by comma
	parts := strings.Split(expr, ",")
	var values []int

	for _, part := range parts {
		part = strings.TrimSpace(part)

		// Check for step (e.g., */5, 1-10/2)
		stepParts := strings.SplitN(part, "/", 2)
		rangePart := stepParts[0]
		step := 1
		if len(stepParts) == 2 {
			var err error
			step, err = strconv.Atoi(stepParts[1])
			if err != nil || step <= 0 {
				return field, fmt.Errorf("invalid step: %s", stepParts[1])
			}
		}

		// Handle wildcard
		if rangePart == "*" {
			for i := min; i <= max; i += step {
				values = append(values, i)
			}
			continue
		}

		// Handle range (e.g., 1-5)
		if strings.Contains(rangePart, "-") {
			rangeParts := strings.SplitN(rangePart, "-", 2)
			start, err := strconv.Atoi(rangeParts[0])
			if err != nil {
				return field, fmt.Errorf("invalid range start: %s", rangeParts[0])
			}
			end, err := strconv.Atoi(rangeParts[1])
			if err != nil {
				return field, fmt.Errorf("invalid range end: %s", rangeParts[1])
			}
			if start < min || end > max || start > end {
				return field, fmt.Errorf("invalid range: %d-%d (valid: %d-%d)", start, end, min, max)
			}
			for i := start; i <= end; i += step {
				values = append(values, i)
			}
			continue
		}

		// Single value
		val, err := strconv.Atoi(rangePart)
		if err != nil {
			return field, fmt.Errorf("invalid value: %s", rangePart)
		}
		if val < min || val > max {
			return field, fmt.Errorf("value out of range: %d (valid: %d-%d)", val, min, max)
		}
		values = append(values, val)
	}

	field.Values = unique(values)
	return field, nil
}

// replaceNames replaces month/weekday names with numbers.
func replaceNames(expr string) string {
	expr = strings.ToUpper(expr)

	months := map[string]string{
		"JAN": "1", "FEB": "2", "MAR": "3", "APR": "4",
		"MAY": "5", "JUN": "6", "JUL": "7", "AUG": "8",
		"SEP": "9", "OCT": "10", "NOV": "11", "DEC": "12",
	}

	days := map[string]string{
		"SUN": "0", "MON": "1", "TUE": "2", "WED": "3",
		"THU": "4", "FRI": "5", "SAT": "6",
	}

	for name, num := range months {
		expr = strings.ReplaceAll(expr, name, num)
	}
	for name, num := range days {
		expr = strings.ReplaceAll(expr, name, num)
	}

	return expr
}

// expandRange generates all integers in a range.
func expandRange(min, max int) []int {
	result := make([]int, 0, max-min+1)
	for i := min; i <= max; i++ {
		result = append(result, i)
	}
	return result
}

// unique removes duplicates and sorts values.
func unique(values []int) []int {
	seen := make(map[int]bool)
	result := make([]int, 0, len(values))
	for _, v := range values {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	// Simple sort
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j] < result[i] {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}

// contains checks if a slice contains a value.
func containsInt(slice []int, val int) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

// Next returns the next time the schedule should run after the given time.
func (s *Schedule) Next(after time.Time) time.Time {
	after = after.In(s.location)

	switch s.Type {
	case ScheduleEvery:
		return after.Add(s.Interval)

	case ScheduleAt:
		if s.AtTime != nil && s.AtTime.After(after) {
			return *s.AtTime
		}
		return time.Time{} // No more runs

	case ScheduleCron:
		return s.nextCron(after)
	}

	return time.Time{}
}

// nextCron calculates the next cron run time.
func (s *Schedule) nextCron(after time.Time) time.Time {
	// Start from the next second
	t := after.Add(time.Second)
	t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, s.location)

	// Try up to 4 years (to handle complex expressions)
	maxTime := after.Add(4 * 365 * 24 * time.Hour)

	for t.Before(maxTime) {
		// Check month
		if !containsInt(s.Month.Values, int(t.Month())) {
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, s.location)
			continue
		}

		// Check day of month and day of week
		domMatch := containsInt(s.DayOfMonth.Values, t.Day())
		dowMatch := containsInt(s.DayOfWeek.Values, int(t.Weekday()))

		// Standard cron: if both specified, either can match
		// If only one specified (other is *), only that one must match
		dayMatch := false
		domIsWild := len(s.DayOfMonth.Values) == 31
		dowIsWild := len(s.DayOfWeek.Values) == 7

		if domIsWild && dowIsWild {
			dayMatch = true
		} else if domIsWild {
			dayMatch = dowMatch
		} else if dowIsWild {
			dayMatch = domMatch
		} else {
			dayMatch = domMatch || dowMatch
		}

		if !dayMatch {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, s.location)
			continue
		}

		// Check hour
		if !containsInt(s.Hour.Values, t.Hour()) {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, s.location)
			continue
		}

		// Check minute
		if !containsInt(s.Minute.Values, t.Minute()) {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute()+1, 0, 0, s.location)
			continue
		}

		// Check second
		if !containsInt(s.Second.Values, t.Second()) {
			t = t.Add(time.Second)
			continue
		}

		return t
	}

	return time.Time{}
}

// IsOneShot returns true if this schedule only runs once.
func (s *Schedule) IsOneShot() bool {
	return s.Type == ScheduleAt
}

// String returns a human-readable description of the schedule.
func (s *Schedule) String() string {
	switch s.Type {
	case ScheduleEvery:
		return fmt.Sprintf("every %s", s.Interval)
	case ScheduleAt:
		if s.AtTime != nil {
			return fmt.Sprintf("at %s", s.AtTime.Format(time.RFC3339))
		}
		return "at (expired)"
	case ScheduleCron:
		if s.Timezone != "" {
			return fmt.Sprintf("cron %s (%s)", s.Expression, s.Timezone)
		}
		return fmt.Sprintf("cron %s", s.Expression)
	}
	return s.Expression
}

// Validate checks if the schedule is valid.
func (s *Schedule) Validate() error {
	switch s.Type {
	case ScheduleEvery:
		if s.Interval < time.Second {
			return fmt.Errorf("interval must be at least 1 second")
		}
	case ScheduleAt:
		if s.AtTime == nil {
			return fmt.Errorf("at time not set")
		}
	case ScheduleCron:
		if len(s.Minute.Values) == 0 {
			return fmt.Errorf("minute field has no values")
		}
		if len(s.Hour.Values) == 0 {
			return fmt.Errorf("hour field has no values")
		}
	}
	return nil
}

// ValidateCronExpression validates a cron expression without creating a full Schedule.
func ValidateCronExpression(expr string) error {
	_, err := ParseSchedule(expr, "")
	return err
}

// IsValidTimezone checks if a timezone string is valid.
func IsValidTimezone(tz string) bool {
	if tz == "" {
		return true
	}
	_, err := time.LoadLocation(tz)
	return err == nil
}

// Common timezone aliases.
var timezoneAliases = map[string]string{
	"WIB":  "Asia/Jakarta",
	"WITA": "Asia/Makassar",
	"WIT":  "Asia/Jayapura",
	"JST":  "Asia/Tokyo",
	"KST":  "Asia/Seoul",
	"SGT":  "Asia/Singapore",
	"EST":  "America/New_York",
	"PST":  "America/Los_Angeles",
	"CST":  "America/Chicago",
	"MST":  "America/Denver",
	"UTC":  "UTC",
	"GMT":  "UTC",
}

// ResolveTimezone resolves timezone aliases to IANA names.
func ResolveTimezone(tz string) string {
	if alias, ok := timezoneAliases[strings.ToUpper(tz)]; ok {
		return alias
	}
	return tz
}

// ScheduleInfo provides information about a schedule for display.
type ScheduleInfo struct {
	Type        ScheduleType `json:"type"`
	Expression  string       `json:"expression"`
	Timezone    string       `json:"timezone,omitempty"`
	NextRun     *time.Time   `json:"next_run,omitempty"`
	Description string       `json:"description"`
}

// Info returns display information about the schedule.
func (s *Schedule) Info() ScheduleInfo {
	info := ScheduleInfo{
		Type:        s.Type,
		Expression:  s.Expression,
		Timezone:    s.Timezone,
		Description: s.String(),
	}

	next := s.Next(time.Now())
	if !next.IsZero() {
		info.NextRun = &next
	}

	return info
}

// durationRegex matches duration strings like "5m", "2h30m", "1d"
var durationRegex = regexp.MustCompile(`^(\d+)(s|m|h|d|w)$`)

// ParseDuration parses a duration string with support for days and weeks.
func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ToLower(s))

	// Try standard format first
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}

	// Handle custom formats (days, weeks)
	if matches := durationRegex.FindStringSubmatch(s); matches != nil {
		val, _ := strconv.Atoi(matches[1])
		unit := matches[2]

		switch unit {
		case "s":
			return time.Duration(val) * time.Second, nil
		case "m":
			return time.Duration(val) * time.Minute, nil
		case "h":
			return time.Duration(val) * time.Hour, nil
		case "d":
			return time.Duration(val) * 24 * time.Hour, nil
		case "w":
			return time.Duration(val) * 7 * 24 * time.Hour, nil
		}
	}

	return 0, fmt.Errorf("invalid duration: %s", s)
}

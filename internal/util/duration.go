package util

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration is a config duration that must be specified as a duration string
// (e.g. "30s", "5m", "6h", "1h30m"). Plain integer values are not accepted.
// Use 0 or omit the field to disable (zero value = disabled).
type Duration struct {
	d time.Duration
}

// NewDuration creates a Duration from a time.Duration (for use in defaults).
func NewDuration(d time.Duration) Duration {
	return Duration{d: d}
}

// UnmarshalYAML implements yaml.Unmarshaler. Only duration strings are accepted.
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	if value.Tag == "!!int" || value.Tag == "!!float" {
		return fmt.Errorf("invalid duration %q: plain numbers are not accepted — use a duration string like \"30s\", \"5m\", \"6h\"", value.Value)
	}
	parsed, err := time.ParseDuration(value.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: use a duration string like \"30s\", \"5m\", \"6h\"", value.Value)
	}
	d.d = parsed
	return nil
}

// MarshalYAML implements yaml.Marshaler, emitting the canonical Go duration string.
func (d Duration) MarshalYAML() (interface{}, error) {
	return d.d.String(), nil
}

// Duration returns the underlying time.Duration.
func (d Duration) Duration() time.Duration { return d.d }

// Seconds returns the duration in whole seconds.
func (d Duration) Seconds() int { return int(d.d.Seconds()) }

// IsZero reports whether the duration is zero (disabled).
func (d Duration) IsZero() bool { return d.d == 0 }

// String returns the canonical Go duration string.
func (d Duration) String() string { return d.d.String() }

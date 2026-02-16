package version_test

import (
	"runtime"
	"strings"
	"testing"

	"github.com/kusa/magabot/internal/version"
)

func TestInfo(t *testing.T) {
	info := version.Info()

	expected := []string{"magabot", "dev", "unknown"}
	for _, s := range expected {
		if !strings.Contains(info, s) {
			t.Errorf("Info() = %q, expected it to contain %q", info, s)
		}
	}

	// Should also contain the Go version.
	if !strings.Contains(info, runtime.Version()) {
		t.Errorf("Info() = %q, expected it to contain Go version %q", info, runtime.Version())
	}
}

func TestInfo_Format(t *testing.T) {
	info := version.Info()

	// Verify the format: "magabot <version> (commit: <commit>, built: <time>, <go>)"
	if !strings.HasPrefix(info, "magabot ") {
		t.Errorf("Info() = %q, expected it to start with 'magabot '", info)
	}
	if !strings.Contains(info, "commit:") {
		t.Errorf("Info() = %q, expected it to contain 'commit:'", info)
	}
	if !strings.Contains(info, "built:") {
		t.Errorf("Info() = %q, expected it to contain 'built:'", info)
	}
}

func TestShort(t *testing.T) {
	short := version.Short()
	if short != version.Version {
		t.Errorf("Short() = %q, want %q (Version)", short, version.Version)
	}
}

func TestShort_DefaultValue(t *testing.T) {
	// With default ldflags, Version should be "dev".
	if version.Short() != "dev" {
		t.Errorf("Short() = %q, want 'dev'", version.Short())
	}
}

func TestFull(t *testing.T) {
	full := version.Full()

	expectedKeys := []string{
		"version",
		"git_commit",
		"build_time",
		"go_version",
		"os",
		"arch",
	}

	for _, key := range expectedKeys {
		if _, ok := full[key]; !ok {
			t.Errorf("Full() missing key %q", key)
		}
	}

	if len(full) != len(expectedKeys) {
		t.Errorf("Full() has %d keys, expected %d", len(full), len(expectedKeys))
	}
}

func TestFull_Values(t *testing.T) {
	full := version.Full()

	tests := []struct {
		key      string
		expected string
	}{
		{"version", version.Version},
		{"git_commit", version.GitCommit},
		{"build_time", version.BuildTime},
		{"go_version", runtime.Version()},
		{"os", runtime.GOOS},
		{"arch", runtime.GOARCH},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			got := full[tc.key]
			if got != tc.expected {
				t.Errorf("Full()[%q] = %q, want %q", tc.key, got, tc.expected)
			}
		})
	}
}

func TestFull_DefaultValues(t *testing.T) {
	full := version.Full()

	if full["version"] != "dev" {
		t.Errorf("expected default version 'dev', got %q", full["version"])
	}
	if full["git_commit"] != "unknown" {
		t.Errorf("expected default git_commit 'unknown', got %q", full["git_commit"])
	}
	if full["build_time"] != "unknown" {
		t.Errorf("expected default build_time 'unknown', got %q", full["build_time"])
	}
}

func TestInfo_OutputFormat(t *testing.T) {
	info := version.Info()

	// Verify the overall format: "magabot <ver> (commit: <x>, built: <x>, <go>)"
	if !strings.HasPrefix(info, "magabot ") {
		t.Errorf("Info() should start with 'magabot ', got %q", info)
	}
	if !strings.Contains(info, "(") || !strings.HasSuffix(info, ")") {
		t.Errorf("Info() should have parenthesized section, got %q", info)
	}
	if !strings.Contains(info, "commit:") {
		t.Errorf("Info() missing 'commit:', got %q", info)
	}
	if !strings.Contains(info, "built:") {
		t.Errorf("Info() missing 'built:', got %q", info)
	}
}

func TestBuildInfo_FieldsPopulated(t *testing.T) {
	// The exported vars should have their default values when not set via ldflags
	if version.Version == "" {
		t.Error("Version should not be empty")
	}
	if version.GitCommit == "" {
		t.Error("GitCommit should not be empty")
	}
	if version.BuildTime == "" {
		t.Error("BuildTime should not be empty")
	}
	if version.GoVersion == "" {
		t.Error("GoVersion should not be empty")
	}
	if !strings.HasPrefix(version.GoVersion, "go") {
		t.Errorf("GoVersion should start with 'go', got %q", version.GoVersion)
	}
}

package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIsAudioFile(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"voice.ogg", true},
		{"audio.oga", true},
		{"music.mp3", true},
		{"clip.m4a", true},
		{"record.wav", true},
		{"VOICE.OGG", true}, // case-insensitive
		{"photo.jpg", false},
		{"doc.pdf", false},
		{"video.mp4", false},
		{"file", false},
		{"", false},
	}
	for _, c := range cases {
		got := isAudioFile(c.path)
		if got != c.want {
			t.Errorf("isAudioFile(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestCleanOldDownloads(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Write an old file (2 days ago)
	old := filepath.Join(dir, "old.ogg")
	if err := os.WriteFile(old, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-48 * time.Hour)
	_ = os.Chtimes(old, oldTime, oldTime)

	// Write a recent file
	recent := filepath.Join(dir, "recent.ogg")
	if err := os.WriteFile(recent, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}

	cleanOldDownloads([]string{dir}, 24*time.Hour, logger)

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("expected old file to be deleted")
	}
	if _, err := os.Stat(recent); err != nil {
		t.Error("expected recent file to be kept")
	}
}

func TestCleanOldDownloads_NonExistentDir(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	// Should not panic on missing directory
	cleanOldDownloads([]string{"/nonexistent/path"}, 24*time.Hour, logger)
}

func TestLocalScriptPath(t *testing.T) {
	home, _ := os.UserHomeDir()
	got := localScriptPath("tts-speak")
	want := filepath.Join(home, ".local", "bin", "tts-speak")
	if got != want {
		t.Errorf("localScriptPath = %q, want %q", got, want)
	}
}

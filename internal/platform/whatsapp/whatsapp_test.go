package whatsapp

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSaveVoice(t *testing.T) {
	dir := t.TempDir()
	b := &Bot{downloadsDir: dir}

	data := []byte("fake ogg audio data")
	path, err := b.saveVoice(data)
	if err != nil {
		t.Fatalf("saveVoice failed: %v", err)
	}

	// File should be inside downloadsDir
	if !strings.HasPrefix(path, dir) {
		t.Errorf("path %q not inside downloadsDir %q", path, dir)
	}

	// File should exist and be readable
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("file content mismatch")
	}

	// Permissions should be 0600 (skip on Windows — permission bits aren't meaningful)
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Errorf("file permissions = %o, want 0600", info.Mode().Perm())
		}
	}

	// Filename should have .ogg extension
	if filepath.Ext(path) != ".ogg" {
		t.Errorf("expected .ogg extension, got %q", filepath.Ext(path))
	}
}

func TestSaveVoice_CreatesDir(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "subdir", "downloads")
	b := &Bot{downloadsDir: dir}

	_, err := b.saveVoice([]byte("data"))
	if err != nil {
		t.Fatalf("saveVoice failed: %v", err)
	}

	if _, err := os.Stat(dir); err != nil {
		t.Error("expected downloadsDir to be created")
	}
}

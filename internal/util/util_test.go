package util

import (
	"testing"
	"time"
)

func TestNewHTTPClient(t *testing.T) {
	t.Run("DefaultTimeout", func(t *testing.T) {
		client := NewHTTPClient(0)
		if client.Timeout != DefaultHTTPTimeout {
			t.Errorf("Expected timeout %v, got %v", DefaultHTTPTimeout, client.Timeout)
		}
	})

	t.Run("CustomTimeout", func(t *testing.T) {
		client := NewHTTPClient(10 * time.Second)
		if client.Timeout != 10*time.Second {
			t.Errorf("Expected timeout 10s, got %v", client.Timeout)
		}
	})
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		max      int
		expected string
	}{
		{"hello world", 5, "he..."},
		{"hi", 5, "hi"},
		{"hello", 3, "hello"}, // max <= 3 returns original
		{"", 5, ""},
	}

	for _, tt := range tests {
		result := Truncate(tt.input, tt.max)
		if result != tt.expected {
			t.Errorf("Truncate(%q, %d) = %q, want %q", tt.input, tt.max, result, tt.expected)
		}
	}
}

func TestTruncateRunes(t *testing.T) {
	tests := []struct {
		input    string
		max      int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world", 8, "hello..."},
		{"こんにちは", 3, "こんに"}, // max <= 3 returns first max runes
		{"日本語テスト", 5, "日本..."},
	}

	for _, tt := range tests {
		result := TruncateRunes(tt.input, tt.max)
		if result != tt.expected {
			t.Errorf("TruncateRunes(%q, %d) = %q, want %q", tt.input, tt.max, result, tt.expected)
		}
	}
}

func TestRandomID(t *testing.T) {
	t.Run("CorrectLength", func(t *testing.T) {
		id := RandomID(16)
		if len(id) != 16 {
			t.Errorf("Expected length 16, got %d", len(id))
		}
	})

	t.Run("UniqueIDs", func(t *testing.T) {
		ids := make(map[string]bool)
		for i := 0; i < 100; i++ {
			id := RandomID(32)
			if ids[id] {
				t.Error("Generated duplicate ID")
			}
			ids[id] = true
		}
	})
}

func TestRandomToken(t *testing.T) {
	t.Run("CorrectLength", func(t *testing.T) {
		token, err := RandomToken(16)
		if err != nil {
			t.Fatalf("RandomToken failed: %v", err)
		}
		// hex encoding doubles the length
		if len(token) != 32 {
			t.Errorf("Expected length 32, got %d", len(token))
		}
	})
}

func TestSanitizeInput(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello\x00world", "helloworld"},
		{"hello\nworld", "hello\nworld"}, // newline preserved
		{"hello\tworld", "hello\tworld"}, // tab preserved
		{"normal text", "normal text"},
		{"hello\x01\x02world", "helloworld"}, // control chars removed
	}

	for _, tt := range tests {
		result := SanitizeInput(tt.input)
		if result != tt.expected {
			t.Errorf("SanitizeInput(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"file.txt", "file.txt"},
		{"../etc/passwd", ".._etc_passwd"},
		{"C:\\Windows\\file", "C__Windows_file"},
		{"file\x00name", "file_name"},
	}

	for _, tt := range tests {
		result := SanitizeFilename(tt.input)
		if result != tt.expected {
			t.Errorf("SanitizeFilename(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestContains(t *testing.T) {
	slice := []string{"a", "b", "c"}
	if !Contains(slice, "b") {
		t.Error("Should contain 'b'")
	}
	if Contains(slice, "d") {
		t.Error("Should not contain 'd'")
	}
}

func TestRemove(t *testing.T) {
	slice := []string{"a", "b", "c"}
	result := Remove(slice, "b")
	if len(result) != 2 {
		t.Errorf("Expected 2 elements, got %d", len(result))
	}
	if Contains(result, "b") {
		t.Error("Should not contain 'b' after removal")
	}
}

func TestAddUnique(t *testing.T) {
	slice := []string{"a", "b"}

	// Add new
	result := AddUnique(slice, "c")
	if len(result) != 3 {
		t.Errorf("Expected 3 elements, got %d", len(result))
	}

	// Add existing (no change)
	result = AddUnique(result, "a")
	if len(result) != 3 {
		t.Errorf("Expected 3 elements (no dup), got %d", len(result))
	}
}

func TestMaskSecret(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"short", "****"},
		{"12345678", "****"},
		{"1234567890", "1234...7890"},
		{"sk-ant-api03-xxx-xxx-xxx", "sk-a...-xxx"},
	}

	for _, tt := range tests {
		result := MaskSecret(tt.input)
		if result != tt.expected {
			t.Errorf("MaskSecret(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestIsValidID(t *testing.T) {
	valid := []string{"user", "user_123", "_private", "User-Name"}
	invalid := []string{"", "123start", "has space", "too@special",
		"verylongidthatexceedsixtyfourcharacterslimitwhichisquitealotofchars"}

	for _, id := range valid {
		if !IsValidID(id) {
			t.Errorf("'%s' should be valid", id)
		}
	}

	for _, id := range invalid {
		if IsValidID(id) {
			t.Errorf("'%s' should be invalid", id)
		}
	}
}

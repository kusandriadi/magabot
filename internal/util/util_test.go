package util

import (
	"strings"
	"testing"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		max      int
		expected string
	}{
		{"short string", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"needs truncate", "hello world", 8, "hello..."},
		{"very short max", "hello", 3, "hello"},
		{"empty string", "", 10, ""},
		{"unicode short", "hello", 10, "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Truncate(tt.input, tt.max)
			if result != tt.expected {
				t.Errorf("Truncate(%q, %d) = %q, want %q", tt.input, tt.max, result, tt.expected)
			}
		})
	}
}

func TestSanitizeInput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"normal text", "hello world", "hello world"},
		{"with newline", "hello\nworld", "hello\nworld"},
		{"with tab", "hello\tworld", "hello\tworld"},
		{"with null byte", "hello\x00world", "helloworld"},
		{"with control chars", "hello\x01\x02world", "helloworld"},
		{"mixed", "hello\x00\nworld\x01", "hello\nworld"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeInput(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeInput(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"normal", "file.txt", "file.txt"},
		{"with slash", "path/file.txt", "path_file.txt"},
		{"with backslash", "path\\file.txt", "path_file.txt"},
		{"with colon", "C:file.txt", "C_file.txt"},
		{"with null", "file\x00.txt", "file_.txt"},
		{"multiple unsafe", "../../../etc/passwd", ".._.._.._etc_passwd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeFilename(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeFilename(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestContains(t *testing.T) {
	slice := []string{"apple", "banana", "cherry"}

	if !Contains(slice, "banana") {
		t.Error("Contains should find 'banana'")
	}

	if Contains(slice, "grape") {
		t.Error("Contains should not find 'grape'")
	}

	if Contains(nil, "apple") {
		t.Error("Contains should handle nil slice")
	}
}

func TestRemove(t *testing.T) {
	slice := []string{"apple", "banana", "cherry"}

	result := Remove(slice, "banana")
	if len(result) != 2 {
		t.Errorf("Remove should return 2 elements, got %d", len(result))
	}
	if Contains(result, "banana") {
		t.Error("Remove should have removed 'banana'")
	}

	// Remove non-existent
	result2 := Remove(slice, "grape")
	if len(result2) != 3 {
		t.Error("Remove of non-existent should not change length")
	}
}

func TestAddUnique(t *testing.T) {
	slice := []string{"apple", "banana"}

	// Add new item
	result := AddUnique(slice, "cherry")
	if len(result) != 3 {
		t.Errorf("AddUnique should add new item, got len %d", len(result))
	}

	// Add existing item
	result2 := AddUnique(slice, "apple")
	if len(result2) != 2 {
		t.Errorf("AddUnique should not add duplicate, got len %d", len(result2))
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
		{"sk-abcdefghijklmnop", "sk-a...mnop"},
	}

	for _, tt := range tests {
		result := MaskSecret(tt.input)
		if result != tt.expected {
			t.Errorf("MaskSecret(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestIsValidID(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"valid_id", true},
		{"ValidID123", true},
		{"_underscore", true},
		{"with-dash", true},
		{"123starts", false},
		{"has space", false},
		{"has.dot", false},
		{"", false},
		{strings.Repeat("a", 65), false},
	}

	for _, tt := range tests {
		result := IsValidID(tt.input)
		if result != tt.expected {
			t.Errorf("IsValidID(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func TestRandomID(t *testing.T) {
	id1 := RandomID(16)
	id2 := RandomID(16)

	if len(id1) != 16 {
		t.Errorf("RandomID(16) should return 16 chars, got %d", len(id1))
	}

	if id1 == id2 {
		t.Error("RandomID should generate unique IDs")
	}
}

func TestRandomToken(t *testing.T) {
	token, err := RandomToken(32)
	if err != nil {
		t.Errorf("RandomToken failed: %v", err)
	}

	if len(token) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("RandomToken(32) should return 64 hex chars, got %d", len(token))
	}
}

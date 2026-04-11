package util

import (
	"strings"
	"testing"
)

func TestIsInteger(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"0", true},
		{"1", true},
		{"-1", true},
		{"123", true},
		{"abc", false},
		{"", false},
		{"1.5", false},
		{"12ab", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := IsInteger(tt.input)
			if result != tt.expected {
				t.Errorf("IsInteger(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestRandString(t *testing.T) {
	t.Run("correct length", func(t *testing.T) {
		for _, length := range []int{1, 5, 10, 32, 64} {
			result := RandString(length, LETTER)
			if len(result) != length {
				t.Errorf("RandString(%d, LETTER) length = %d, want %d", length, len(result), length)
			}
		}
	})

	t.Run("uses source characters only", func(t *testing.T) {
		source := "abc"
		result := RandString(100, source)
		for _, c := range result {
			if !strings.ContainsRune(source, c) {
				t.Errorf("RandString(100, %q) contains unexpected char %q", source, c)
			}
		}
	})

	t.Run("randomness check", func(t *testing.T) {
		result1 := RandString(32, ALL)
		result2 := RandString(32, ALL)
		if result1 == result2 {
			t.Logf("Warning: two random strings of length 32 are equal (unlikely but possible)")
		}
	})

	t.Run("digits only", func(t *testing.T) {
		result := RandString(20, DIGITS)
		for _, c := range result {
			if c < '0' || c > '9' {
				t.Errorf("RandString(20, DIGITS) contains non-digit: %q", c)
			}
		}
	})
}

func TestVerifyEmailFormat(t *testing.T) {
	tests := []struct {
		email    string
		expected bool
	}{
		{"test@example.com", true},
		{"user123@test.org", true},
		{"a@bb.cc", true},
		{"", false},
		{"@example.com", false},
		{"test@", false},
		{"test@.com", false},
		{"te st@example.com", false},
		{"a@b.cc", false}, // 域名部分太短
	}
	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			result := VerifyEmailFormat(tt.email)
			if result != tt.expected {
				t.Errorf("VerifyEmailFormat(%q) = %v, want %v", tt.email, result, tt.expected)
			}
		})
	}
}

func TestColorFunctions(t *testing.T) {
	tests := []struct {
		name     string
		fn       func(string) string
		colorSeq string
	}{
		{"Red", Red, RED},
		{"Green", Green, GREEN},
		{"Yellow", Yellow, YELLOW},
		{"Blue", Blue, BLUE},
		{"Fuchsia", Fuchsia, FUCHSIA},
		{"Cyan", Cyan, CYAN},
		{"White", White, WHITE},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.fn("hello")
			expected := tt.colorSeq + "hello" + RESET
			if result != expected {
				t.Errorf("%s(\"hello\") = %q, want %q", tt.name, result, expected)
			}
		})
	}
}

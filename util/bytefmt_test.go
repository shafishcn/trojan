package util

import (
	"testing"
)

func TestBytefmt(t *testing.T) {
	tests := []struct {
		name     string
		input    uint64
		expected string
	}{
		{"zero bytes", 0, "0B"},
		{"one byte", 1, "1B"},
		{"bytes below KB", 500, "500B"},
		{"exactly 1 KB", 1024, "1K"},
		{"1.5 KB", 1536, "1.5K"},
		{"exactly 1 MB", 1024 * 1024, "1M"},
		{"1.25 MB", 1024*1024 + 256*1024, "1.25M"},
		{"exactly 1 GB", 1024 * 1024 * 1024, "1G"},
		{"2.5 GB", 2*1024*1024*1024 + 512*1024*1024, "2.5G"},
		{"exactly 1 TB", 1024 * 1024 * 1024 * 1024, "1T"},
		{"large value 10 TB", 10 * 1024 * 1024 * 1024 * 1024, "10T"},
		{"1023 bytes", 1023, "1023B"},
		{"1025 bytes", 1025, "1K"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Bytefmt(tt.input)
			if result != tt.expected {
				t.Errorf("Bytefmt(%d) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestBytefmt_NoTrailingZeros(t *testing.T) {
	// 确保常见值格式化后没有尾部多余的零
	exactKB := Bytefmt(1024)
	if exactKB != "1K" {
		t.Errorf("Bytefmt(1024) = %q, want \"1K\"", exactKB)
	}
	exactMB := Bytefmt(1024 * 1024)
	if exactMB != "1M" {
		t.Errorf("Bytefmt(1048576) = %q, want \"1M\"", exactMB)
	}
	exactGB := Bytefmt(1024 * 1024 * 1024)
	if exactGB != "1G" {
		t.Errorf("Bytefmt(1073741824) = %q, want \"1G\"", exactGB)
	}
}

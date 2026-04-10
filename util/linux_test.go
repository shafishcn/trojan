package util

import (
	"reflect"
	"testing"
)

func TestJournalctlArgs(t *testing.T) {
	t.Run("tail fixed lines", func(t *testing.T) {
		got := journalctlArgs("trojan", 300)
		want := []string{"-f", "-u", "trojan", "-o", "cat", "-n", "300"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("journalctlArgs() = %v, want %v", got, want)
		}
	})

	t.Run("disable tail", func(t *testing.T) {
		got := journalctlArgs("trojan", -1)
		want := []string{"-f", "-u", "trojan", "-o", "cat", "--no-tail"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("journalctlArgs() = %v, want %v", got, want)
		}
	})
}

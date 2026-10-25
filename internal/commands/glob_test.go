package commands

import (
	"strings"
	"testing"
)

func TestGlobMatch(t *testing.T) {
	tests := []struct {
		pattern, s string
		want       bool
	}{
		{"*", "", true},
		{"*", "anything", true},
		{"h?llo", "hello", true},
		{"h?llo", "hallo", true},
		{"h?llo", "hllo", false},
		{"h*llo", "hllo", true},
		{"h*llo", "heeeello", true},
		{"h*llo", "hello world", false},
		{"h[ae]llo", "hello", true},
		{"h[ae]llo", "hallo", true},
		{"h[ae]llo", "hillo", false},
		{"h[^e]llo", "hallo", true},
		{"h[^e]llo", "hello", false},
		{"h[a-b]llo", "hbllo", true},
		{"h[a-b]llo", "hcllo", false},
		{"h[b-a]llo", "hallo", true}, // reversed range, as Redis allows
		{`h\*llo`, "h*llo", true},
		{`h\*llo`, "hello", false},
		{`h[\]]llo`, "h]llo", true},
		{"user:*", "user:1", true},
		{"user:*", "users:1", false},
		{"*:*:*", "a:b:c", true},
		{"*:*:*", "a:b", false},
		{"a*b*c", "aXXbYYc", true},
		{"a*b*c", "aXXbYY", false},
		{"", "", true},
		{"", "a", false},
		{"abc", "abc", true},
		{"abc", "abcd", false},
		{"[abc", "b", true}, // unterminated class runs to the end
		{"?", "", false},
		{"**", "x", true},
		{`\`, `\`, true},
		{"k\xff*", "k\xffz", true}, // bytes, not runes
	}
	for _, tc := range tests {
		if got := globMatch(tc.pattern, tc.s); got != tc.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", tc.pattern, tc.s, got, tc.want)
		}
	}
}

func TestGlobMatchNoBlowUp(t *testing.T) {
	// Patterns like a*a*a*...b against aaaa... take exponential time with
	// naive recursion. This must finish instantly.
	pattern := strings.Repeat("a*", 30) + "b"
	s := strings.Repeat("a", 5000)
	if globMatch(pattern, s) {
		t.Fatal("should not match")
	}
}

package runner

import (
	"strings"
	"testing"
)

func TestMatch(t *testing.T) {
	cases := []struct {
		name     string
		expected string
		got      string
		exact    bool
		want     bool
	}{
		{"identical", "1 2 3\n", "1 2 3\n", false, true},
		{"extra whitespace", "1  2\n3\n", "1 2 3", false, true},
		{"crlf", "a\r\nb\r\n", "a\nb\n", false, true},
		{"different token", "1 2", "1 2 3", false, false},
		{"exact rejects whitespace", "1 2\n", "1 2", true, false},
		{"exact identical", "1 2\n", "1 2\n", true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Match([]byte(tc.expected), []byte(tc.got), tc.exact)
			if got != tc.want {
				t.Fatalf("Match(%q, %q, exact=%v) = %v, want %v", tc.expected, tc.got, tc.exact, got, tc.want)
			}
		})
	}
}

func TestDiffSummaryLineOriented(t *testing.T) {
	got := DiffSummary("562\n", "563\n", 10)
	if !strings.Contains(got, "- 562") || !strings.Contains(got, "+ 563") {
		t.Fatalf("expected line diff, got:\n%s", got)
	}
	if strings.Contains(got, "- 2\n") {
		t.Fatalf("diff should be line-oriented, got:\n%s", got)
	}
}

func TestDiffSummaryTruncates(t *testing.T) {
	got := DiffSummary("a\nb\nc\n", "x\ny\nz\n", 2)
	if !strings.Contains(got, "diff truncated") {
		t.Fatalf("expected truncation marker, got:\n%s", got)
	}
}

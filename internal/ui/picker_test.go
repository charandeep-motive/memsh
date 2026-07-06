package ui

import "testing"

func TestTruncate(t *testing.T) {
	cases := []struct {
		name string
		in   string
		w    int
		want string
	}{
		{"short unchanged", "git status", 20, "git status"},
		{"exact width unchanged", "abcde", 5, "abcde"},
		{"long cut with ellipsis", "kubectl config use-context", 10, "kubectl c…"},
		{"newline collapses to first line", "line one\nline two", 20, "line one"},
		{"newline then truncate", "a very long first line here\nsecond", 10, "a very lo…"},
		{"tiny width", "hello", 1, "…"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := truncate(tc.in, tc.w); got != tc.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tc.in, tc.w, got, tc.want)
			}
		})
	}
}

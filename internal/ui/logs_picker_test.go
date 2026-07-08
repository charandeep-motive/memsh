package ui

import (
	"reflect"
	"strings"
	"testing"
)

func TestSanitizeForDisplayRemovesControlSequences(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "vim OSC colour query stripped",
			in:   "before\x1b]11;rgb:1e1e/1e1e/1e1e\x07after",
			want: "beforeafter",
		},
		{
			name: "consecutive OSC sequences stripped",
			in:   "\x1b]11;rgb:1e1e/1e1e/1e1e\x1b]10;rgb:ffff/ffff/ffff\x07",
			want: "",
		},
		{
			name: "alt-screen and cursor control stripped",
			in:   "\x1b[?1049h\x1b[2J\x1b[Hhello\x1b[?1049l",
			want: "hello",
		},
		{
			name: "sgr colour stripped in plain mode",
			in:   "\x1b[32mgreen\x1b[0m",
			want: "green",
		},
		{
			name: "stray control bytes removed",
			in:   "a\x00b\x07c\x1bMd",
			want: "abcd",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeForDisplay(tc.in, false)
			if got != tc.want {
				t.Errorf("sanitizeForDisplay(%q, false) = %q, want %q", tc.in, got, tc.want)
			}
			if strings.ContainsAny(got, "\x1b\x07\x00") {
				t.Errorf("sanitizeForDisplay left control bytes in %q", got)
			}
		})
	}
}

func TestSanitizeForDisplayKeepsColorForPager(t *testing.T) {
	// SGR colour survives, but cursor/OSC control does not.
	in := "\x1b[?25l\x1b]11;rgb:1e1e/1e1e/1e1e\x07\x1b[32mgreen\x1b[0m\x1b[K"
	got := sanitizeForDisplay(in, true)
	if !strings.Contains(got, "\x1b[32m") || !strings.Contains(got, "green") || !strings.Contains(got, "\x1b[0m") {
		t.Errorf("sanitizeForDisplay(keepColor) dropped SGR/text: %q", got)
	}
	if strings.Contains(got, "]11;") || strings.Contains(got, "\x1b[?25l") || strings.Contains(got, "\x1b[K") {
		t.Errorf("sanitizeForDisplay(keepColor) left non-SGR control: %q", got)
	}
}

func TestCleanTerminalOutputStripsVimCapture(t *testing.T) {
	// A vim-style capture: alt-screen, OSC colour query, cursor moves, then a
	// real line. Nothing dangerous must survive.
	in := []byte("\x1b[?1049h\x1b]11;rgb:1e1e/1e1e/1e1e\x07\x1b[2;1Hedited text\x1b[?1049l\r\n")
	got := cleanTerminalOutput(in)
	joined := strings.Join(got, "\n")
	if strings.ContainsAny(joined, "\x1b\x07\x00") {
		t.Errorf("cleanTerminalOutput left control bytes: %q", got)
	}
	if !strings.Contains(joined, "edited text") {
		t.Errorf("cleanTerminalOutput dropped real text: %q", got)
	}
}

func TestRenderForPager(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "git progress collapses to final state",
			in:   "remote: Counting objects: 0% (1/1324)\rremote: Counting objects: 100% (1324/1324), done.\r\n",
			want: "remote: Counting objects: 100% (1324/1324), done.\n",
		},
		{
			name: "ansi colour sequences preserved",
			in:   "\x1b[32mdone\x1b[0m\r\n",
			want: "\x1b[32mdone\x1b[0m\n",
		},
		{
			name: "bsd headers dropped",
			in:   "Script started on X\nhello\r\nScript done on Y\n",
			want: "hello\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(RenderForPager([]byte(tc.in)))
			if got != tc.want {
				t.Errorf("RenderForPager(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestCleanTerminalOutput(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "crlf line endings preserved as content",
			in:   "line-a\r\nline-b\r\n",
			want: []string{"line-a", "line-b"},
		},
		{
			name: "bsd script header and footer dropped",
			in:   "Script started on Wed Jul  8 14:25:00 2026\nHELLO\r\nScript done on Wed Jul  8 14:25:01 2026\n",
			want: []string{"HELLO"},
		},
		{
			name: "ansi escapes stripped",
			in:   "\x1b[32mgreen\x1b[0m text\r\n",
			want: []string{"green text"},
		},
		{
			name: "mid-line carriage return overwrite keeps final segment",
			in:   "progress 10%\rprogress 100%\r\n",
			want: []string{"progress 100%"},
		},
		{
			name: "zsh prompt marker line dropped",
			in:   "HELLO_ONE_123\r\n%                       \r",
			want: []string{"HELLO_ONE_123"},
		},
		{
			name: "blank lines removed",
			in:   "a\r\n\r\n\r\nb\r\n",
			want: []string{"a", "b"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cleanTerminalOutput([]byte(tc.in))
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("cleanTerminalOutput(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

package ui

import (
	"reflect"
	"testing"
)

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

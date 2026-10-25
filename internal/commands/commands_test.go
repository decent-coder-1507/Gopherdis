package commands

import (
	"bytes"
	"strings"
	"testing"

	"gopherdis/internal/resp"
)

// run executes one command line (split on spaces) and returns the raw reply.
func run(t *testing.T, tbl *Table, line string) string {
	t.Helper()
	var args [][]byte
	for _, f := range strings.Fields(line) {
		args = append(args, []byte(f))
	}
	return runArgs(t, tbl, args...)
}

func runArgs(t *testing.T, tbl *Table, args ...[]byte) string {
	t.Helper()
	var buf bytes.Buffer
	w := resp.NewWriter(&buf)
	tbl.Run(args, w)
	w.Flush()
	return buf.String()
}

func TestPingEchoCommand(t *testing.T) {
	tbl := NewTable()
	tests := []struct{ in, want string }{
		{"PING", "+PONG\r\n"},
		{"ping", "+PONG\r\n"},
		{"Ping", "+PONG\r\n"},
		{"PING hello", "$5\r\nhello\r\n"},
		{"PING a b", "-ERR wrong number of arguments for 'ping' command\r\n"},
		{"ECHO hi", "$2\r\nhi\r\n"},
		{"echo hi", "$2\r\nhi\r\n"},
		{"ECHO", "-ERR wrong number of arguments for 'echo' command\r\n"},
		{"ECHO a b", "-ERR wrong number of arguments for 'echo' command\r\n"},
		{"COMMAND", "*0\r\n"},
		{"COMMAND DOCS", "*0\r\n"},
		{"command docs", "*0\r\n"},
		{"FOO", "-ERR unknown command 'FOO'\r\n"},
		{"foo bar", "-ERR unknown command 'foo'\r\n"},
	}
	for _, tc := range tests {
		if got := run(t, tbl, tc.in); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestEchoKeepsBytes(t *testing.T) {
	msg := []byte("Hello World\r\n\x00")
	got := runArgs(t, NewTable(), []byte("ECHO"), msg)
	if want := "$14\r\nHello World\r\n\x00\r\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

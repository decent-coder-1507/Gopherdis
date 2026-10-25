package commands

import (
	"bytes"
	"strings"
	"testing"

	"gopherdis/internal/resp"
	"gopherdis/internal/store"
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
	tbl := NewTable(store.New())
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
	got := runArgs(t, NewTable(store.New()), []byte("ECHO"), msg)
	if want := "$14\r\nHello World\r\n\x00\r\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSetGet(t *testing.T) {
	tbl := NewTable(store.New())
	tests := []struct{ in, want string }{
		{"GET name", "$-1\r\n"},
		{"SET name Srijoy", "+OK\r\n"},
		{"GET name", "$6\r\nSrijoy\r\n"},
		{"set name Paul", "+OK\r\n"},
		{"get name", "$4\r\nPaul\r\n"},
		{"GET nothing", "$-1\r\n"},
		{"SET", "-ERR wrong number of arguments for 'set' command\r\n"},
		{"SET k", "-ERR wrong number of arguments for 'set' command\r\n"},
		{"SET k v junk", "-ERR syntax error\r\n"},
		{"GET", "-ERR wrong number of arguments for 'get' command\r\n"},
		{"GET a b", "-ERR wrong number of arguments for 'get' command\r\n"},
	}
	for _, tc := range tests {
		if got := run(t, tbl, tc.in); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSetGetEmptyAndBinary(t *testing.T) {
	tbl := NewTable(store.New())
	runArgs(t, tbl, []byte("SET"), []byte("e"), []byte{})
	if got := run(t, tbl, "GET e"); got != "$0\r\n\r\n" {
		t.Errorf("empty value: got %q", got)
	}
	runArgs(t, tbl, []byte("SET"), []byte("b"), []byte("\x00\r\n\xff"))
	if got := run(t, tbl, "GET b"); got != "$4\r\n\x00\r\n\xff\r\n" {
		t.Errorf("binary value: got %q", got)
	}
}

func TestConfigGet(t *testing.T) {
	tbl := NewTable(store.New())
	tests := []struct{ in, want string }{
		{"CONFIG GET save", "*2\r\n$4\r\nsave\r\n$0\r\n\r\n"},
		{"config get APPENDONLY", "*2\r\n$10\r\nappendonly\r\n$2\r\nno\r\n"},
		{"CONFIG GET nosuchthing", "*0\r\n"},
		{"CONFIG GET save nosuchthing appendonly",
			"*4\r\n$4\r\nsave\r\n$0\r\n\r\n$10\r\nappendonly\r\n$2\r\nno\r\n"},
		{"CONFIG", "-ERR wrong number of arguments for 'config' command\r\n"},
		{"CONFIG GET", "-ERR wrong number of arguments for 'config|get' command\r\n"},
		{"CONFIG SET a b", "-ERR unknown subcommand 'SET'\r\n"},
	}
	for _, tc := range tests {
		if got := run(t, tbl, tc.in); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.in, got, tc.want)
		}
	}
}

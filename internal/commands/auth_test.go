package commands

import (
	"testing"

	"gopherdis/internal/store"
)

const (
	noAuthReply    = "-NOAUTH Authentication required.\r\n"
	wrongPassReply = "-WRONGPASS invalid username-password pair or user is disabled.\r\n"
)

func TestAuthRequired(t *testing.T) {
	tbl := NewTable(store.New(), Env{Password: "secret123"})
	ctx, out := newConn(tbl)

	steps := []struct{ in, want string }{
		{"GET a", noAuthReply},
		{"PING", noAuthReply},
		{"NOSUCHCOMMAND", noAuthReply}, // nothing leaks, not even which commands exist
		{"CONFIG GET requirepass", noAuthReply},
		{"AUTH wrong", wrongPassReply},
		{"AUTH secret12", wrongPassReply},
		{"AUTH secret1234", wrongPassReply},
		{"AUTH SECRET123", wrongPassReply},
		{"GET a", noAuthReply}, // still locked after failures
		{"AUTH nobody secret123", wrongPassReply},
		{"AUTH a b c", "-ERR syntax error\r\n"},
		{"AUTH", "-ERR wrong number of arguments for 'auth' command\r\n"},
		{"auth secret123", "+OK\r\n"},
		{"SET a 1", "+OK\r\n"},
		{"GET a", "$1\r\n1\r\n"},
		{"AUTH wrong", wrongPassReply},
		{"GET a", "$1\r\n1\r\n"}, // a failed AUTH later doesn't log the client out
	}
	for _, s := range steps {
		if got := out.run(ctx, s.in); got != s.want {
			t.Errorf("%s: got %q, want %q", s.in, got, s.want)
		}
	}
}

func TestAuthIsPerConnection(t *testing.T) {
	tbl := NewTable(store.New(), Env{Password: "pw"})
	first, out1 := newConn(tbl)
	second, out2 := newConn(tbl)
	if got := out1.run(first, "AUTH default pw"); got != "+OK\r\n" {
		t.Fatalf("AUTH default pw: %q", got)
	}
	if got := out1.run(first, "PING"); got != "+PONG\r\n" {
		t.Errorf("first connection after AUTH: %q", got)
	}
	if got := out2.run(second, "PING"); got != noAuthReply {
		t.Errorf("second connection without AUTH: %q", got)
	}
}

func TestAuthWithoutPassword(t *testing.T) {
	tbl := NewTable(store.New(), Env{})
	got := run(t, tbl, "AUTH anything")
	want := "-ERR AUTH called without any password configured for the default user. Are you sure your configuration is correct?\r\n"
	if got != want {
		t.Errorf("got %q", got)
	}
	if got := run(t, tbl, "PING"); got != "+PONG\r\n" {
		t.Errorf("no password set, PING: %q", got)
	}
}

func TestPasswordMatches(t *testing.T) {
	if !passwordMatches([]byte("s3cret"), "s3cret") {
		t.Error("right password rejected")
	}
	for _, guess := range []string{"", "s", "s3cre", "s3cret ", "S3cret", "s3cret\x00"} {
		if passwordMatches([]byte(guess), "s3cret") {
			t.Errorf("%q accepted", guess)
		}
	}
}

package commands

import (
	"testing"
	"time"

	"gopherdis/internal/store"
)

const (
	wrongTypeReply = "-WRONGTYPE Operation against a key holding the wrong kind of value\r\n"
	notIntReply    = "-ERR value is not an integer or out of range\r\n"
)

// The "How to test" lines of Step 6 in order, then the other rules.
func TestStep6Commands(t *testing.T) {
	tbl := NewTable(store.New())
	tests := []struct{ in, want string }{
		{"SET n 10", "+OK\r\n"},
		{"INCR n", ":11\r\n"},
		{"INCR new", ":1\r\n"},
		{"SET word hi", "+OK\r\n"},
		{"INCR word", notIntReply},
		{"RPUSH list a b", ":2\r\n"},
		{"LPUSH list z", ":3\r\n"},
		{"GET list", wrongTypeReply},
		{"EXISTS n list nope", ":2\r\n"},
		{"DEL n list nope", ":2\r\n"},
		{"EXISTS n list", ":0\r\n"},

		{"DECR new", ":0\r\n"},
		{"DECR fresh", ":-1\r\n"},
		{"SET f 1.5", "+OK\r\n"},
		{"DECR f", notIntReply},
		{"SET big 9223372036854775807", "+OK\r\n"},
		{"INCR big", notIntReply},
		{"SET small -9223372036854775808", "+OK\r\n"},
		{"DECR small", notIntReply},
		{"LPUSH word x", wrongTypeReply},
		{"RPUSH word x", wrongTypeReply},
		{"LPUSH l2 x", ":1\r\n"},
		{"INCR l2", wrongTypeReply},
		{"DECR l2", wrongTypeReply},
		{"SET l2 str", "+OK\r\n"},
		{"GET l2", "$3\r\nstr\r\n"},
		{"EXISTS word word", ":2\r\n"},

		{"EXISTS", "-ERR wrong number of arguments for 'exists' command\r\n"},
		{"DEL", "-ERR wrong number of arguments for 'del' command\r\n"},
		{"INCR", "-ERR wrong number of arguments for 'incr' command\r\n"},
		{"INCR a b", "-ERR wrong number of arguments for 'incr' command\r\n"},
		{"DECR", "-ERR wrong number of arguments for 'decr' command\r\n"},
		{"LPUSH k", "-ERR wrong number of arguments for 'lpush' command\r\n"},
		{"RPUSH k", "-ERR wrong number of arguments for 'rpush' command\r\n"},
	}
	for _, tc := range tests {
		if got := run(t, tbl, tc.in); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestIncrKeepsExpiry(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tbl := NewTable(store.NewWithClock(func() time.Time { return now }))
	run(t, tbl, "SET n 1 EX 10")
	run(t, tbl, "INCR n")
	run(t, tbl, "DECR n")
	run(t, tbl, "INCR n")
	now = now.Add(9 * time.Second)
	if got := run(t, tbl, "GET n"); got != "$1\r\n2\r\n" {
		t.Fatalf("before expiry: %q", got)
	}
	now = now.Add(time.Second)
	if got := run(t, tbl, "GET n"); got != "$-1\r\n" {
		t.Fatalf("INCR lost the expiry: %q", got)
	}
}

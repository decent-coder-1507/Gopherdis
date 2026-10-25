package commands

import (
	"bufio"
	"sort"
	"strings"
	"testing"
	"time"

	"gopherdis/internal/resp"
	"gopherdis/internal/store"
)

func TestTTLAndExpire(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tbl := NewTable(store.NewWithClock(func() time.Time { return now }))
	tests := []struct{ in, want string }{
		{"TTL nokey", ":-2\r\n"},
		{"PTTL nokey", ":-2\r\n"},
		{"SET k v", "+OK\r\n"},
		{"TTL k", ":-1\r\n"},
		{"PTTL k", ":-1\r\n"},
		{"EXPIRE k 100", ":1\r\n"},
		{"TTL k", ":100\r\n"},
		{"PTTL k", ":100000\r\n"},
		{"EXPIRE nokey 100", ":0\r\n"},
		{"PERSIST k", ":1\r\n"},
		{"PERSIST k", ":0\r\n"}, // no expiry any more
		{"PERSIST nokey", ":0\r\n"},
		{"TTL k", ":-1\r\n"},
		{"SET p v PX 1500", "+OK\r\n"},
		{"TTL p", ":2\r\n"}, // 1.5 s rounds to 2, like Redis
		{"PTTL p", ":1500\r\n"},
		{"EXPIRE k abc", "-ERR value is not an integer or out of range\r\n"},
		{"EXPIRE k 99999999999999999", "-ERR invalid expire time in 'expire' command\r\n"},
		{"EXPIRE k", "-ERR wrong number of arguments for 'expire' command\r\n"},
		{"TTL", "-ERR wrong number of arguments for 'ttl' command\r\n"},
		{"EXPIRE k 0", ":1\r\n"}, // deletes, like Redis
		{"EXISTS k", ":0\r\n"},
		{"RPUSH l a", ":1\r\n"},
		{"EXPIRE l 10", ":1\r\n"}, // works on any type
		{"TTL l", ":10\r\n"},
	}
	for _, tc := range tests {
		if got := run(t, tbl, tc.in); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.in, got, tc.want)
		}
	}

	now = now.Add(1400 * time.Millisecond)
	if got := run(t, tbl, "PTTL p"); got != ":100\r\n" {
		t.Errorf("PTTL after 1.4 s: %q", got)
	}
	now = now.Add(100 * time.Millisecond)
	if got := run(t, tbl, "TTL p"); got != ":-2\r\n" {
		t.Errorf("TTL after expiry: %q", got)
	}
	now = now.Add(10 * time.Second)
	if got := run(t, tbl, "LLEN l"); got != ":0\r\n" {
		t.Errorf("EXPIRE on a list did not expire it: %q", got)
	}
}

func TestPersistedKeyKeepsValue(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tbl := NewTable(store.NewWithClock(func() time.Time { return now }))
	run(t, tbl, "SET n 5 EX 10")
	run(t, tbl, "PERSIST n")
	now = now.Add(time.Hour)
	if got := run(t, tbl, "INCR n"); got != ":6\r\n" {
		t.Errorf("got %q", got)
	}
}

// arrayReply parses a RESP array reply; nil items become "<nil>".
func arrayReply(t *testing.T, raw string) []string {
	t.Helper()
	v, err := resp.NewReader(bufio.NewReader(strings.NewReader(raw))).Read()
	if err != nil || v.Type != resp.Array {
		t.Fatalf("not an array: %q (%v)", raw, err)
	}
	out := []string{}
	for _, item := range v.Array {
		if item.Null {
			out = append(out, "<nil>")
		} else {
			out = append(out, string(item.Bulk))
		}
	}
	return out
}

func TestKeys(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tbl := NewTable(store.NewWithClock(func() time.Time { return now }))
	for _, k := range []string{"user:1", "user:2", "user:10", "hello", "hallo", "hxllo"} {
		run(t, tbl, "SET "+k+" x")
	}
	run(t, tbl, "RPUSH users a")
	run(t, tbl, "SET gone x PX 10")
	now = now.Add(time.Second)

	tests := []struct{ pattern, want string }{
		{"*", "hallo hello hxllo user:1 user:10 user:2 users"}, // not "gone": it expired
		{"user:*", "user:1 user:10 user:2"},
		{"user:?", "user:1 user:2"},
		{"h[ae]llo", "hallo hello"},
		{"h[^e]llo", "hallo hxllo"},
		{"nothing*", ""},
		{"gone", ""},
	}
	for _, tc := range tests {
		got := arrayReply(t, run(t, tbl, "KEYS "+tc.pattern))
		sort.Strings(got)
		want := strings.Fields(tc.want)
		if strings.Join(got, " ") != strings.Join(want, " ") {
			t.Errorf("KEYS %s = %v, want %v", tc.pattern, got, want)
		}
	}
}

func TestLRangeLLen(t *testing.T) {
	tbl := NewTable(store.New())
	run(t, tbl, "RPUSH l a b c d e")
	tests := []struct{ in, want string }{
		{"LRANGE l 0 -1", "a b c d e"},
		{"LRANGE l 0 0", "a"},
		{"LRANGE l 1 2", "b c"},
		{"LRANGE l -2 -1", "d e"},
		{"LRANGE l -100 100", "a b c d e"},
		{"LRANGE l 3 1", ""},
		{"LRANGE l 5 10", ""},
		{"LRANGE l -1 -2", ""},
		{"LRANGE l 4 4", "e"},
		{"LRANGE nokey 0 -1", ""},
	}
	for _, tc := range tests {
		got := strings.Join(arrayReply(t, run(t, tbl, tc.in)), " ")
		if got != tc.want {
			t.Errorf("%s = %q, want %q", tc.in, got, tc.want)
		}
	}

	run(t, tbl, "SET s x")
	errs := []struct{ in, want string }{
		{"LLEN l", ":5\r\n"},
		{"LLEN nokey", ":0\r\n"},
		{"LLEN s", wrongTypeReply},
		{"LRANGE s 0 -1", wrongTypeReply},
		{"LRANGE l a 1", notIntReply},
		{"LRANGE l 0 1.5", notIntReply},
		{"LRANGE l 0", "-ERR wrong number of arguments for 'lrange' command\r\n"},
		{"LLEN", "-ERR wrong number of arguments for 'llen' command\r\n"},
	}
	for _, tc := range errs {
		if got := run(t, tbl, tc.in); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMGet(t *testing.T) {
	tbl := NewTable(store.New())
	run(t, tbl, "SET a 1")
	run(t, tbl, "SET b 2")
	run(t, tbl, "RPUSH l x")
	got := strings.Join(arrayReply(t, run(t, tbl, "MGET a nope b l a")), " ")
	if got != "1 <nil> 2 <nil> 1" {
		t.Errorf("MGET = %q", got)
	}
	if got := run(t, tbl, "MGET"); got != "-ERR wrong number of arguments for 'mget' command\r\n" {
		t.Errorf("MGET with no keys: %q", got)
	}
}

func TestDBSizeFlushAll(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tbl := NewTable(store.NewWithClock(func() time.Time { return now }))
	if got := run(t, tbl, "DBSIZE"); got != ":0\r\n" {
		t.Fatalf("empty DBSIZE: %q", got)
	}
	run(t, tbl, "SET a 1")
	run(t, tbl, "RPUSH l x")
	run(t, tbl, "SET t 1 PX 100")
	if got := run(t, tbl, "DBSIZE"); got != ":3\r\n" {
		t.Fatalf("DBSIZE: %q", got)
	}
	now = now.Add(time.Second)
	if got := run(t, tbl, "DBSIZE"); got != ":2\r\n" {
		t.Fatalf("DBSIZE after one key expired (not yet cleaned up): %q", got)
	}

	for _, in := range []string{"FLUSHALL", "flushall async", "FLUSHALL SYNC"} {
		run(t, tbl, "SET x 1")
		if got := run(t, tbl, in); got != "+OK\r\n" {
			t.Fatalf("%s: %q", in, got)
		}
		if got := run(t, tbl, "DBSIZE"); got != ":0\r\n" {
			t.Fatalf("DBSIZE after %s: %q", in, got)
		}
	}
	if got := run(t, tbl, "FLUSHALL NOW"); got != "-ERR syntax error\r\n" {
		t.Errorf("FLUSHALL NOW: %q", got)
	}
	if got := run(t, tbl, "DBSIZE x"); got != "-ERR wrong number of arguments for 'dbsize' command\r\n" {
		t.Errorf("DBSIZE x: %q", got)
	}
}

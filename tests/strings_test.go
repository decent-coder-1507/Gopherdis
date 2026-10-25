package tests

import (
	"testing"

	"github.com/redis/go-redis/v9"
)

func TestSetGet(t *testing.T) {
	ctx := testContext(t)
	rdb := newClient(t, startServer(t))

	if err := rdb.Set(ctx, "name", "Srijoy", 0).Err(); err != nil {
		t.Fatal(err)
	}
	if got, err := rdb.Get(ctx, "name").Result(); err != nil || got != "Srijoy" {
		t.Fatalf("GET name: %q, %v", got, err)
	}

	// Overwrite.
	rdb.Set(ctx, "name", "Paul", 0)
	if got, _ := rdb.Get(ctx, "name").Result(); got != "Paul" {
		t.Fatalf("after overwrite: %q", got)
	}

	// Missing key is nil.
	if err := rdb.Get(ctx, "nothing").Err(); err != redis.Nil {
		t.Fatalf("GET nothing: %v, want redis.Nil", err)
	}
}

func TestSetGetValues(t *testing.T) {
	ctx := testContext(t)
	rdb := newClient(t, startServer(t))

	values := map[string]string{
		"empty":  "",
		"spaces": "hello big world",
		"binary": "\x00\x01\r\n\xff\xfe end",
		"crlf":   "\r\n\r\n",
	}
	for k, v := range values {
		if err := rdb.Set(ctx, k, v, 0).Err(); err != nil {
			t.Fatalf("SET %s: %v", k, err)
		}
	}
	for k, want := range values {
		got, err := rdb.Get(ctx, k).Result()
		if err != nil || got != want {
			t.Errorf("GET %s: got %q, %v; want %q", k, got, err, want)
		}
	}
}

func TestSetGetArgCounts(t *testing.T) {
	ctx := testContext(t)
	rdb := newClient(t, startServer(t))

	cases := []struct {
		args []any
		want string
	}{
		{[]any{"SET"}, "ERR wrong number of arguments for 'set' command"},
		{[]any{"SET", "k"}, "ERR wrong number of arguments for 'set' command"},
		{[]any{"GET"}, "ERR wrong number of arguments for 'get' command"},
		{[]any{"GET", "a", "b"}, "ERR wrong number of arguments for 'get' command"},
	}
	for _, c := range cases {
		err := rdb.Do(ctx, c.args...).Err()
		if err == nil || err.Error() != c.want {
			t.Errorf("%v: got %v, want %q", c.args, err, c.want)
		}
	}
}

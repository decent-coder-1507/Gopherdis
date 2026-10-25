package tests

import (
	"strconv"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestSetExpiryOverNetwork(t *testing.T) {
	ctx := testContext(t)
	rdb := newClient(t, startServer(t))

	now := time.Now()
	steps := [][]any{
		{"SET", "px", "v", "PX", "150"},
		{"SET", "ex", "v", "EX", "100"},
		{"SET", "pxat", "v", "PXAT", strconv.FormatInt(now.Add(150*time.Millisecond).UnixMilli(), 10)},
		{"SET", "exat", "v", "EXAT", strconv.FormatInt(now.Add(100*time.Second).Unix(), 10)},
	}
	for _, args := range steps {
		if err := rdb.Do(ctx, args...).Err(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	for _, k := range []string{"px", "ex", "pxat", "exat"} {
		if got, err := rdb.Get(ctx, k).Result(); err != nil || got != "v" {
			t.Fatalf("GET %s before expiry: %q, %v", k, got, err)
		}
	}

	time.Sleep(300 * time.Millisecond)
	for _, k := range []string{"px", "pxat"} {
		if err := rdb.Get(ctx, k).Err(); err != redis.Nil {
			t.Errorf("GET %s after expiry: %v, want nil", k, err)
		}
	}
	for _, k := range []string{"ex", "exat"} {
		if got, _ := rdb.Get(ctx, k).Result(); got != "v" {
			t.Errorf("GET %s: %q, want it to still exist", k, got)
		}
	}
}

func TestSetExpiryErrorsOverNetwork(t *testing.T) {
	ctx := testContext(t)
	rdb := newClient(t, startServer(t))

	for _, args := range [][]any{
		{"SET", "k", "v", "EX", "0"},
		{"SET", "k", "v", "PX", "-1"},
		{"SET", "k", "v", "EX", "soon"},
	} {
		err := rdb.Do(ctx, args...).Err()
		if err == nil || err.Error() != "ERR invalid expire time in 'set' command" {
			t.Errorf("%v: %v", args, err)
		}
	}
	err := rdb.Do(ctx, "SET", "k", "v", "EX", "10", "PX", "10").Err()
	if err == nil || err.Error() != "ERR syntax error" {
		t.Errorf("two options: %v", err)
	}
}

func TestPlainSetClearsExpiryOverNetwork(t *testing.T) {
	ctx := testContext(t)
	rdb := newClient(t, startServer(t))

	rdb.Set(ctx, "k", "old", 100*time.Millisecond)
	rdb.Set(ctx, "k", "new", 0)
	time.Sleep(200 * time.Millisecond)
	if got, err := rdb.Get(ctx, "k").Result(); err != nil || got != "new" {
		t.Fatalf("got %q, %v", got, err)
	}
}

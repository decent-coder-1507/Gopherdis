package tests

import (
	"testing"
	"time"
)

const (
	wrongType  = "WRONGTYPE Operation against a key holding the wrong kind of value"
	notInteger = "ERR value is not an integer or out of range"
)

func TestStep6OverNetwork(t *testing.T) {
	ctx := testContext(t)
	rdb := newClient(t, startServer(t))

	rdb.Set(ctx, "n", "10", 0)
	if n, err := rdb.Incr(ctx, "n").Result(); err != nil || n != 11 {
		t.Fatalf("INCR n: %d, %v", n, err)
	}
	if n, err := rdb.Incr(ctx, "new").Result(); err != nil || n != 1 {
		t.Fatalf("INCR new: %d, %v", n, err)
	}
	if n, err := rdb.Decr(ctx, "new").Result(); err != nil || n != 0 {
		t.Fatalf("DECR new: %d, %v", n, err)
	}
	rdb.Set(ctx, "word", "hi", 0)
	if err := rdb.Incr(ctx, "word").Err(); err == nil || err.Error() != notInteger {
		t.Fatalf("INCR word: %v", err)
	}
	if n, err := rdb.RPush(ctx, "list", "a", "b").Result(); err != nil || n != 2 {
		t.Fatalf("RPUSH: %d, %v", n, err)
	}
	if n, err := rdb.LPush(ctx, "list", "z").Result(); err != nil || n != 3 {
		t.Fatalf("LPUSH: %d, %v", n, err)
	}
	if err := rdb.Get(ctx, "list").Err(); err == nil || err.Error() != wrongType {
		t.Fatalf("GET list: %v", err)
	}
	if err := rdb.LPush(ctx, "word", "x").Err(); err == nil || err.Error() != wrongType {
		t.Fatalf("LPUSH word: %v", err)
	}
	if n, err := rdb.Exists(ctx, "n", "list", "nope").Result(); err != nil || n != 2 {
		t.Fatalf("EXISTS: %d, %v", n, err)
	}
	if n, err := rdb.Del(ctx, "n", "list", "nope").Result(); err != nil || n != 2 {
		t.Fatalf("DEL: %d, %v", n, err)
	}
	if n, _ := rdb.Exists(ctx, "n", "list").Result(); n != 0 {
		t.Fatalf("keys still exist after DEL: %d", n)
	}
}

func TestIncrKeepsExpiryOverNetwork(t *testing.T) {
	ctx := testContext(t)
	rdb := newClient(t, startServer(t))

	rdb.Set(ctx, "n", "1", 200*time.Millisecond)
	if n, err := rdb.Incr(ctx, "n").Result(); err != nil || n != 2 {
		t.Fatalf("INCR: %d, %v", n, err)
	}
	time.Sleep(300 * time.Millisecond)
	if n, _ := rdb.Exists(ctx, "n").Result(); n != 0 {
		t.Fatal("INCR removed the expiry")
	}
}

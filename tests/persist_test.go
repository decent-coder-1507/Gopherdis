package tests

import (
	"path/filepath"
	"testing"
	"time"
)

// The Step 7 "How to test" sequence: write some keys, SAVE, restart, read back.
func TestSaveAndRestart(t *testing.T) {
	ctx := testContext(t)
	path := filepath.Join(t.TempDir(), "dump.gob")

	addr, stop := startServerWith(t, options{dumpPath: path})
	rdb := newClient(t, addr)
	rdb.Set(ctx, "a", "1", 0)
	rdb.RPush(ctx, "l", "x", "y")
	rdb.Set(ctx, "t", "1", 1000*time.Second)
	rdb.Set(ctx, "bin", "\x00\r\n\xff", 0)
	rdb.Set(ctx, "short", "gone soon", 100*time.Millisecond)
	if got, err := rdb.Do(ctx, "SAVE").Text(); err != nil || got != "OK" {
		t.Fatalf("SAVE: %q, %v", got, err)
	}
	stop()

	time.Sleep(150 * time.Millisecond) // let "short" expire while the server is down

	addr, _ = startServerWith(t, options{dumpPath: path})
	rdb = newClient(t, addr)
	if got, _ := rdb.Get(ctx, "a").Result(); got != "1" {
		t.Errorf("GET a after restart: %q", got)
	}
	if n, _ := rdb.Exists(ctx, "l", "t").Result(); n != 2 {
		t.Errorf("EXISTS l t after restart: %d", n)
	}
	if got, _ := rdb.Get(ctx, "bin").Result(); got != "\x00\r\n\xff" {
		t.Errorf("binary value after restart: %q", got)
	}
	if n, _ := rdb.Exists(ctx, "short").Result(); n != 0 {
		t.Error("a key that expired while the server was down came back")
	}
	// The list kept its order and is still a list.
	if n, _ := rdb.LPush(ctx, "l", "w").Result(); n != 3 {
		t.Errorf("LPUSH on loaded list: %d", n)
	}
}

func TestSaveWithoutSaver(t *testing.T) {
	ctx := testContext(t)
	rdb := newClient(t, startServer(t))
	if err := rdb.Do(ctx, "SAVE").Err(); err == nil {
		t.Fatal("SAVE succeeded with saving switched off")
	}
}

func TestBgsaveAndLastsave(t *testing.T) {
	ctx := testContext(t)
	addr, _ := startServerWith(t, options{dumpPath: filepath.Join(t.TempDir(), "dump.gob")})
	rdb := newClient(t, addr)
	rdb.Set(ctx, "a", "1", 0)

	before, err := rdb.LastSave(ctx).Result()
	if err != nil {
		t.Fatal(err)
	}
	// Two BGSAVEs in one pipeline: the second arrives while the first runs.
	pipe := rdb.Pipeline()
	first := pipe.BgSave(ctx)
	second := pipe.BgSave(ctx)
	pipe.Exec(ctx)
	if first.Val() != "Background saving started" {
		t.Fatalf("first BGSAVE: %v", first.Err())
	}
	if second.Err() == nil || second.Err().Error() != "ERR Background save already in progress" {
		t.Fatalf("second BGSAVE: %v", second.Err())
	}

	time.Sleep(1100 * time.Millisecond) // LASTSAVE has 1-second resolution
	if err := rdb.Save(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	if after, _ := rdb.LastSave(ctx).Result(); after <= before {
		t.Fatalf("LASTSAVE did not move: %d -> %d", before, after)
	}
}

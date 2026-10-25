package tests

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// 1,000 commands in one pipeline; every reply must come back in order.
func TestPipelineOrder(t *testing.T) {
	ctx := testContext(t)
	rdb := newClient(t, startServer(t))

	pipe := rdb.Pipeline()
	var cmds []redis.Cmder
	for i := 0; i < 1000; i++ {
		key := "k" + strconv.Itoa((i/4)%10) // the SET and GET of a group share a key
		switch i % 4 {
		case 0:
			cmds = append(cmds, pipe.Set(ctx, key, i, 0))
		case 1:
			cmds = append(cmds, pipe.Get(ctx, key))
		case 2:
			cmds = append(cmds, pipe.Incr(ctx, "counter"))
		case 3:
			cmds = append(cmds, pipe.Echo(ctx, "e"+strconv.Itoa(i)))
		}
	}
	if _, err := pipe.Exec(ctx); err != nil {
		t.Fatal(err)
	}

	counter := int64(0)
	for i, c := range cmds {
		switch i % 4 {
		case 0:
			if c.(*redis.StatusCmd).Val() != "OK" {
				t.Fatalf("reply %d: %v", i, c)
			}
		case 1:
			// GET right after the SET of the same key in the same pipeline.
			if got := c.(*redis.StringCmd).Val(); got != strconv.Itoa(i-1) {
				t.Fatalf("reply %d: GET = %q, want %d", i, got, i-1)
			}
		case 2:
			counter++
			if got := c.(*redis.IntCmd).Val(); got != counter {
				t.Fatalf("reply %d: INCR = %d, want %d", i, got, counter)
			}
		case 3:
			if got := c.(*redis.StringCmd).Val(); got != "e"+strconv.Itoa(i) {
				t.Fatalf("reply %d: ECHO = %q", i, got)
			}
		}
	}
}

// Many commands written in a single write still get every reply, in order.
func TestPipelineRawSingleWrite(t *testing.T) {
	addr := startServer(t)
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))

	var batch []byte
	for i := 0; i < 500; i++ {
		batch = fmt.Appendf(batch, "*2\r\n$4\r\nECHO\r\n$%d\r\n%d\r\n", len(strconv.Itoa(i)), i)
	}
	if _, err := c.Write(batch); err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(c)
	for i := 0; i < 500; i++ {
		if _, err := r.ReadString('\n'); err != nil { // $len
			t.Fatal(err)
		}
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if line != strconv.Itoa(i)+"\r\n" {
			t.Fatalf("reply %d: %q", i, line)
		}
	}
	// Nothing extra after the 500 replies.
	c.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if n, _ := io.ReadFull(r, make([]byte, 1)); n != 0 {
		t.Fatal("got more replies than commands")
	}
}

func TestStep9CommandsOverNetwork(t *testing.T) {
	ctx := testContext(t)
	rdb := newClient(t, startServer(t))

	if d, _ := rdb.TTL(ctx, "nokey").Result(); d != -2 {
		t.Errorf("TTL nokey = %v", d)
	}
	rdb.Set(ctx, "k", "v", 0)
	if d, _ := rdb.TTL(ctx, "k").Result(); d != -1 {
		t.Errorf("TTL with no expiry = %v", d)
	}
	if ok, _ := rdb.Expire(ctx, "k", 100*time.Second).Result(); !ok {
		t.Error("EXPIRE k returned 0")
	}
	if d, _ := rdb.TTL(ctx, "k").Result(); d != 100*time.Second {
		t.Errorf("TTL = %v", d)
	}
	if d, _ := rdb.PTTL(ctx, "k").Result(); d <= 99*time.Second || d > 100*time.Second {
		t.Errorf("PTTL = %v", d)
	}
	if ok, _ := rdb.Persist(ctx, "k").Result(); !ok {
		t.Error("PERSIST returned 0")
	}
	if ok, _ := rdb.Expire(ctx, "nokey", time.Second).Result(); ok {
		t.Error("EXPIRE on missing key returned 1")
	}

	rdb.RPush(ctx, "list", "a", "b", "c")
	if got, _ := rdb.LRange(ctx, "list", 0, -1).Result(); fmt.Sprint(got) != "[a b c]" {
		t.Errorf("LRANGE = %v", got)
	}
	if n, _ := rdb.LLen(ctx, "list").Result(); n != 3 {
		t.Errorf("LLEN = %d", n)
	}
	if err := rdb.LLen(ctx, "k").Err(); err == nil || err.Error() != wrongType {
		t.Errorf("LLEN on string: %v", err)
	}

	vals, _ := rdb.MGet(ctx, "k", "nope", "list").Result()
	if len(vals) != 3 || vals[0] != "v" || vals[1] != nil || vals[2] != nil {
		t.Errorf("MGET = %#v", vals)
	}

	keys, _ := rdb.Keys(ctx, "*").Result()
	sort.Strings(keys)
	if fmt.Sprint(keys) != "[k list]" {
		t.Errorf("KEYS * = %v", keys)
	}
	if n, _ := rdb.DBSize(ctx).Result(); n != 2 {
		t.Errorf("DBSIZE = %d", n)
	}
	if err := rdb.FlushAll(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	if n, _ := rdb.DBSize(ctx).Result(); n != 0 {
		t.Errorf("DBSIZE after FLUSHALL = %d", n)
	}
}

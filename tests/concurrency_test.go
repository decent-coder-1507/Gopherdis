package tests

import (
	"fmt"
	"sync"
	"testing"

	"github.com/redis/go-redis/v9"
)

// 50 clients, each on its own connection, each doing 1,000 SETs and GETs at
// the same time. Every value must come back exactly as written.
func TestManyClients(t *testing.T) {
	ctx := testContext(t)
	addr := startServer(t)

	const clients, ops = 50, 1000
	var wg sync.WaitGroup
	for c := 0; c < clients; c++ {
		wg.Add(1)
		go func(c int) {
			defer wg.Done()
			rdb := redis.NewClient(&redis.Options{Addr: addr, PoolSize: 1, MaxRetries: -1})
			defer rdb.Close()
			for i := 0; i < ops; i++ {
				key := fmt.Sprintf("c%d:%d", c, i)
				val := fmt.Sprintf("value-%d-%d", c, i)
				if err := rdb.Set(ctx, key, val, 0).Err(); err != nil {
					t.Errorf("SET %s: %v", key, err)
					return
				}
				if got, err := rdb.Get(ctx, key).Result(); err != nil || got != val {
					t.Errorf("GET %s: %q, %v", key, got, err)
					return
				}
			}
		}(c)
	}
	wg.Wait()

	rdb := newClient(t, addr)
	for c := 0; c < clients; c++ {
		for i := 0; i < ops; i++ {
			key := fmt.Sprintf("c%d:%d", c, i)
			want := fmt.Sprintf("value-%d-%d", c, i)
			if got, err := rdb.Get(ctx, key).Result(); err != nil || got != want {
				t.Fatalf("final check %s: %q, %v", key, got, err)
			}
		}
	}
}

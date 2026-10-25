package tests

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"gopherdis/internal/commands"
	"gopherdis/internal/server"
	"gopherdis/internal/store"
)

// startServer runs a server on a free local port and returns its address.
// It is closed when the test ends.
func startServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := server.New(commands.NewTable(store.New()))
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ln) }()
	t.Cleanup(func() {
		srv.Close()
		if err := <-done; err != nil {
			t.Errorf("Serve returned %v", err)
		}
	})
	return ln.Addr().String()
}

// newClient returns a go-redis client for addr, closed when the test ends.
func newClient(t *testing.T, addr string) *redis.Client {
	t.Helper()
	c := redis.NewClient(&redis.Options{Addr: addr, MaxRetries: -1})
	t.Cleanup(func() { c.Close() })
	return c
}

func testContext(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

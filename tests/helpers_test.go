package tests

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"gopherdis/internal/commands"
	"gopherdis/internal/persist"
	"gopherdis/internal/server"
	"gopherdis/internal/store"
)

// options changes how startServerWith sets up a server.
type options struct {
	// dumpPath turns on saving to this file and loads it at start, like main does.
	dumpPath string
}

// startServer runs a server on a free local port and returns its address.
// It is closed when the test ends.
func startServer(t *testing.T) string {
	t.Helper()
	addr, _ := startServerWith(t, options{})
	return addr
}

// startServerWith is startServer with options. It also returns a function
// that stops the server early, for tests that restart it.
func startServerWith(t *testing.T, opts options) (addr string, stop func()) {
	t.Helper()
	st := store.New()
	cmds := commands.NewTable(st)
	if opts.dumpPath != "" {
		saver := persist.NewSaver(opts.dumpPath, st)
		if _, _, err := saver.Load(); err != nil {
			t.Fatalf("loading %s: %v", opts.dumpPath, err)
		}
		cmds.SetSaver(saver)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := server.New(cmds)
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ln) }()
	stopped := false
	stop = func() {
		if stopped {
			return
		}
		stopped = true
		srv.Close()
		if err := <-done; err != nil {
			t.Errorf("Serve returned %v", err)
		}
	}
	t.Cleanup(stop)
	return ln.Addr().String(), stop
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

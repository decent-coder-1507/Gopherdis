package tests

import (
	"bufio"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"gopherdis/internal/server"
)

func TestPasswordOverNetwork(t *testing.T) {
	ctx := testContext(t)
	addr, _ := startServerWith(t, options{password: "secret123"})

	// No password: refused.
	anon := newClient(t, addr)
	err := anon.Get(ctx, "a").Err()
	if err == nil || err.Error() != "NOAUTH Authentication required." {
		t.Fatalf("GET without AUTH: %v", err)
	}

	// Wrong password: the client library's AUTH on connect fails.
	wrong := redis.NewClient(&redis.Options{Addr: addr, Password: "nope", MaxRetries: -1})
	defer wrong.Close()
	err = wrong.Ping(ctx).Err()
	if err == nil || err.Error() != "WRONGPASS invalid username-password pair or user is disabled." {
		t.Fatalf("wrong password: %v", err)
	}

	// Right password: everything works.
	good := redis.NewClient(&redis.Options{Addr: addr, Password: "secret123", MaxRetries: -1})
	defer good.Close()
	if err := good.Set(ctx, "a", "1", 0).Err(); err != nil {
		t.Fatalf("SET with password: %v", err)
	}
	if got, _ := good.Get(ctx, "a").Result(); got != "1" {
		t.Fatalf("GET with password: %q", got)
	}

	// Username "default" plus password, as redis-cli --user sends it.
	withUser := redis.NewClient(&redis.Options{Addr: addr, Username: "default", Password: "secret123", MaxRetries: -1})
	defer withUser.Close()
	if err := withUser.Ping(ctx).Err(); err != nil {
		t.Fatalf("AUTH default <password>: %v", err)
	}
}

func TestInlineOverNetwork(t *testing.T) {
	addr := startServer(t)
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	io.WriteString(c, "PING\r\n")
	line, _ := bufio.NewReader(c).ReadString('\n')
	if line != "+PONG\r\n" {
		t.Fatalf("got %q", line)
	}
}

func TestMaxClientsOverNetwork(t *testing.T) {
	ctx := testContext(t)
	addr, _ := startServerWith(t, options{server: server.Options{MaxClients: 1}})
	first := redis.NewClient(&redis.Options{Addr: addr, PoolSize: 1, MaxRetries: -1})
	defer first.Close()
	if err := first.Ping(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	second := redis.NewClient(&redis.Options{Addr: addr, PoolSize: 1, MaxRetries: -1})
	defer second.Close()
	// The server writes the error and closes at once. go-redis writes its
	// handshake before reading, so it sees either that error or the
	// closed connection ("broken pipe"), as with real Redis. The exact
	// bytes are checked over raw TCP in internal/server.TestMaxClients.
	err := second.Ping(ctx).Err()
	if err == nil {
		t.Fatal("second client was served")
	}
	var rerr redis.Error
	if errors.As(err, &rerr) && err.Error() != "ERR max number of clients reached" {
		t.Fatalf("second client: %v", err)
	}
	// The first client is unaffected.
	if err := first.Ping(ctx).Err(); err != nil {
		t.Fatalf("first client after the refusal: %v", err)
	}
}

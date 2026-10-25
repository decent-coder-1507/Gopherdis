package tests

import "testing"

func TestPingEcho(t *testing.T) {
	ctx := testContext(t)
	rdb := newClient(t, startServer(t))

	if got, err := rdb.Ping(ctx).Result(); err != nil || got != "PONG" {
		t.Fatalf("PING: %q, %v", got, err)
	}
	if got, err := rdb.Do(ctx, "PING", "hello").Text(); err != nil || got != "hello" {
		t.Fatalf("PING hello: %q, %v", got, err)
	}
	if got, err := rdb.Echo(ctx, "Hello World").Result(); err != nil || got != "Hello World" {
		t.Fatalf("ECHO: %q, %v", got, err)
	}
}

func TestUnknownCommandAndArgCount(t *testing.T) {
	ctx := testContext(t)
	rdb := newClient(t, startServer(t))

	err := rdb.Do(ctx, "FOO").Err()
	if err == nil || err.Error() != "ERR unknown command 'FOO'" {
		t.Fatalf("FOO: %v", err)
	}
	err = rdb.Do(ctx, "ECHO").Err()
	if err == nil || err.Error() != "ERR wrong number of arguments for 'echo' command" {
		t.Fatalf("ECHO with no args: %v", err)
	}
	// The connection is still fine after errors.
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatalf("PING after errors: %v", err)
	}
}

func TestCommandDocs(t *testing.T) {
	ctx := testContext(t)
	rdb := newClient(t, startServer(t))

	got, err := rdb.Do(ctx, "COMMAND", "DOCS").Slice()
	if err != nil || len(got) != 0 {
		t.Fatalf("COMMAND DOCS: %v, %v", got, err)
	}
}

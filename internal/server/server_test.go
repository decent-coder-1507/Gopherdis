package server

import (
	"bufio"
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"gopherdis/internal/commands"
	"gopherdis/internal/store"
)

func startServer(t *testing.T) *Server {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := New(commands.NewTable(store.New(), commands.Env{}), Options{})
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ln) }()
	t.Cleanup(func() {
		srv.Close()
		if err := <-done; err != nil {
			t.Errorf("Serve returned %v", err)
		}
	})
	return srv
}

func dial(t *testing.T, srv *Server) (net.Conn, *bufio.Reader) {
	t.Helper()
	for srv.Addr() == nil {
		time.Sleep(time.Millisecond)
	}
	c, err := net.Dial("tcp", srv.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	c.SetDeadline(time.Now().Add(5 * time.Second))
	return c, bufio.NewReader(c)
}

func readLine(t *testing.T, r *bufio.Reader) string {
	t.Helper()
	line, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("reading reply: %v", err)
	}
	return line
}

func TestRawPingAndUnknown(t *testing.T) {
	srv := startServer(t)
	c, r := dial(t, srv)

	io.WriteString(c, "*1\r\n$4\r\nPING\r\n")
	if got := readLine(t, r); got != "+PONG\r\n" {
		t.Fatalf("got %q", got)
	}
	// An unknown command gets an error and the connection stays usable.
	io.WriteString(c, "*1\r\n$3\r\nFOO\r\n")
	if got := readLine(t, r); got != "-ERR unknown command 'FOO'\r\n" {
		t.Fatalf("got %q", got)
	}
	io.WriteString(c, "*2\r\n$4\r\nECHO\r\n$2\r\nhi\r\n")
	if got := readLine(t, r) + readLine(t, r); got != "$2\r\nhi\r\n" {
		t.Fatalf("got %q", got)
	}
}

func TestCommandSplitAcrossWrites(t *testing.T) {
	srv := startServer(t)
	c, r := dial(t, srv)
	for _, part := range []string{"*1\r", "\n$4\r\nPI", "NG", "\r\n"} {
		io.WriteString(c, part)
		time.Sleep(5 * time.Millisecond)
	}
	if got := readLine(t, r); got != "+PONG\r\n" {
		t.Fatalf("got %q", got)
	}
}

func TestBrokenClientDoesNotStopServer(t *testing.T) {
	srv := startServer(t)

	// One client sends garbage, another leaves in the middle of a command.
	bad, _ := dial(t, srv)
	io.WriteString(bad, "*abc\r\n")
	half, _ := dial(t, srv)
	io.WriteString(half, "*2\r\n$4\r\nECHO\r\n$10\r\nabc")
	half.Close()

	// The garbage client is told why and disconnected...
	got, _ := io.ReadAll(bad)
	if !strings.HasPrefix(string(got), "-ERR Protocol error") || !strings.HasSuffix(string(got), "\r\n") {
		t.Errorf("garbage client got %q, want a protocol error and then EOF", got)
	}
	// ...and everyone else is still served.
	c, r := dial(t, srv)
	io.WriteString(c, "*1\r\n$4\r\nPING\r\n")
	if got := readLine(t, r); got != "+PONG\r\n" {
		t.Fatalf("got %q", got)
	}
}

func TestCloseDisconnectsClients(t *testing.T) {
	srv := startServer(t)
	c, _ := dial(t, srv)
	srv.Close()
	buf := make([]byte, 1)
	if _, err := c.Read(buf); err == nil {
		t.Fatal("expected the connection to be closed")
	}
}

func TestShutdownWakesIdleClients(t *testing.T) {
	srv := startServer(t)
	idle, _ := dial(t, srv)
	busy, r := dial(t, srv)
	io.WriteString(busy, "*1\r\n$4\r\nPING\r\n")
	if got := readLine(t, r); got != "+PONG\r\n" {
		t.Fatal(got)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("Shutdown took %v; idle clients should be woken at once", time.Since(start))
	}
	for _, c := range []net.Conn{idle, busy} {
		if _, err := c.Read(make([]byte, 1)); err == nil {
			t.Error("connection still open after Shutdown")
		}
	}
	// No new clients.
	if c, err := net.DialTimeout("tcp", srv.Addr().String(), time.Second); err == nil {
		c.Close()
		t.Error("server still accepts clients after Shutdown")
	}
}

func TestShutdownFinishesSentCommands(t *testing.T) {
	srv := startServer(t)
	c, r := dial(t, srv)
	// Two commands already sent: both are answered before the connection closes.
	io.WriteString(c, "*2\r\n$4\r\nECHO\r\n$1\r\na\r\n*2\r\n$4\r\nECHO\r\n$1\r\nb\r\n")
	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	var got string
	for {
		line, err := r.ReadString('\n')
		got += line
		if err != nil {
			break
		}
	}
	if got != "$1\r\na\r\n$1\r\nb\r\n" {
		t.Fatalf("replies before close: %q", got)
	}
}

func TestShutdownCommandClosesConnection(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	requested := make(chan bool, 1)
	srv := New(commands.NewTable(store.New(), commands.Env{Shutdown: func(save bool) { requested <- save }}), Options{})
	go srv.Serve(ln)
	defer srv.Close()

	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	// SHUTDOWN, then a PING that must not run.
	io.WriteString(c, "*2\r\n$8\r\nSHUTDOWN\r\n$6\r\nNOSAVE\r\n*1\r\n$4\r\nPING\r\n")
	if save := <-requested; save {
		t.Error("NOSAVE was ignored")
	}
	data, _ := io.ReadAll(c)
	if len(data) != 0 {
		t.Fatalf("got %q after SHUTDOWN, want the connection closed with no reply", data)
	}
}

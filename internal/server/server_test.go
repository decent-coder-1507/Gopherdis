package server

import (
	"bufio"
	"io"
	"net"
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
	srv := New(commands.NewTable(store.New()))
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
	io.WriteString(bad, "?garbage\r\n")
	half, _ := dial(t, srv)
	io.WriteString(half, "*2\r\n$4\r\nECHO\r\n$10\r\nabc")
	half.Close()

	// The garbage client is disconnected...
	buf := make([]byte, 1)
	if _, err := bad.Read(buf); err == nil {
		t.Errorf("expected the garbage client to be disconnected")
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

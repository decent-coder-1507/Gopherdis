package server

import (
	"bufio"
	"io"
	"math/rand"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"gopherdis/internal/commands"
	"gopherdis/internal/store"
)

func startServerWithOptions(t *testing.T, opts Options) *Server {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := New(commands.NewTable(store.New(), commands.Env{}), opts)
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	for srv.Addr() == nil { // Serve has started once Addr is set
		time.Sleep(time.Millisecond)
	}
	return srv
}

func ping(t *testing.T, c net.Conn, r *bufio.Reader) {
	t.Helper()
	io.WriteString(c, "*1\r\n$4\r\nPING\r\n")
	if got := readLine(t, r); got != "+PONG\r\n" {
		t.Fatalf("PING: %q", got)
	}
}

func TestMaxClients(t *testing.T) {
	srv := startServerWithOptions(t, Options{MaxClients: 2})
	a, ra := dial(t, srv)
	ping(t, a, ra)
	b, rb := dial(t, srv)
	ping(t, b, rb)

	// The third client gets the error and is closed.
	c, _ := dial(t, srv)
	got, _ := io.ReadAll(c)
	if string(got) != "-ERR max number of clients reached\r\n" {
		t.Fatalf("third client got %q", got)
	}

	// The first two still work, and once one leaves there's room again.
	ping(t, a, ra)
	a.Close()
	deadline := time.Now().Add(2 * time.Second)
	for srv.ConnectedClients() > 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	d, rd := dial(t, srv)
	ping(t, d, rd)
}

func TestBulkTooBig(t *testing.T) {
	srv := startServerWithOptions(t, Options{MaxBulkLen: 100})
	c, r := dial(t, srv)
	// Only the header is sent. The server must refuse it without waiting
	// for 101 bytes of data.
	io.WriteString(c, "*2\r\n$4\r\nECHO\r\n$101\r\n")
	if got := readLine(t, r); got != "-ERR Protocol error: invalid bulk length\r\n" {
		t.Fatalf("got %q", got)
	}
	if _, err := r.ReadByte(); err != io.EOF {
		t.Fatalf("connection not closed: %v", err)
	}

	// Exactly at the limit is fine.
	c2, r2 := dial(t, srv)
	io.WriteString(c2, "*2\r\n$4\r\nECHO\r\n$100\r\n"+strings.Repeat("x", 100)+"\r\n")
	if got := readLine(t, r2); got != "$100\r\n" {
		t.Fatalf("100-byte value: %q", got)
	}
}

func TestTooManyParts(t *testing.T) {
	srv := startServerWithOptions(t, Options{MaxArrayLen: 3})
	c, r := dial(t, srv)
	io.WriteString(c, "*4\r\n")
	if got := readLine(t, r); got != "-ERR Protocol error: invalid multibulk length\r\n" {
		t.Fatalf("got %q", got)
	}
	if _, err := r.ReadByte(); err != io.EOF {
		t.Fatalf("connection not closed: %v", err)
	}
}

func TestInlineCommands(t *testing.T) {
	srv := startServerWithOptions(t, Options{})
	c, r := dial(t, srv)
	io.WriteString(c, "PING\r\n")
	if got := readLine(t, r); got != "+PONG\r\n" {
		t.Fatalf("inline PING: %q", got)
	}
	io.WriteString(c, "SET greeting hello\n")
	if got := readLine(t, r); got != "+OK\r\n" {
		t.Fatalf("inline SET: %q", got)
	}
	io.WriteString(c, "get greeting\r\n")
	if got := readLine(t, r) + readLine(t, r); got != "$5\r\nhello\r\n" {
		t.Fatalf("inline GET: %q", got)
	}
	io.WriteString(c, "\r\nNOPE\r\n") // a blank line is skipped
	if got := readLine(t, r); got != "-ERR unknown command 'NOPE'\r\n" {
		t.Fatalf("inline unknown command: %q", got)
	}
}

// Random bytes from many clients at once must never take the server down.
func TestGarbageNeverCrashes(t *testing.T) {
	srv := startServerWithOptions(t, Options{MaxBulkLen: 1 << 16, MaxArrayLen: 64})
	addr := srv.Addr().String()

	seeds := []string{"*", "$", "*1\r\n", "*1\r\n$", "*-1\r\n", "$-1\r\n", "*2\r\n$3\r\nGET\r\n", "\r\n", "*99999999999\r\n", "$-5\r\n", "PING\r\n"}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(int64(i)))
			c, err := net.Dial("tcp", addr)
			if err != nil {
				return
			}
			defer c.Close()
			c.SetDeadline(time.Now().Add(2 * time.Second))
			go io.Copy(io.Discard, c) // read whatever comes back so writes never block
			for j := 0; j < 200; j++ {
				var b []byte
				if rng.Intn(2) == 0 {
					b = []byte(seeds[rng.Intn(len(seeds))])
				}
				junk := make([]byte, rng.Intn(64))
				rng.Read(junk)
				b = append(b, junk...)
				if _, err := c.Write(b); err != nil {
					return // the server closed us; that's allowed
				}
			}
		}(i)
	}
	wg.Wait()

	c, r := dial(t, srv)
	ping(t, c, r)
}

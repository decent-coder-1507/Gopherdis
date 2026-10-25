package server

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"gopherdis/internal/commands"
	"gopherdis/internal/resp"
)

// Options are the server's limits. Zero values mean "no limit" for
// MaxClients and the RESP defaults (512 MB, 1,048,576 parts) for the sizes.
type Options struct {
	MaxClients  int
	MaxBulkLen  int64
	MaxArrayLen int64
}

// Server accepts clients and runs their commands against a command table.
type Server struct {
	cmds *commands.Table
	opts Options

	mu     sync.Mutex
	ln     net.Listener
	conns  map[net.Conn]struct{}
	closed bool
	wg     sync.WaitGroup
}

// New returns a server that runs commands from cmds.
func New(cmds *commands.Table, opts Options) *Server {
	return &Server{cmds: cmds, opts: opts, conns: make(map[net.Conn]struct{})}
}

// ListenAndServe listens on addr (for example ":6379") and serves clients
// until Close is called.
func (s *Server) ListenAndServe(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return s.Serve(ln)
}

// Serve accepts clients on ln until Close is called. Each client gets its
// own goroutine. Serve returns nil after Close.
func (s *Server) Serve(ln net.Listener) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		ln.Close()
		return nil
	}
	s.ln = ln
	s.mu.Unlock()

	var backoff time.Duration
	for {
		conn, err := ln.Accept()
		if err != nil {
			if s.isClosed() {
				return nil
			}
			// Errors like "too many open files" pass; wait a little and retry
			// instead of spinning or giving up.
			if isTemporary(err) {
				backoff = min(max(2*backoff, 5*time.Millisecond), time.Second)
				slog.Warn("accept failed; retrying", "err", err, "retry_in", backoff)
				time.Sleep(backoff)
				continue
			}
			return err
		}
		backoff = 0

		switch s.track(conn) {
		case closed:
			conn.Close()
			return nil
		case full:
			slog.Warn("max number of clients reached; refusing a client", "addr", conn.RemoteAddr().String(), "maxclients", s.opts.MaxClients)
			go refuse(conn, "ERR max number of clients reached")
			continue
		}
		go func() {
			defer s.wg.Done()
			defer s.untrack(conn)
			s.handle(conn)
		}()
	}
}

// refuse sends one error to a client we won't serve and closes it. A
// client that doesn't read gets cut off after a second.
func refuse(conn net.Conn, msg string) {
	conn.SetWriteDeadline(time.Now().Add(time.Second))
	io.WriteString(conn, "-"+msg+"\r\n")
	conn.Close()
}

func isTemporary(err error) bool {
	var t interface{ Temporary() bool }
	return errors.As(err, &t) && t.Temporary()
}

// Addr returns the address the server listens on, or nil before Serve.
func (s *Server) Addr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln == nil {
		return nil
	}
	return s.ln.Addr()
}

// ConnectedClients returns the number of open client connections.
func (s *Server) ConnectedClients() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.conns)
}

// Shutdown stops the server gently: it stops accepting clients, lets every
// client finish the command it is running (and any commands it already sent),
// then closes its connection. Idle clients are woken up by moving their read
// deadline to now. If ctx ends first, the remaining connections are closed
// at once and ctx.Err() is returned.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		if s.ln != nil {
			s.ln.Close()
		}
	}
	for c := range s.conns {
		c.SetReadDeadline(time.Now())
	}
	s.mu.Unlock()

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		s.mu.Lock()
		for c := range s.conns {
			c.Close()
		}
		s.mu.Unlock()
		<-done
		return ctx.Err()
	}
}

// Close stops accepting clients, closes every open connection and waits
// for the client goroutines to end.
func (s *Server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	var err error
	if s.ln != nil {
		err = s.ln.Close()
	}
	for c := range s.conns {
		c.Close()
	}
	s.mu.Unlock()
	s.wg.Wait()
	return err
}

func (s *Server) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

type trackResult int

const (
	accepted trackResult = iota
	closed
	full
)

// track registers a new connection and counts its goroutine in wg. Both
// happen under mu together with the closed check, so Close can't start
// waiting between them and miss a goroutine.
func (s *Server) track(c net.Conn) trackResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return closed
	}
	if s.opts.MaxClients > 0 && len(s.conns) >= s.opts.MaxClients {
		return full
	}
	s.conns[c] = struct{}{}
	s.wg.Add(1)
	return accepted
}

func (s *Server) untrack(c net.Conn) {
	s.mu.Lock()
	delete(s.conns, c)
	s.mu.Unlock()
	c.Close()
}

// handle runs the read-command, run-it, write-reply loop for one client.
// It returns when the client disconnects or sends something we can't parse;
// either way only this client is affected.
//
// Pipelining: if the client has already sent more commands (they are
// waiting in the read buffer), we run them before flushing, so a batch of
// commands gets its replies in one write, in order.
func (s *Server) handle(conn net.Conn) {
	addr := conn.RemoteAddr().String()
	slog.Debug("client connected", "addr", addr)
	defer slog.Debug("client disconnected", "addr", addr)

	r := resp.NewReader(bufio.NewReader(conn))
	r.MaxBulkLen, r.MaxArrayLen = s.opts.MaxBulkLen, s.opts.MaxArrayLen
	w := resp.NewWriter(conn)
	ctx := s.cmds.NewContext(w)
	for {
		args, err := r.ReadCommand()
		if err != nil {
			// Bytes that aren't valid RESP (or too big): tell the client
			// why, then close only this connection. The rest of what it
			// sent can't be trusted to line up with commands any more.
			var perr *resp.ProtocolError
			if errors.As(err, &perr) {
				slog.Warn("bad message from client; closing connection", "addr", addr, "err", err)
				w.WriteError("ERR Protocol error: " + perr.Detail)
				w.Flush()
			} else if errors.Is(err, io.ErrUnexpectedEOF) {
				slog.Debug("client left in the middle of a command", "addr", addr)
			}
			return
		}
		if len(args) == 0 {
			continue
		}
		s.cmds.Run(ctx, args)
		if ctx.CloseConn {
			w.Flush()
			return
		}
		if r.Buffered() > 0 {
			continue
		}
		if err := w.Flush(); err != nil {
			return
		}
	}
}

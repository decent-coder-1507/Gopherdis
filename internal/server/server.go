package server

import (
	"bufio"
	"errors"
	"log"
	"net"
	"sync"
	"time"

	"gopherdis/internal/commands"
	"gopherdis/internal/resp"
)

// Server accepts clients and runs their commands against a command table.
type Server struct {
	cmds *commands.Table

	mu     sync.Mutex
	ln     net.Listener
	conns  map[net.Conn]struct{}
	closed bool
	wg     sync.WaitGroup
}

// New returns a server that runs commands from cmds.
func New(cmds *commands.Table) *Server {
	return &Server{cmds: cmds, conns: make(map[net.Conn]struct{})}
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
				log.Printf("accept error: %v; retrying in %v", err, backoff)
				time.Sleep(backoff)
				continue
			}
			return err
		}
		backoff = 0

		if !s.track(conn) {
			conn.Close()
			return nil
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer s.untrack(conn)
			s.handle(conn)
		}()
	}
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

func (s *Server) track(c net.Conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.conns[c] = struct{}{}
	return true
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
	r := resp.NewReader(bufio.NewReader(conn))
	w := resp.NewWriter(conn)
	ctx := s.cmds.NewContext(w)
	for {
		args, err := r.ReadCommand()
		if err != nil {
			return
		}
		if len(args) == 0 {
			continue
		}
		s.cmds.Run(ctx, args)
		if r.Buffered() > 0 {
			continue
		}
		if err := w.Flush(); err != nil {
			return
		}
	}
}

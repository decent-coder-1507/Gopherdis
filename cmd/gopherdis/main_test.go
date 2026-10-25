package main

import (
	"bufio"
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// freePort returns a TCP port that was free a moment ago.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// started is a server running through run(), as main runs it.
type started struct {
	addr    string
	signals chan os.Signal
	done    chan error
}

func startRun(t *testing.T, dir string, extra ...string) *started {
	t.Helper()
	args := append([]string{"--port", strconv.Itoa(freePort(t)), "--dir", dir}, extra...)
	cfg, err := parseFlags(args, os.Stderr)
	if err != nil {
		t.Fatal(err)
	}
	s := &started{signals: make(chan os.Signal, 1), done: make(chan error, 1)}
	ready := make(chan string, 1)
	go func() { s.done <- run(cfg, s.signals, ready) }()
	select {
	case s.addr = <-ready:
	case err := <-s.done:
		t.Fatalf("run ended early: %v", err)
	}
	return s
}

func (s *started) wait(t *testing.T) error {
	t.Helper()
	select {
	case err := <-s.done:
		return err
	case <-time.After(15 * time.Second):
		t.Fatal("server did not shut down")
		return nil
	}
}

func client(t *testing.T, addr string) *redis.Client {
	c := redis.NewClient(&redis.Options{Addr: addr, MaxRetries: -1})
	t.Cleanup(func() { c.Close() })
	return c
}

func TestShutdownCommandSaves(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	s := startRun(t, dir, "--save-interval", "0")
	rdb := client(t, s.addr)
	rdb.Set(ctx, "x", "1", 0)
	rdb.Shutdown(ctx) // no reply; the connection just closes
	if err := s.wait(t); err != nil {
		t.Fatalf("run returned %v", err)
	}

	// Start again on the same folder: the key is there without any SAVE.
	s = startRun(t, dir)
	rdb = client(t, s.addr)
	if got, err := rdb.Get(ctx, "x").Result(); err != nil || got != "1" {
		t.Fatalf("after restart: %q, %v", got, err)
	}
	s.signals <- os.Interrupt
	s.wait(t)
}

func TestShutdownNoSave(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	s := startRun(t, dir, "--save-interval", "0")
	rdb := client(t, s.addr)
	rdb.Set(ctx, "x", "1", 0)
	rdb.ShutdownNoSave(ctx)
	if err := s.wait(t); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "dump.gob")); err == nil {
		t.Fatal("SHUTDOWN NOSAVE wrote a dump file")
	}
}

func TestSignalSaves(t *testing.T) {
	for _, sig := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			dir := t.TempDir()
			ctx := context.Background()
			s := startRun(t, dir, "--save-interval", "0")
			rdb := client(t, s.addr)
			rdb.Set(ctx, "x", "1", 0)
			rdb.RPush(ctx, "l", "a", "b")

			s.signals <- sig
			if err := s.wait(t); err != nil {
				t.Fatal(err)
			}
			s = startRun(t, dir)
			rdb = client(t, s.addr)
			if n, _ := rdb.Exists(ctx, "x", "l").Result(); n != 2 {
				t.Fatalf("after restart %d of 2 keys exist", n)
			}
			s.signals <- os.Interrupt
			s.wait(t)
		})
	}
}

func TestAutoSaveRunsOnlyAfterChanges(t *testing.T) {
	dir := t.TempDir()
	dump := filepath.Join(dir, "dump.gob")
	ctx := context.Background()
	s := startRun(t, dir, "--save-interval", "100ms")
	defer func() {
		s.signals <- os.Interrupt
		s.wait(t)
	}()
	rdb := client(t, s.addr)

	time.Sleep(350 * time.Millisecond)
	if _, err := os.Stat(dump); err == nil {
		t.Fatal("auto-save wrote a file although nothing changed")
	}
	before, _ := rdb.LastSave(ctx).Result()

	rdb.Set(ctx, "x", "1", 0)
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(dump); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("auto-save never saved the change")
		}
		time.Sleep(20 * time.Millisecond)
	}
	// LASTSAVE moves when the save lands (it has 1-second resolution, so
	// it can only be checked not to go backwards here).
	if after, _ := rdb.LastSave(ctx).Result(); after < before {
		t.Errorf("LASTSAVE went from %d to %d", before, after)
	}
}

func TestServerKeepsAnsweringDuringBackgroundSave(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	s := startRun(t, dir, "--save-interval", "0")
	defer func() {
		s.signals <- os.Interrupt
		s.wait(t)
	}()
	rdb := client(t, s.addr)

	// Enough data that the save takes a noticeable time.
	pipe := rdb.Pipeline()
	value := strings.Repeat("v", 1000)
	for i := 0; i < 50000; i++ {
		pipe.Set(ctx, "key:"+strconv.Itoa(i), value, 0)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		t.Fatal(err)
	}

	if got, err := rdb.BgSave(ctx).Result(); err != nil || got != "Background saving started" {
		t.Fatalf("BGSAVE: %q, %v", got, err)
	}
	// Right away, while the save runs: commands work, and a second BGSAVE is refused.
	if err := rdb.Set(ctx, "during", "save", 0).Err(); err != nil {
		t.Fatalf("SET during BGSAVE: %v", err)
	}
	if err := rdb.BgSave(ctx).Err(); err == nil || !strings.Contains(err.Error(), "already in progress") {
		t.Fatalf("second BGSAVE: %v", err)
	}
}

// The real binary, stopped with real signals.
func TestProcessSignals(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("signals can't be sent to a process on Windows")
	}
	if testing.Short() {
		t.Skip("builds the binary")
	}
	bin := filepath.Join(t.TempDir(), "gopherdis")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			dir := t.TempDir()
			port := strconv.Itoa(freePort(t))
			cmd := exec.Command(bin, "--port", port, "--dir", dir, "--save-interval", "0")
			stderr, _ := cmd.StderrPipe()
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			logs := bufio.NewScanner(stderr)
			waitForLog(t, logs, "server started")

			ctx := context.Background()
			rdb := client(t, "127.0.0.1:"+port)
			if err := rdb.Set(ctx, "x", "1", 0).Err(); err != nil {
				t.Fatal(err)
			}
			cmd.Process.Signal(sig)
			waitForLog(t, logs, "shutdown complete")
			if err := cmd.Wait(); err != nil {
				t.Fatalf("exit: %v (want code 0)", err)
			}
			if _, err := os.Stat(filepath.Join(dir, "dump.gob")); err != nil {
				t.Fatalf("no dump file after %v: %v", sig, err)
			}
		})
	}
}

func waitForLog(t *testing.T, logs *bufio.Scanner, want string) {
	t.Helper()
	for logs.Scan() {
		if strings.Contains(logs.Text(), want) {
			return
		}
	}
	t.Fatalf("log never said %q", want)
}

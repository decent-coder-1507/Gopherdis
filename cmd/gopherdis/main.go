// Command gopherdis runs a small Redis-compatible server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gopherdis/internal/commands"
	"gopherdis/internal/persist"
	"gopherdis/internal/server"
	"gopherdis/internal/store"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

// shutdownTimeout is the longest a shutdown may take before we give up
// waiting and exit anyway. Clients get half of it to finish their commands,
// so the save still has time to run.
const (
	shutdownTimeout = 10 * time.Second
	clientsTimeout  = shutdownTimeout / 2
)

func main() {
	cfg, err := parseFlags(os.Args[1:], os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		os.Exit(0)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "gopherdis:", err)
		os.Exit(2)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevels[cfg.loglevel]})))

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	if err := run(cfg, signals, nil); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

// run starts the server and blocks until it has shut down, either because
// a signal arrived on signals or a client sent SHUTDOWN. If ready is not
// nil, the listening address is sent on it once clients can connect.
func run(cfg *config, signals <-chan os.Signal, ready chan<- string) error {
	st := store.New()
	saver := persist.NewSaver(cfg.dumpPath(), st)

	// Load saved data before accepting clients. A damaged file stops the
	// server: starting empty would quietly lose the data.
	start := time.Now()
	keys, found, err := saver.Load()
	if err != nil {
		return fmt.Errorf("cannot load saved data: %w", err)
	}
	if found {
		slog.Info("data loaded", "file", cfg.dumpPath(), "keys", keys, "took", time.Since(start).Round(time.Microsecond))
	}

	ln, err := net.Listen("tcp", cfg.addr())
	if err != nil {
		return err
	}

	// SHUTDOWN sends its save choice here. The channel holds one request;
	// a second SHUTDOWN while one is pending changes nothing.
	shutdownRequests := make(chan bool, 1)

	var srv *server.Server
	cmds := commands.NewTable(st, commands.Env{
		Saver:            saver,
		Version:          version,
		Config:           cfg.params(),
		ConnectedClients: func() int { return srv.ConnectedClients() },
		StartTime:        start,
		Shutdown: func(save bool) {
			select {
			case shutdownRequests <- save:
			default:
			}
		},
	})
	srv = server.New(cmds)

	stopCleanup := st.StartCleanup()
	stopAutoSave := func() {}
	if cfg.saveInterval > 0 {
		stopAutoSave = saver.StartAutoSave(cfg.saveInterval)
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()
	slog.Info("server started", "address", cfg.bind, "port", ln.Addr().(*net.TCPAddr).Port, "version", version)
	if ready != nil {
		ready <- ln.Addr().String()
	}

	save := true
	select {
	case sig := <-signals:
		// A second Ctrl+C now stops the process at once, the usual escape
		// hatch if a shutdown hangs.
		signal.Reset(os.Interrupt, syscall.SIGTERM)
		slog.Info("shutting down", "reason", "signal "+sig.String())
	case save = <-shutdownRequests:
		slog.Info("shutting down", "reason", "SHUTDOWN command", "save", save)
	case err := <-serveErr:
		stopAutoSave()
		stopCleanup()
		return fmt.Errorf("server stopped: %w", err)
	}

	// From here on, give up after shutdownTimeout. The dump file is replaced
	// atomically, so exiting in the middle of a save leaves the old one intact.
	watchdog := time.AfterFunc(shutdownTimeout, func() {
		slog.Warn("shutdown took longer than " + shutdownTimeout.String() + "; exiting anyway")
		os.Exit(1)
	})
	defer watchdog.Stop()
	return shutdown(srv, saver, save, func() { stopAutoSave(); stopCleanup() })
}

// shutdown stops accepting clients, lets running commands finish, stops the
// background jobs, saves (if save is true) and reports.
func shutdown(srv *server.Server, saver *persist.Saver, save bool, stopJobs func()) error {
	ctx, cancel := context.WithTimeout(context.Background(), clientsTimeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Warn("clients did not finish in time; their connections were closed", "err", err)
	}
	stopJobs()

	if save {
		if _, err := saver.SaveWait(); err != nil {
			return fmt.Errorf("could not save during shutdown: %w", err)
		}
	}
	slog.Info("shutdown complete")
	return nil
}

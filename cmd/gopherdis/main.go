// Command gopherdis runs a small Redis-compatible server.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"gopherdis/internal/commands"
	"gopherdis/internal/persist"
	"gopherdis/internal/server"
	"gopherdis/internal/store"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

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

	if err := run(cfg); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func run(cfg *config) error {
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

	stopCleanup := st.StartCleanup()
	defer stopCleanup()

	var srv *server.Server
	cmds := commands.NewTable(st, commands.Env{
		Saver:            saver,
		Version:          version,
		Config:           cfg.params(),
		ConnectedClients: func() int { return srv.ConnectedClients() },
		StartTime:        start,
	})
	srv = server.New(cmds)

	slog.Info("server started", "address", cfg.bind, "port", cfg.port, "version", version)
	return srv.ListenAndServe(cfg.addr())
}

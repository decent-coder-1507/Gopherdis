package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopherdis/internal/commands"
)

// config holds every setting. Each has a default, so the server runs with
// no flags at all.
type config struct {
	port         int
	bind         string
	dir          string
	dbfilename   string
	saveInterval time.Duration
	requirepass  string
	maxclients   int
	loglevel     string
	// showVersion is --version: print the version and exit.
	showVersion bool
}

// parseFlags reads the settings from args (without the program name).
// It returns flag.ErrHelp after printing the help text for --help.
func parseFlags(args []string, output io.Writer) (*config, error) {
	c := &config{}
	fs := flag.NewFlagSet("gopherdis", flag.ContinueOnError)
	fs.SetOutput(output)
	fs.IntVar(&c.port, "port", 6379, "TCP port to listen on")
	fs.StringVar(&c.bind, "bind", "127.0.0.1", "address to listen on; the default accepts connections from this computer only")
	fs.StringVar(&c.dir, "dir", ".", "folder for the save file")
	fs.StringVar(&c.dbfilename, "dbfilename", "dump.gob", "save file name, inside --dir")
	fs.DurationVar(&c.saveInterval, "save-interval", 60*time.Second, "how often to save if data changed, e.g. 60s or 5m; 0 turns it off")
	fs.StringVar(&c.requirepass, "requirepass", "", "password clients must send with AUTH; empty means no password")
	fs.IntVar(&c.maxclients, "maxclients", 10000, "most clients connected at once")
	fs.StringVar(&c.loglevel, "loglevel", "info", "log detail: debug, info, warn or error")
	fs.BoolVar(&c.showVersion, "version", false, "print the version and exit")
	fs.Usage = func() { printUsage(fs) }

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() > 0 {
		return nil, fmt.Errorf("unexpected argument %q (settings are flags, see --help)", fs.Arg(0))
	}
	if c.showVersion {
		return c, nil // nothing else matters
	}
	return c, c.validate()
}

// printUsage prints one line per flag, in the --name form people type.
func printUsage(fs *flag.FlagSet) {
	out := fs.Output()
	fmt.Fprintf(out, "Usage: gopherdis [flags]\n\nA small Redis-compatible server.\n\nFlags:\n")
	fs.VisitAll(func(f *flag.Flag) {
		if f.Name == "version" {
			fmt.Fprintf(out, "  --%-14s %s\n", f.Name, f.Usage)
			return
		}
		def := f.DefValue
		if def == "" {
			def = `""`
		}
		fmt.Fprintf(out, "  --%-14s %s (default %s)\n", f.Name, f.Usage, def)
	})
}

func (c *config) validate() error {
	var problems []string
	if c.port < 1 || c.port > 65535 {
		problems = append(problems, "--port must be between 1 and 65535")
	}
	if c.bind != "" && net.ParseIP(c.bind) == nil && c.bind != "localhost" {
		problems = append(problems, "--bind must be an IP address such as 127.0.0.1 or 0.0.0.0")
	}
	if c.dbfilename == "" || c.dbfilename != filepath.Base(c.dbfilename) {
		problems = append(problems, "--dbfilename must be a plain file name; use --dir for the folder")
	}
	if c.saveInterval < 0 {
		problems = append(problems, "--save-interval can't be negative")
	}
	if c.maxclients < 1 {
		problems = append(problems, "--maxclients must be at least 1")
	}
	if _, ok := logLevels[c.loglevel]; !ok {
		problems = append(problems, "--loglevel must be debug, info, warn or error")
	}
	if info, err := os.Stat(c.dir); err != nil || !info.IsDir() {
		problems = append(problems, fmt.Sprintf("--dir %q is not an existing folder", c.dir))
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

var logLevels = map[string]slog.Level{
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"warn":  slog.LevelWarn,
	"error": slog.LevelError,
}

// addr is the address to listen on, e.g. "127.0.0.1:6379".
func (c *config) addr() string {
	return net.JoinHostPort(c.bind, strconv.Itoa(c.port))
}

// dumpPath is the full path of the save file.
func (c *config) dumpPath() string {
	return filepath.Join(c.dir, c.dbfilename)
}

// params lists the settings for CONFIG GET, under the flag names. "save"
// is also given in Redis' own "seconds changes" form for tools that ask.
func (c *config) params() []commands.Param {
	save := ""
	if c.saveInterval > 0 {
		save = strconv.Itoa(int(c.saveInterval.Seconds())) + " 1"
	}
	return []commands.Param{
		{Name: "port", Value: strconv.Itoa(c.port)},
		{Name: "bind", Value: c.bind},
		{Name: "dir", Value: c.dir},
		{Name: "dbfilename", Value: c.dbfilename},
		{Name: "save-interval", Value: c.saveInterval.String()},
		{Name: "save", Value: save},
		{Name: "appendonly", Value: "no"},
		{Name: "requirepass", Value: c.requirepass},
		{Name: "maxclients", Value: strconv.Itoa(c.maxclients)},
		{Name: "loglevel", Value: c.loglevel},
	}
}

package commands

import (
	"strings"
	"sync/atomic"
	"time"

	"gopherdis/internal/persist"
	"gopherdis/internal/resp"
	"gopherdis/internal/store"
)

// Env holds what commands need from outside the store: the saver, the
// settings and facts about the running server. Zero fields switch the
// matching feature off, which keeps tests short.
type Env struct {
	// Saver writes the database to disk. Nil means saving is off.
	Saver *persist.Saver
	// Version is reported by INFO.
	Version string
	// Config is what CONFIG GET reports, in this order.
	Config []Param
	// ConnectedClients returns the number of open client connections.
	ConnectedClients func() int
	// StartTime is when the server started, for INFO's uptime.
	StartTime time.Time
}

// Param is one setting reported by CONFIG GET.
type Param struct {
	Name, Value string
}

// Context is what a handler gets for one command call. The server keeps one
// Context per connection and reuses it for every command on it.
type Context struct {
	// Args holds the whole command, name included: for "ECHO hi" it is
	// ["ECHO", "hi"]. The slices are only valid during the call.
	Args [][]byte
	// Out is where the handler writes its reply. The caller flushes it.
	Out *resp.Writer
	// Store is the database.
	Store *store.Store
	// Env is the server around the store. It is shared; don't change it.
	Env *Env

	table *Table
}

// Handler runs one command and writes exactly one reply to ctx.Out.
type Handler func(ctx *Context)

// Command describes one command in the table.
type Command struct {
	Name string
	// Arity counts the name too, like in Redis: 2 means exactly one
	// argument, -2 means at least one argument.
	Arity   int
	Handler Handler
}

// Table maps upper-case command names to commands.
type Table struct {
	cmds  map[string]Command
	store *store.Store
	env   Env
	// processed counts commands run (found and with the right argument count).
	processed atomic.Int64
}

// NewTable returns a table with every built-in command registered,
// working on st.
func NewTable(st *store.Store, env Env) *Table {
	if env.StartTime.IsZero() {
		env.StartTime = time.Now()
	}
	t := &Table{cmds: make(map[string]Command), store: st, env: env}
	t.add("PING", -1, ping)
	t.add("ECHO", 2, echo)
	t.add("COMMAND", -1, command)
	t.add("SET", -3, set)
	t.add("GET", 2, get)
	t.add("CONFIG", -2, config)
	t.add("EXISTS", -2, exists)
	t.add("DEL", -2, del)
	t.add("INCR", 2, incr)
	t.add("DECR", 2, decr)
	t.add("LPUSH", -3, lpush)
	t.add("RPUSH", -3, rpush)
	t.add("SAVE", 1, save)
	t.add("TTL", 2, ttl)
	t.add("PTTL", 2, pttl)
	t.add("EXPIRE", 3, expire)
	t.add("PERSIST", 2, persistKey)
	t.add("KEYS", 2, keys)
	t.add("LRANGE", 4, lrange)
	t.add("LLEN", 2, llen)
	t.add("MGET", -2, mget)
	t.add("DBSIZE", 1, dbsize)
	t.add("FLUSHALL", -1, flushall)
	t.add("INFO", -1, info)
	return t
}

func (t *Table) add(name string, arity int, h Handler) {
	t.cmds[name] = Command{Name: name, Arity: arity, Handler: h}
}

// NewContext returns the Context for one client connection whose replies
// go to out.
func (t *Table) NewContext(out *resp.Writer) *Context {
	return &Context{Out: out, Store: t.store, Env: &t.env, table: t}
}

// Run looks up the command named by args[0], checks its argument count
// and runs it with ctx. args must not be empty.
func (t *Table) Run(ctx *Context, args [][]byte) {
	cmd, ok := t.lookup(args[0])
	if !ok {
		ctx.Out.WriteError("ERR unknown command '" + string(args[0]) + "'")
		return
	}
	if (cmd.Arity > 0 && len(args) != cmd.Arity) || (cmd.Arity < 0 && len(args) < -cmd.Arity) {
		WrongArgs(ctx.Out, cmd.Name)
		return
	}
	ctx.Args = args
	cmd.Handler(ctx)
	ctx.Args = nil
	t.processed.Add(1) // after the call, as Redis counts, so INFO doesn't count itself
}

// lookup finds a command by name, ignoring case, without allocating: this
// runs for every command, so strings.ToUpper's allocation adds up.
func (t *Table) lookup(name []byte) (Command, bool) {
	// Most clients send upper-case names, and string(name) in a map index
	// doesn't allocate.
	if cmd, ok := t.cmds[string(name)]; ok {
		return cmd, true
	}
	var upper [16]byte
	if len(name) > len(upper) {
		return Command{}, false
	}
	for i, c := range name {
		if 'a' <= c && c <= 'z' {
			c -= 'a' - 'A'
		}
		upper[i] = c
	}
	cmd, ok := t.cmds[string(upper[:len(name)])]
	return cmd, ok
}

// notIntegerMsg is the reply for an argument that should be a whole number but isn't.
const notIntegerMsg = "ERR value is not an integer or out of range"

// writeErr writes err as an error reply. Store errors already carry their
// prefix (ERR, WRONGTYPE, ...).
func writeErr(out *resp.Writer, err error) {
	out.WriteError(err.Error())
}

// WrongArgs writes the standard wrong-number-of-arguments error.
func WrongArgs(out *resp.Writer, name string) {
	out.WriteError("ERR wrong number of arguments for '" + strings.ToLower(name) + "' command")
}

// PING [message]
func ping(ctx *Context) {
	switch len(ctx.Args) {
	case 1:
		ctx.Out.WriteSimple("PONG")
	case 2:
		ctx.Out.WriteBulk(ctx.Args[1])
	default:
		WrongArgs(ctx.Out, "ping")
	}
}

// ECHO message
func echo(ctx *Context) {
	ctx.Out.WriteBulk(ctx.Args[1])
}

// COMMAND [subcommand ...]
//
// redis-cli 7+ sends COMMAND DOCS when it starts in interactive mode and
// waits for the answer, so we reply with an empty array to anything.
func command(ctx *Context) {
	ctx.Out.WriteArray(0)
}

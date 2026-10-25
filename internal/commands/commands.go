package commands

import (
	"strings"

	"gopherdis/internal/persist"
	"gopherdis/internal/resp"
	"gopherdis/internal/store"
)

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
	// Saver writes the database to disk. It is nil when saving is off.
	Saver *persist.Saver
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
	saver *persist.Saver
}

// NewTable returns a table with every built-in command registered,
// working on st.
func NewTable(st *store.Store) *Table {
	t := &Table{cmds: make(map[string]Command), store: st}
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
	return t
}

// SetSaver sets the saver used by SAVE. Without one, SAVE replies with an error.
func (t *Table) SetSaver(s *persist.Saver) {
	t.saver = s
}

func (t *Table) add(name string, arity int, h Handler) {
	t.cmds[name] = Command{Name: name, Arity: arity, Handler: h}
}

// NewContext returns the Context for one client connection whose replies
// go to out.
func (t *Table) NewContext(out *resp.Writer) *Context {
	return &Context{Out: out, Store: t.store, Saver: t.saver}
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

// configValues are the settings CONFIG GET can report. redis-benchmark asks
// for "save" and "appendonly" when it starts and prints a warning unless both
// come back as [name, value] pairs. We save nothing automatically yet and
// have no append-only file.
var configValues = map[string]string{
	"save":       "",
	"appendonly": "no",
}

// CONFIG GET parameter [parameter ...]
//
// Each known name adds a [name, value] pair to the reply; unknown names add
// nothing, so a request for only unknown names gets an empty array.
func config(ctx *Context) {
	sub := strings.ToUpper(string(ctx.Args[1]))
	if sub != "GET" {
		ctx.Out.WriteError("ERR unknown subcommand '" + string(ctx.Args[1]) + "'")
		return
	}
	if len(ctx.Args) < 3 {
		WrongArgs(ctx.Out, "config|get")
		return
	}
	var pairs []string
	for _, arg := range ctx.Args[2:] {
		name := strings.ToLower(string(arg))
		if val, ok := configValues[name]; ok {
			pairs = append(pairs, name, val)
		}
	}
	ctx.Out.WriteArray(len(pairs))
	for _, p := range pairs {
		ctx.Out.WriteBulkString(p)
	}
}

package commands

import (
	"strings"

	"gopherdis/internal/resp"
	"gopherdis/internal/store"
)

// Context is what a handler gets for one command call.
type Context struct {
	// Args holds the whole command, name included: for "ECHO hi" it is
	// ["ECHO", "hi"]. The slices are only valid during the call.
	Args [][]byte
	// Out is where the handler writes its reply. The caller flushes it.
	Out *resp.Writer
	// Store is the database.
	Store *store.Store
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
	return t
}

func (t *Table) add(name string, arity int, h Handler) {
	t.cmds[name] = Command{Name: name, Arity: arity, Handler: h}
}

// Run looks up the command named by args[0], checks its argument count
// and runs it. args must not be empty.
func (t *Table) Run(args [][]byte, out *resp.Writer) {
	name := string(args[0])
	cmd, ok := t.cmds[strings.ToUpper(name)]
	if !ok {
		out.WriteError("ERR unknown command '" + name + "'")
		return
	}
	if (cmd.Arity > 0 && len(args) != cmd.Arity) || (cmd.Arity < 0 && len(args) < -cmd.Arity) {
		WrongArgs(out, cmd.Name)
		return
	}
	cmd.Handler(&Context{Args: args, Out: out, Store: t.store})
}

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

package commands

import (
	"fmt"
	"strings"
	"time"
)

// alwaysReported are added to CONFIG GET when the server doesn't set them.
// redis-benchmark asks for "save" and "appendonly" when it starts and prints
// a warning unless both come back as [name, value] pairs.
var alwaysReported = []Param{
	{"save", ""},
	{"appendonly", "no"},
}

// params returns the settings CONFIG GET can report.
func (e *Env) params() []Param {
	out := append([]Param(nil), e.Config...)
	for _, p := range alwaysReported {
		found := false
		for _, have := range out {
			if have.Name == p.Name {
				found = true
				break
			}
		}
		if !found {
			out = append(out, p)
		}
	}
	return out
}

// CONFIG GET pattern [pattern ...]
//
// Every setting whose name matches one of the glob patterns adds a
// [name, value] pair to the reply, each setting at most once. Names that
// match nothing add nothing, so CONFIG GET of an unknown name gets an
// empty array.
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
	for _, p := range ctx.Env.params() {
		for _, arg := range ctx.Args[2:] {
			if globMatch(strings.ToLower(string(arg)), p.Name) {
				pairs = append(pairs, p.Name, p.Value)
				break
			}
		}
	}
	ctx.Out.WriteArray(len(pairs))
	for _, s := range pairs {
		ctx.Out.WriteBulkString(s)
	}
}

// INFO [section ...]
//
// Replies with simple "name:value" lines. Sections are accepted for
// compatibility but everything is always returned; there isn't much.
func info(ctx *Context) {
	env := ctx.Env
	clients := 0
	if env.ConnectedClients != nil {
		clients = env.ConnectedClients()
	}
	var lastSave int64
	if env.Saver != nil {
		lastSave = env.Saver.LastSave().Unix()
	}

	var b strings.Builder
	b.WriteString("# Server\r\n")
	fmt.Fprintf(&b, "gopherdis_version:%s\r\n", env.Version)
	fmt.Fprintf(&b, "uptime_in_seconds:%d\r\n", int64(time.Since(env.StartTime).Seconds()))
	b.WriteString("\r\n# Clients\r\n")
	fmt.Fprintf(&b, "connected_clients:%d\r\n", clients)
	b.WriteString("\r\n# Stats\r\n")
	fmt.Fprintf(&b, "total_commands_processed:%d\r\n", ctx.table.processed.Load())
	b.WriteString("\r\n# Keyspace\r\n")
	fmt.Fprintf(&b, "keys:%d\r\n", ctx.Store.DBSize())
	b.WriteString("\r\n# Persistence\r\n")
	fmt.Fprintf(&b, "last_save_time:%d\r\n", lastSave)
	ctx.Out.WriteBulkString(b.String())
}

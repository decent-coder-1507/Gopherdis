package commands

import "strings"

// keyArgs returns the arguments from index 1 on as strings.
func keyArgs(args [][]byte) []string {
	keys := make([]string, len(args)-1)
	for i, a := range args[1:] {
		keys[i] = string(a)
	}
	return keys
}

// EXISTS key [key ...]
func exists(ctx *Context) {
	ctx.Out.WriteInteger(int64(ctx.Store.Exists(keyArgs(ctx.Args)...)))
}

// DEL key [key ...]
func del(ctx *Context) {
	ctx.Out.WriteInteger(int64(ctx.Store.Del(keyArgs(ctx.Args)...)))
}

// INCR key
func incr(ctx *Context) { incrBy(ctx, 1) }

// DECR key
func decr(ctx *Context) { incrBy(ctx, -1) }

func incrBy(ctx *Context, delta int64) {
	n, err := ctx.Store.IncrBy(string(ctx.Args[1]), delta)
	if err != nil {
		writeErr(ctx.Out, err)
		return
	}
	ctx.Out.WriteInteger(n)
}

// KEYS pattern
func keys(ctx *Context) {
	pattern := string(ctx.Args[1])
	var names []string
	if pattern == "*" {
		names = ctx.Store.Keys(func(string) bool { return true })
	} else {
		names = ctx.Store.Keys(func(k string) bool { return globMatch(pattern, k) })
	}
	ctx.Out.WriteArray(len(names))
	for _, k := range names {
		ctx.Out.WriteBulkString(k)
	}
}

// DBSIZE
func dbsize(ctx *Context) {
	ctx.Out.WriteInteger(int64(ctx.Store.DBSize()))
}

// FLUSHALL [ASYNC | SYNC]
//
// Both modes do the same thing here: the old maps are simply dropped.
func flushall(ctx *Context) {
	if len(ctx.Args) > 2 {
		ctx.Out.WriteError("ERR syntax error")
		return
	}
	if len(ctx.Args) == 2 {
		switch strings.ToUpper(string(ctx.Args[1])) {
		case "ASYNC", "SYNC":
		default:
			ctx.Out.WriteError("ERR syntax error")
			return
		}
	}
	ctx.Store.Flush()
	ctx.Out.WriteSimple("OK")
}

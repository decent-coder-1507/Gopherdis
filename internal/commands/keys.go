package commands

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

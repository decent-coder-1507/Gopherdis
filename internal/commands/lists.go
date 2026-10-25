package commands

// LPUSH key value [value ...]
func lpush(ctx *Context) { push(ctx, true) }

// RPUSH key value [value ...]
func rpush(ctx *Context) { push(ctx, false) }

func push(ctx *Context, front bool) {
	n, err := ctx.Store.Push(string(ctx.Args[1]), ctx.Args[2:], front)
	if err != nil {
		writeErr(ctx.Out, err)
		return
	}
	ctx.Out.WriteInteger(int64(n))
}

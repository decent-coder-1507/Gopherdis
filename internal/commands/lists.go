package commands

import "strconv"

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

// LRANGE key start stop
func lrange(ctx *Context) {
	start, err1 := strconv.ParseInt(string(ctx.Args[2]), 10, 64)
	stop, err2 := strconv.ParseInt(string(ctx.Args[3]), 10, 64)
	if err1 != nil || err2 != nil {
		ctx.Out.WriteError(notIntegerMsg)
		return
	}
	items, err := ctx.Store.LRange(string(ctx.Args[1]), start, stop)
	if err != nil {
		writeErr(ctx.Out, err)
		return
	}
	ctx.Out.WriteArray(len(items))
	for _, it := range items {
		ctx.Out.WriteBulk(it)
	}
}

// LLEN key
func llen(ctx *Context) {
	n, err := ctx.Store.LLen(string(ctx.Args[1]))
	if err != nil {
		writeErr(ctx.Out, err)
		return
	}
	ctx.Out.WriteInteger(int64(n))
}

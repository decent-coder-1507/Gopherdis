package commands

// SET key value
func set(ctx *Context) {
	if len(ctx.Args) != 3 {
		ctx.Out.WriteError("ERR syntax error")
		return
	}
	ctx.Store.Set(string(ctx.Args[1]), ctx.Args[2])
	ctx.Out.WriteSimple("OK")
}

// GET key
func get(ctx *Context) {
	val, ok, err := ctx.Store.Get(string(ctx.Args[1]))
	switch {
	case err != nil:
		writeErr(ctx.Out, err)
	case !ok:
		ctx.Out.WriteNull()
	default:
		ctx.Out.WriteBulk(val)
	}
}

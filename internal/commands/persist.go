package commands

// SAVE
//
// Writes the whole database to disk before replying. Other clients keep
// running commands while the file is written.
func save(ctx *Context) {
	saver := ctx.Env.Saver
	if saver == nil {
		ctx.Out.WriteError("ERR saving is not configured")
		return
	}
	if _, err := saver.Save(); err != nil {
		ctx.Out.WriteError("ERR save failed: " + err.Error())
		return
	}
	ctx.Out.WriteSimple("OK")
}

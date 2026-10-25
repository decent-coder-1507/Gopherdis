package commands

import "log"

// SAVE
//
// Writes the whole database to disk before replying. Other clients keep
// running commands while the file is written.
func save(ctx *Context) {
	if ctx.Saver == nil {
		ctx.Out.WriteError("ERR saving is not configured")
		return
	}
	if _, err := ctx.Saver.Save(); err != nil {
		log.Printf("save to %s failed: %v", ctx.Saver.Path(), err)
		ctx.Out.WriteError("ERR save failed: " + err.Error())
		return
	}
	ctx.Out.WriteSimple("OK")
}

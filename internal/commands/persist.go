package commands

import (
	"errors"
	"strings"

	"gopherdis/internal/persist"
)

const errNoSaving = "ERR saving is not configured"

// SAVE
//
// Writes the whole database to disk before replying. Other clients keep
// running commands while the file is written. Fails if another save is
// already running.
func save(ctx *Context) {
	saver := ctx.Env.Saver
	if saver == nil {
		ctx.Out.WriteError(errNoSaving)
		return
	}
	if _, err := saver.Save(); err != nil {
		if errors.Is(err, persist.ErrSaveInProgress) {
			writeErr(ctx.Out, err)
		} else {
			ctx.Out.WriteError("ERR save failed: " + err.Error())
		}
		return
	}
	ctx.Out.WriteSimple("OK")
}

// BGSAVE
//
// Starts a save in the background and replies straight away. Only one save
// may run at a time.
func bgsave(ctx *Context) {
	saver := ctx.Env.Saver
	if saver == nil {
		ctx.Out.WriteError(errNoSaving)
		return
	}
	if err := saver.BackgroundSave(); err != nil {
		writeErr(ctx.Out, err)
		return
	}
	ctx.Out.WriteSimple("Background saving started")
}

// LASTSAVE
//
// Replies with the Unix time of the last good save (the start time if
// there hasn't been one).
func lastsave(ctx *Context) {
	if ctx.Env.Saver == nil {
		ctx.Out.WriteInteger(ctx.Env.StartTime.Unix())
		return
	}
	ctx.Out.WriteInteger(ctx.Env.Saver.LastSave().Unix())
}

// SHUTDOWN [NOSAVE | SAVE]
//
// Asks the server to save and stop (NOSAVE: stop without saving). As in
// Redis there is no reply: the client's connection is closed.
func shutdown(ctx *Context) {
	saveFirst := true
	switch {
	case len(ctx.Args) == 1:
	case len(ctx.Args) == 2 && strings.EqualFold(string(ctx.Args[1]), "NOSAVE"):
		saveFirst = false
	case len(ctx.Args) == 2 && strings.EqualFold(string(ctx.Args[1]), "SAVE"):
	default:
		ctx.Out.WriteError("ERR syntax error")
		return
	}
	if ctx.Env.Shutdown == nil {
		ctx.Out.WriteError("ERR SHUTDOWN is not available")
		return
	}
	ctx.Env.Shutdown(saveFirst)
	ctx.CloseConn = true
}

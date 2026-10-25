package commands

import (
	"strconv"
	"time"
)

// TTL key
func ttl(ctx *Context) { replyTTL(ctx, false) }

// PTTL key
func pttl(ctx *Context) { replyTTL(ctx, true) }

// replyTTL replies -2 if the key doesn't exist, -1 if it has no expiry,
// otherwise the time left in milliseconds (PTTL) or seconds rounded to the
// nearest second (TTL), as Redis does.
func replyTTL(ctx *Context, millis bool) {
	left, exists, hasExpiry := ctx.Store.TTL(string(ctx.Args[1]))
	switch {
	case !exists:
		ctx.Out.WriteInteger(-2)
	case !hasExpiry:
		ctx.Out.WriteInteger(-1)
	case millis:
		ctx.Out.WriteInteger(left.Milliseconds())
	default:
		ctx.Out.WriteInteger((left.Milliseconds() + 500) / 1000)
	}
}

// EXPIRE key seconds
//
// A zero or negative time deletes the key straight away, like in Redis.
func expire(ctx *Context) {
	secs, err := strconv.ParseInt(string(ctx.Args[2]), 10, 64)
	if err != nil {
		ctx.Out.WriteError(notIntegerMsg)
		return
	}
	if secs > maxExpireMs/1000 || secs < -maxExpireMs/1000 {
		ctx.Out.WriteError("ERR invalid expire time in 'expire' command")
		return
	}
	at := ctx.Store.Now().Add(time.Duration(secs) * time.Second)
	if ctx.Store.Expire(string(ctx.Args[1]), at) {
		ctx.Out.WriteInteger(1)
	} else {
		ctx.Out.WriteInteger(0)
	}
}

// PERSIST key
func persistKey(ctx *Context) {
	if ctx.Store.Persist(string(ctx.Args[1])) {
		ctx.Out.WriteInteger(1)
	} else {
		ctx.Out.WriteInteger(0)
	}
}

package commands

import (
	"strconv"
	"strings"
	"time"
)

const errInvalidExpire = "ERR invalid expire time in 'set' command"

// SET key value [EX seconds | PX milliseconds | EXAT unix-seconds | PXAT unix-ms]
func set(ctx *Context) {
	var expireAt time.Time
	if len(ctx.Args) > 3 {
		var errMsg string
		expireAt, errMsg = parseSetExpiry(ctx.Args[3:], ctx.Store.Now())
		if errMsg != "" {
			ctx.Out.WriteError(errMsg)
			return
		}
	}
	ctx.Store.Set(string(ctx.Args[1]), ctx.Args[2], expireAt)
	ctx.Out.WriteSimple("OK")
}

// maxExpireMs keeps every expiry time well inside what time.Time and int64
// milliseconds can hold, so the conversions below can't overflow.
const maxExpireMs = int64(1) << 50 // about 35,000 years

// parseSetExpiry reads the options after SET key value. It returns the
// absolute expiry time, or an error message for the client.
func parseSetExpiry(opts [][]byte, now time.Time) (time.Time, string) {
	// Exactly one option and its number. Anything else, including two
	// expiry options, is a syntax error.
	if len(opts) != 2 {
		return time.Time{}, "ERR syntax error"
	}
	unit := strings.ToUpper(string(opts[0]))
	switch unit {
	case "EX", "PX", "EXAT", "PXAT":
	default:
		return time.Time{}, "ERR syntax error"
	}

	n, err := strconv.ParseInt(string(opts[1]), 10, 64)
	if err != nil || n <= 0 {
		return time.Time{}, errInvalidExpire
	}
	if unit == "EX" || unit == "EXAT" {
		if n > maxExpireMs/1000 {
			return time.Time{}, errInvalidExpire
		}
		n *= 1000
	}
	if n > maxExpireMs {
		return time.Time{}, errInvalidExpire
	}

	if unit == "EX" || unit == "PX" {
		return now.Add(time.Duration(n) * time.Millisecond), ""
	}
	// An absolute time in the past is allowed; the key is simply already expired.
	return time.UnixMilli(n), ""
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

// MGET key [key ...]
func mget(ctx *Context) {
	vals := ctx.Store.MGet(keyArgs(ctx.Args))
	ctx.Out.WriteArray(len(vals))
	for _, v := range vals {
		if v == nil {
			ctx.Out.WriteNull()
		} else {
			ctx.Out.WriteBulk(v)
		}
	}
}

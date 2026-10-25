package commands

import (
	"crypto/sha256"
	"crypto/subtle"
)

// AUTH password
// AUTH username password
//
// Only the "default" user exists, as in Redis without ACLs, so the
// two-argument form (sent by redis-cli --user and some client libraries)
// works with username "default".
func auth(ctx *Context) {
	var user, pass []byte
	switch len(ctx.Args) {
	case 2:
		user, pass = []byte("default"), ctx.Args[1]
	case 3:
		user, pass = ctx.Args[1], ctx.Args[2]
	default:
		ctx.Out.WriteError("ERR syntax error")
		return
	}

	if ctx.Env.Password == "" {
		ctx.Out.WriteError("ERR AUTH called without any password configured for the default user. Are you sure your configuration is correct?")
		return
	}
	if string(user) != "default" || !passwordMatches(pass, ctx.Env.Password) {
		ctx.Out.WriteError("WRONGPASS invalid username-password pair or user is disabled.")
		return
	}
	ctx.authenticated = true
	ctx.Out.WriteSimple("OK")
}

// passwordMatches compares in constant time, so the time taken doesn't hint
// at how much of a guess was right. Both sides are hashed first because
// subtle.ConstantTimeCompare returns early when the lengths differ, which
// would leak the password's length.
func passwordMatches(given []byte, want string) bool {
	g := sha256.Sum256(given)
	w := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(g[:], w[:]) == 1
}

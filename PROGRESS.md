# Progress

One report per step. The commit hash of each step is filled in with the following step,
since a commit can't contain its own hash.

## Step 0: Project setup
Status: DONE

What I built:
- Folder layout: `cmd/gopherdis`, `internal/{resp,server,commands,store,persist}`, `tests`.
  Each package has a `doc.go` with a one-line description so the tree builds before it has code.
- `go.mod` with module `gopherdis`, Go 1.22.
- `main.go` that prints `gopherdis starting` and exits.
- `Makefile` with `build`, `test` (`go test -race ./...`), `run` and `vet`.
- `README.md` and this file. `.gitignore` for the binary and dump files, `.gitattributes` to keep LF endings.

Files changed:
- all new: `go.mod`, `Makefile`, `cmd/gopherdis/main.go`, `internal/*/doc.go`, `tests/doc.go`,
  `README.md`, `PROGRESS.md`, `.gitignore`, `.gitattributes`

Tests:
- Added: none yet (nothing to test).
- Result of `go test -race ./...`: passes (no test files).

Manual checks:
- `go version` -> go1.23.9 linux/amd64
- `redis-cli --version` -> redis-cli 7.2.11
- `redis-benchmark --version` -> redis-benchmark 7.2.11
- `make build && ./bin/gopherdis` -> `gopherdis starting`

"Done when" checklist:
- [x] `go version`, `redis-cli --version` and `redis-benchmark --version` all work.
- [x] `make build` and `make test` run without errors.
- [x] First commit is made: `Step 0: project setup`.

Problems / decisions:
- I develop on Windows, so Go, make and the Redis tools run in an Alpine WSL distro
  (Go 1.23.9, Redis 7.2.11, gcc and musl-dev for the race detector).
- A real Redis server is already running on 6379 in that distro and other programs use it,
  so I left it alone. Gopherdis still listens on 6379 by default. Until `--port` exists
  (Step 10), I run manual checks inside a private network namespace (`unshare -n`), where
  6379 is free; after that I use `--port 6400`.

Commit: 44a5760 "Step 0: project setup"

## Step 1: RESP parser and serializer
Status: DONE

What I built:
- `resp.Value`, one struct for all five RESP2 types plus the nil bulk string and nil array,
  with small constructors (`BulkValue`, `NullValue`, `ArrayValue`, ...).
- `resp.Reader`: reads exactly one value from a `bufio.Reader`. Bulk strings are read by
  their length, never by looking for `\r\n`, so binary data with CRLF inside is safe.
- `resp.Writer`: `WriteSimple`, `WriteError`, `WriteInteger`, `WriteBulk`, `WriteBulkString`,
  `WriteNull`, `WriteNullArray`, `WriteArray` (header) and `WriteValue` (any value).
  It buffers through a `bufio.Writer`; nothing is sent until `Flush`.
- All bad input comes back as an error wrapping `resp.ErrProtocol`, or `io.ErrUnexpectedEOF`
  when the stream ends mid-message. A clean `io.EOF` only happens between messages, so the
  server can tell a normal disconnect from a broken one.

Files changed:
- `internal/resp/value.go`, `reader.go`, `writer.go`, `resp_test.go`

Tests:
- Added: 14 parse cases, 10 serialize cases, 11 round trips, 21 bad-input cases, split reads
  (one byte per read and a pipe with 3-byte writes), several messages in one read, EOF between
  messages, a huge-length header, and a fuzz test (`FuzzReader`).
- Result of `go test -race ./...`: all passing. `go test -fuzz=FuzzReader -fuzztime=30s`
  ran about 1.9 million inputs with no crash.

Manual checks:
- none needed; no networking in this step.

"Done when" checklist:
- [x] All the tests above exist and pass.
- [x] Bad input returns errors and never panics.
- [x] Commit: `Step 1: RESP parser and serializer`.

Problems / decisions:
- Three guards so bad input can't crash or exhaust the process, even before Step 12's limits:
  arrays may nest at most 32 deep (deep nesting would grow the stack without end);
  a bulk string bigger than 64 KB grows as its bytes actually arrive instead of being
  allocated from the header; header lines longer than the 4 KB read buffer are rejected.
- Lengths are parsed strictly: digits only, or exactly `-1`. `$+3` and `$-2` are errors.

Commit: fac8bcb "Step 1: RESP parser and serializer"

## Step 2: TCP server, PING and ECHO
Status: DONE

What I built:
- `server.Server`: listens on TCP, one goroutine per client running read → run → reply → flush.
  A client that disconnects or sends something unparseable only ends its own goroutine.
  `Serve(ln)` takes a listener so tests can use a free port; `Close()` stops accepting,
  closes open connections and waits for client goroutines. Temporary accept errors
  (e.g. too many open files) are retried with a short backoff.
- `commands.Table`: a map from upper-case name to handler plus arity (Redis style: the name
  counts, negative means "at least"). The table checks the argument count, so handlers don't.
- `PING`, `PING msg`, `ECHO msg`, `COMMAND [anything]` (empty array), and the standard
  unknown-command and wrong-argument-count errors.
- `main.go` listens on `:6379`.

Files changed:
- `cmd/gopherdis/main.go`, `internal/commands/commands.go`, `internal/server/server.go`
- tests: `internal/commands/commands_test.go`, `internal/server/server_test.go`,
  `tests/helpers_test.go`, `tests/basic_test.go`
- `go.mod`, `go.sum`: go-redis v9.18.0 (tests only)

Tests:
- Added: 16 command cases (case-insensitivity, errors, binary ECHO); raw-TCP server tests
  (unknown command keeps the connection open, a command split across 4 writes, a garbage
  client and a client leaving mid-command don't affect others, Close disconnects clients);
  go-redis end-to-end tests for PING, ECHO, unknown command, wrong arg count, COMMAND DOCS.
- Result of `go test -race ./...`: all passing.

Manual checks (in a private network namespace, so 6379 is free):
- `redis-cli PING` -> `PONG`
- `redis-cli ECHO "Hello World"` -> `Hello World`
- `redis-cli FOO` -> `ERR unknown command 'FOO'`
- `redis-cli` interactive -> prompt `127.0.0.1:6379>`, `PING` -> `PONG`, `ECHO hi` -> `"hi"`

"Done when" checklist:
- [x] All commands above work with the real `redis-cli`, including interactive mode.
- [x] The automated end-to-end test passes.
- [x] Commit: `Step 2: TCP server, PING and ECHO`.

Problems / decisions:
- go-redis is pinned to v9.18.0: it is the newest release that still builds with Go 1.22
  (v9.19+ needs Go 1.24, v9.23 needs 1.26).
- go-redis sends `HELLO 3` and `CLIENT SETINFO` on connect. We answer them with
  unknown-command errors and it falls back to RESP2, so no special handling is needed.
- A message that isn't an array of bulk strings closes the connection for now.
  Step 12 adds the `-ERR Protocol error` reply and inline commands.

Commit: f6c15ff "Step 2: TCP server, PING and ECHO"

## Step 3: SET and GET
Status: DONE

What I built:
- `store.Store`: a map from key to `*Entry`. `Entry` already has everything later steps need:
  `Kind` (string or list), `Str`, `List` and `ExpireAt` (zero = no expiry).
- Values are `[]byte` and `Set` stores a copy, so binary data is kept exactly and the store
  never shares memory with a network buffer. Stored slices are never changed in place, so a
  reply can be written from a slice after the store has moved on.
- `SET key value` -> `+OK`; `GET key` -> bulk string or `$-1`. `GET` on a list key returns the
  WRONGTYPE error (no lists exist yet, but the check is in place and tested).
- `commands.NewTable` now takes the store; handlers get it through `Context.Store`.

Files changed:
- `internal/store/store.go`, `internal/commands/strings.go`, `internal/commands/commands.go`,
  `cmd/gopherdis/main.go`
- tests: `internal/store/store_test.go`, `internal/commands/commands_test.go`,
  `tests/strings_test.go`, helper updates in `tests/helpers_test.go` and `internal/server/server_test.go`

Tests:
- Added: 4 store tests (set/get/overwrite/missing, empty and binary, Set copies its input,
  wrong type), 13 command cases, 3 go-redis tests (overwrite, missing key, empty value,
  value with spaces, binary bytes, CRLF inside a value, wrong argument counts).
- Result of `go test -race ./...`: all passing.

Manual checks:
- `redis-cli SET name Srijoy` -> `OK`
- `redis-cli GET name` -> `Srijoy`
- `redis-cli GET nothing` -> (nil; redis-cli prints an empty line when not on a terminal)
- `redis-cli SET name Paul` -> `OK`, then `GET name` -> `Paul`
- `redis-cli SET "with space" "a b c"` then `GET "with space"` -> `a b c`

"Done when" checklist:
- [x] SET and GET work in `redis-cli` and in tests.
- [x] Commit: `Step 3: SET and GET`.

Problems / decisions:
- `SET k v extra` replies `-ERR syntax error`, as real Redis does for an unknown option.
  Step 5 turns the extra arguments into the EX/PX/EXAT/PXAT options.

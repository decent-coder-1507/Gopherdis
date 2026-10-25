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

Commit: bad0efe "Step 3: SET and GET"

## Step 4: Concurrent clients
Status: DONE

What I built:
- `sync.RWMutex` in `Store`: `Get` takes the read lock, `Set` the write lock. `Set` copies the
  value before taking the lock, so the lock covers only the map write. No lock is ever held
  while reading from or writing to a connection: handlers get the value, release the lock,
  then write the reply.
- Each connection already writes through a `bufio.Writer` (from Step 1's `resp.Writer`) and
  flushes after every reply. Step 9 changes that for pipelining.
- `CONFIG GET name [name ...]`: `save` -> `""`, `appendonly` -> `"no"`, other names nothing.
  `CONFIG` with another subcommand gets `-ERR unknown subcommand`.

Files changed:
- `internal/store/store.go`, `internal/commands/commands.go`
- tests: `internal/store/store_test.go`, `internal/commands/commands_test.go`, `tests/concurrency_test.go`

Tests:
- Added: store test with 50 goroutines x 1,000 SET+GET on their own keys plus one shared key;
  go-redis test with 50 separate client connections x 1,000 SET+GET, then a final check of all
  50,000 keys; 7 CONFIG cases.
- Result of `go test -race ./...`: all passing, no data races.

Manual checks:
- `redis-benchmark -t SET,GET -q` -> `SET: 67934.78 requests per second`, `GET: 66800.27 requests per second`,
  no errors, no warnings
- `redis-benchmark -t SET,GET -q -n 1000` -> SET 52631.58, GET 76923.08 rps
- `redis-cli PING` afterwards -> `PONG` (server still running)

"Done when" checklist:
- [x] `redis-benchmark` finishes with no errors and the server does not crash.
- [x] `go test -race ./...` reports no data races.
- [x] Commit: `Step 4: concurrent clients`.

Problems / decisions:
- The guide says an empty array for `CONFIG GET` avoids redis-benchmark's warnings. With
  redis-benchmark 7.2 it doesn't: it still prints `WARNING: Could not fetch server CONFIG`,
  because it expects a `[name, value]` pair for both `save` and `appendonly`. So those two names
  return their real values (we have no automatic saving and no AOF yet); any other name still
  returns an empty array. Step 10 extends the same table with the real settings.
- These numbers are from WSL2 with the benchmark and server on the same machine, so they
  move around by 20% or so between runs. Step 8 does the proper comparison.

Commit: 24f3e41 "Step 4: concurrent clients"

## Step 5: Key expiry
Status: DONE

What I built:
- `SET key value EX|PX|EXAT|PXAT n`, options case-insensitive, at most one. Zero, negative,
  non-numeric or absurdly large times reply `-ERR invalid expire time in 'set' command`;
  two options, a missing number or an unknown option reply `-ERR syntax error`.
- The expiry is stored as an absolute time in `Entry.ExpireAt`. A plain `SET` writes a new
  entry with no expiry, which clears the old one.
- Lazy expiry: a read finds the key expired under the read lock, then takes the write lock,
  checks again (another client may have replaced the key meanwhile) and deletes it.
- Active expiry (`store/expire.go`): 10 times a second, sample up to 20 keys from the
  `expires` index (only keys with an expiry are in it) and delete the expired ones; repeat
  at once if more than 5 were expired. Each round takes the lock on its own, and one run is
  capped at 25 ms, so clients are never blocked for long.
- The clock is a `now func() time.Time` in the store (`NewWithClock`), so tests move time by
  hand instead of sleeping.
- `main.go` starts the cleanup and stops it on exit.

Files changed:
- `internal/store/store.go`, `internal/store/expire.go`, `internal/commands/strings.go`, `cmd/gopherdis/main.go`
- tests: `internal/store/expire_test.go`, `internal/store/store_test.go`,
  `internal/commands/commands_test.go`, `tests/expiry_test.go`

Tests:
- Added: lazy expiry at the exact expiry instant (fake clock), plain SET clearing the expiry,
  cleanup removing 1,000 unread expired keys while keeping 20 live ones, a cycle repeating when
  most samples are expired, the background cleanup with the real clock; all four options before
  and after expiry, EXAT in the past, 15 bad-time and syntax cases; go-redis tests over the network
  for the four options, the errors and plain SET clearing the expiry.
- Result of `go test -race ./...`: all passing.

Manual checks:
- `redis-cli SET temp hello PX 2000` -> `OK`; `GET temp` -> `hello`; after 2 s `GET temp` -> (nil)
- `redis-cli SET a 1 EX 0` -> `ERR invalid expire time in 'set' command`
- `redis-cli SET a 1 ex 10 px 100` -> `ERR syntax error`
- `SET b 1 PX 3000`, wait 3 s, `GET b` -> (nil)

"Done when" checklist:
- [x] All four options work and all tests pass.
- [x] Expired keys are removed even if they are never read again.
- [x] Commit: `Step 5: key expiry`.

Problems / decisions:
- An `EXAT`/`PXAT` time in the past is accepted and the key is gone at once, the same as real
  Redis. Only zero and negative numbers are rejected, as the guide says.
- Expiry times are capped at 2^50 ms (about 35,000 years) so seconds-to-milliseconds and
  `time.Time` math can't overflow.
- "Random" sampling uses Go's map iteration, which starts at a random place each time.
  It's not perfectly uniform, but it's what makes the cleanup cheap, and the tests show it
  clears everything.

Commit: 2f7d75f "Step 5: key expiry"

## Step 6: EXISTS, DEL, INCR, DECR, LPUSH, RPUSH
Status: DONE

What I built:
- `EXISTS k...` (a key named twice counts twice) and `DEL k...` (counts keys actually deleted).
  Both treat expired keys as missing; `DEL` still clears them out.
- `INCR`/`DECR` through `Store.IncrBy`: 64-bit, a missing (or expired) key starts at 0, the key
  keeps its expiry, overflow and non-integers reply `-ERR value is not an integer or out of range`.
  Parsing follows Redis: optional `-`, no `+`, no spaces, no leading zeros, no `-0`.
- `LPUSH`/`RPUSH` through `Store.Push`, which creates the list when needed. `LPUSH mylist a b c`
  gives `c, b, a`.
- `store.List`: two slices, one for the front (reversed) and one for the back, so pushing at
  either end is an append. With a single slice every LPUSH would copy the whole list, and
  `redis-benchmark -t lpush` (100,000 pushes onto one list) would slow to a crawl.
- WRONGTYPE: GET/INCR/DECR on a list and LPUSH/RPUSH on a string. SET still replaces any type.

Files changed:
- `internal/store/keys.go`, `internal/store/list.go`, `internal/store/store.go` (`Entry.List` is now `*List`)
- `internal/commands/keys.go`, `internal/commands/lists.go`, `internal/commands/commands.go`
- tests: `internal/store/keys_test.go`, `internal/store/list_test.go`, `internal/store/store_test.go`,
  `internal/commands/keys_test.go`, `tests/keys_test.go`

Tests:
- Added: every "How to test" line of this step, in order, at command level and over the network
  with go-redis; 16 bad-integer strings (each must leave the value unchanged); overflow at both
  ends of int64; INCR keeping an expiry and INCR on an expired key starting fresh; push order for
  LPUSH, RPUSH and mixed pushes; every WRONGTYPE combination; argument-count errors for all six.
- Result of `go test -race ./...`: all passing.

Manual checks:
- `redis-cli SET n 10 ; redis-cli INCR n` -> `OK`, `11`
- `redis-cli INCR new` -> `1`
- `redis-cli SET word hi ; redis-cli INCR word` -> `OK`, `ERR value is not an integer or out of range`
- `redis-cli RPUSH list a b ; redis-cli LPUSH list z` -> `2`, `3`
- `redis-cli GET list` -> `WRONGTYPE Operation against a key holding the wrong kind of value`
- `redis-cli EXISTS n list nope` -> `2`; `redis-cli DEL n list nope` -> `2`
- `redis-benchmark -t lpush,rpush,incr -n 100000 -q` -> INCR 77220, LPUSH 82102, RPUSH 81566 rps

"Done when" checklist:
- [x] All six commands work, with correct errors.
- [x] Commit: `Step 6: EXISTS, DEL, INCR, DECR, LPUSH, RPUSH`.

Problems / decisions:
- Lists are the one value changed in place (under the write lock). Anything that reads a list
  copies the item slice out while holding the lock (`List.Slice`), so the reply can be
  written after the lock is released. The items' bytes are never changed, so only the slice
  headers are copied.

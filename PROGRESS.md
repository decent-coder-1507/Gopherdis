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

Commit: 2b2b379 "Step 6: EXISTS, DEL, INCR, DECR, LPUSH, RPUSH"

## Step 7: SAVE and load on start-up
Status: DONE

What I built:
- `store.Snapshot()` copies every live key (type, data, expiry) under the read lock and returns.
  String bytes are shared (they never change in place); list items are copied out.
  `store.Restore()` replaces the database from a snapshot and skips keys whose expiry has passed.
- `persist.Save` writes `dump.gob.tmp` with `encoding/gob`, then `Sync()`, `Close()`, and renames
  it over `dump.gob` (and syncs the directory so the rename itself is on disk). If anything
  fails, the temp file is removed and the old dump is untouched.
- The file starts with a small header (`GOPHERDIS-DUMP`, format version 1), so a wrong or
  future file gets a clear message instead of a confusing decode error.
- `persist.Saver` ties a store to a path and allows one save at a time. The slow disk write
  happens after the store lock is released.
- `SAVE` -> `+OK` (or an error naming what went wrong). `main.go` loads `dump.gob` before it
  starts listening; a damaged file logs the reason and exits with status 1.

Files changed:
- `internal/store/snapshot.go`, `internal/persist/persist.go`, `internal/commands/persist.go`,
  `internal/commands/commands.go`, `cmd/gopherdis/main.go`
- tests: `internal/persist/persist_test.go`, `internal/store/snapshot_test.go`,
  `internal/commands/commands_test.go`, `tests/persist_test.go`, `tests/helpers_test.go`

Tests:
- Added: save then load gives the same keys, types, bytes, list order and expiry times (empty
  string, binary, list with an empty item); load replaces old data; expired keys (string and list)
  are skipped; a missing file is fine; six kinds of broken file (garbage, empty, truncated,
  header only, wrong magic, random bytes) each give an error naming the file and leave the store
  empty; no temp file is left after three saves; a failed save keeps the old file byte for byte;
  saving 20 times while another goroutine writes (race detector); snapshot is a real copy;
  end-to-end: SET/RPUSH/SET EX/binary, SAVE, stop, start a new server on the same file, read back,
  and a key that expired while the server was down stays gone.
- Result of `go test -race ./...`: all passing.

Manual checks:
- `redis-cli SET a 1 ; redis-cli RPUSH l x y ; redis-cli SET t 1 EX 1000` -> `OK`, `2`, `OK`
- `redis-cli SAVE` -> `OK`; the folder holds only `dump.gob`
- restart -> log `loaded 3 keys from dump.gob`; `redis-cli GET a` -> `1`; `redis-cli EXISTS l t` -> `2`
- cut `dump.gob` to 40 bytes and start -> `cannot load saved data: dump.gob is not a valid dump file:
  unexpected EOF`, exit code 1

"Done when" checklist:
- [x] Data, lists and expiry times survive a restart.
- [x] Commit: `Step 7: SAVE and load on start-up`.

Problems / decisions:
- The dump path is fixed to `dump.gob` in the working directory for now; Step 10 adds
  `--dir` and `--dbfilename`.
- gob turns an empty `[]byte` into nil. `Restore` turns it back into an empty value so an
  empty string still exists after a reload (tested).

Commit: dcf968d "Step 7: SAVE and load on start-up"

## Step 8: Benchmark and performance fixes
Status: DONE

How I measured:
- Gopherdis on 6379 and a separate real `redis-server --port 6380 --save ""` (7.2.11), both in the
  same private network namespace in WSL2 (8 CPUs), so the Redis already running on this machine
  wasn't involved. `redis-benchmark -p <port> -t set,get -n 100000 -q` (50 clients), 3 runs each,
  middle value used.

Results after the fixes:

| Test | Real Redis (req/sec) | Gopherdis (req/sec) | Gopherdis as % of Redis |
|------|---------------------:|--------------------:|------------------------:|
| SET  | 124,378              | 74,405              | 60%                     |
| GET  | 131,406              | 70,423              | 54%                     |

Before the fixes: real 126,422 / 122,100, Gopherdis 80,064 / 75,586 (63% / 62%).
The two sets of numbers are within run-to-run noise of each other (single runs ranged
from 45k to 86k on our side), so the honest summary is "about 55-65% of real Redis".

What I looked at:
- Lock held too long: no. Handlers take the store lock only around the map access; replies
  are written after it's released.
- Too many small writes: no. Replies go through the `bufio.Writer`, one flush per reply.
- Allocations: yes. A Go benchmark over real TCP (`internal/server/bench_test.go`, 50 parallel
  clients) with `-cpuprofile`/`-memprofile` showed every command allocating a `[]resp.Value`, one
  byte slice per argument, an args slice, a `Context`, and an upper-case copy of the command name.

What I changed:
- `resp.Reader.ReadCommand`: reads a command straight into a buffer the reader reuses (parts
  are sliced out of it after the whole command is read, since appending can move the buffer).
  Buffers over 1 MB aren't kept, so one huge command doesn't pin memory to a connection.
  The general `Read` is still there for tests and the fuzzer.
- One `commands.Context` per connection instead of one per command.
- Command lookup tries the name as sent, then upper-cases it into a 16-byte stack array;
  neither allocates.
- Result in the Go benchmark: SET 11 -> 5 allocs/op (637 -> 204 B), GET 6 -> 1 alloc/op
  (348 -> 59 B), counting the benchmark client's own allocations.

Why we're still slower:
- The CPU profile is dominated by the read and write syscalls and by goroutine scheduling;
  our own code (parse + lookup + store) is under 5% of the time. Real Redis runs one thread
  with epoll and answers without handing work between threads. With Go's netpoller, each
  request wakes a goroutine, often on another OS thread. Running with `GOMAXPROCS=1` sometimes
  hit 103k SET / 112k GET but swung down to 41k on other runs, which points at the same cause.
  I left GOMAXPROCS at Go's default: pinning it to 1 would throw away all other cores, and
  Step 9's pipelining attacks the real cost (one syscall pair per command).

Files changed:
- `internal/resp/command.go` (new), `internal/resp/reader.go`, `internal/commands/commands.go`,
  `internal/server/server.go`
- tests: `internal/resp/resp_test.go`, `internal/commands/commands_test.go`, `internal/server/bench_test.go`

Tests:
- Added: `ReadCommand` on 5 commands fed one byte at a time (empty value, CRLF inside data,
  empty and null arrays), parts never overlapping (a 192 KB part between two 1-byte parts), 9
  bad inputs, and `FuzzReadCommand`, which checks it agrees with the general `Read` on every
  input it accepts (20 s, no failures). Benchmarks `BenchmarkSet`, `BenchmarkGet`.
- Result of `go test -race ./...`: all passing.

"Done when" checklist:
- [x] The results table is in PROGRESS.md.
- [x] At least 33% of real Redis speed for SET and GET: 60% and 54%.
- [x] Commit: `Step 8: benchmark and performance fixes`.

Problems / decisions:
- Handlers now get argument slices that are only valid during the call (this was already the
  documented rule). Everything that keeps data (SET, LPUSH, RPUSH) copies it in the store, and
  keys become Go strings, which copy too.

Commit: e56f5ca "Step 8: benchmark and performance fixes"

## Step 9: Pipelining and extra commands
Status: DONE

What I built:
- Pipelining: after running a command, if more bytes are already in the read buffer
  (`Reader.Buffered() > 0`), the server runs the next command before flushing. It flushes
  once nothing more is waiting, so a batch gets its replies in one write, in order.
- `TTL`/`PTTL`: -2 no key, -1 no expiry, else time left. TTL rounds to the nearest second,
  as Redis does (`PX 1500` -> `TTL` 2).
- `EXPIRE key seconds`: 1 if set, 0 if no key. Works on any type. Zero or a negative time
  deletes the key at once (Redis behaviour). The entry is replaced, not edited, so a reader
  holding the old entry is unaffected.
- `PERSIST`: 1 only if the key existed and had an expiry.
- `KEYS pattern`: glob with `*`, `?`, `[abc]`, `[a-z]`, `[^abc]` and `\` escapes, byte-based like
  Redis. The matcher remembers the last `*` and retries from there, with no recursion, so
  patterns like `a*a*a*...b` can't blow up (tested with 30 stars against 5,000 bytes). Expired keys
  are skipped.
- `LRANGE key start stop` (negative indexes, clamping, empty for no key or an empty range),
  `LLEN` (0 for no key), `MGET` (nil for missing or non-string keys), `DBSIZE` (live keys
  only, so a key that expired but wasn't cleaned up yet isn't counted), `FLUSHALL [ASYNC|SYNC]`.
- WRONGTYPE for LRANGE/LLEN on a string.

Files changed:
- `internal/server/server.go`, `internal/resp/reader.go` (`Buffered`), `internal/store/more.go`,
  `internal/commands/{expire,glob,keys,lists,strings,commands}.go`
- tests: `internal/commands/glob_test.go`, `internal/commands/step9_test.go`,
  `internal/store/keys_test.go`, `tests/step9_test.go`

Tests:
- Added: go-redis pipeline of 1,000 mixed commands (SET, GET of the key just set, INCR, ECHO)
  with every reply checked in order; 500 commands sent in one raw write with 500 ordered replies
  and nothing extra; 34 glob cases; TTL/PTTL/EXPIRE/PERSIST with a fake clock including rounding,
  expiry at the exact moment, EXPIRE 0 and EXPIRE on a list; KEYS with 7 patterns (expired key
  left out); 10 LRANGE index cases plus wrong type, bad numbers and arg counts; MGET with missing
  and wrong-type keys; DBSIZE not counting an expired key; FLUSHALL in its three forms.
- Result of `go test -race ./...`: all passing.

Benchmark with pipelining (`redis-benchmark -t set,get -n 100000 -P 16 -q`, 3 runs, middle value):

| Test | Real Redis (req/sec) | Gopherdis (req/sec) | Gopherdis as % of Redis |
|------|---------------------:|--------------------:|------------------------:|
| SET  | 724,638              | 769,231             | 106%                    |
| GET  | 806,452              | 884,956             | 110%                    |

Without pipelining in the same session: real 112,740 / 116,009, Gopherdis 83,682 / 86,580
(74% / 75%). Once 16 commands share one read and one write, the syscall cost that held us back
in Step 8 is spread out, and Go can use several cores while real Redis runs commands on one.

Manual checks:
- `SET k v`, `TTL k` -> `-1`; `EXPIRE k 100` -> `1`; `TTL k` -> `100`; `PTTL k` -> `99996`
- `PERSIST k` -> `1`; `TTL nokey` -> `-2`
- `RPUSH l a b c d` -> `4`; `LRANGE l 0 -1` -> `a b c d`; `LRANGE l -2 -1` -> `c d`; `LLEN l` -> `4`
- `MGET k nope l` -> `v`, (nil), (nil)
- `KEYS "*"` -> `k l`; `KEYS "l*"` -> `l`; `DBSIZE` -> `2`; `FLUSHALL` -> `OK`; `DBSIZE` -> `0`

"Done when" checklist:
- [x] The pipeline test passes with all replies in order.
- [x] All new commands are tested, including the edge cases in the table.
- [x] Benchmark with `-P 16` is recorded in PROGRESS.md.
- [x] Commit: `Step 9: pipelining and extra commands`.

Problems / decisions:
- `EXPIRE` with zero or a negative time deletes the key and returns 1, as Redis does; the guide
  only says what happens for positive times. A time too large to represent gives
  `-ERR invalid expire time in 'expire' command`.
- `KEYS` with a huge database blocks writers while it runs (it holds the read lock), just like
  in real Redis. That's why Redis has SCAN; SCAN isn't part of this guide.

Commit: ea495ed "Step 9: pipelining and extra commands"

## Step 10: Settings, logging, INFO
Status: DONE

What I built:
- Flags (Go's `flag` package, `--name` or `-name`): `--port` 6379, `--bind` 127.0.0.1, `--dir` .,
  `--dbfilename` dump.gob, `--save-interval` 60s, `--requirepass` (empty), `--maxclients` 10000,
  `--loglevel` info. All are checked at start: port range, bind must be an IP, dbfilename must
  be a plain name, dir must exist, log level must be known. Every problem is listed in one message
  and the exit code is 2. `--help` prints one line per flag with its default.
  (`--save-interval`, `--requirepass` and `--maxclients` are read now and take effect in Steps 11 and 12.)
- `CONFIG GET` reports the real settings under the flag names, accepts glob patterns
  (`CONFIG GET *`, `CONFIG GET max*`) and several names; unknown names still give an empty array.
  `save` is also given in Redis' "seconds changes" form (`60 1`) for tools that ask.
- Logging with `log/slog` text output: info for server started (address, port, version), data
  loaded (keys, time), save finished (file, keys, time); debug for client connect/disconnect;
  warn for a client sending a bad message; error for failed saves and failed start-up. No stored
  value or password is ever logged.
- `INFO [section]`: bulk string of `name:value` lines with version, uptime, connected clients,
  total commands processed, number of keys and last save time.
- `commands.Env` collects what commands need beyond the store (saver, version, settings,
  client count, start time), replacing `SetSaver`. `main.go` now parses flags and calls `run`.

Bug found and fixed:
- The race detector caught a real race in `server.Close` (from Step 2): `wg.Add` for a new
  client ran after the "closed?" check had released the lock, so `Close` could start waiting
  before the Add and return while a client goroutine was still starting. `wg.Add` now happens
  under the same lock as the closed check. Ran the server tests 20 more times with `-race`: clean.

Files changed:
- `cmd/gopherdis/main.go`, `cmd/gopherdis/config.go` (new), `internal/commands/{commands,admin,persist}.go`,
  `internal/persist/persist.go` (logs, `LastSave`), `internal/server/server.go` (logs, `ConnectedClients`, race fix)
- tests: `cmd/gopherdis/config_test.go`, `internal/commands/admin_test.go`, `tests/info_test.go`,
  plus `Env{}` in the existing tests

Tests:
- Added: defaults; every flag set; 13 bad flag/argument cases; `--help` lists every flag;
  CONFIG GET exact, case-insensitive, glob, duplicate names, unknown; INFO fields and that
  unknown commands aren't counted; INFO over the network with two clients connected.
- Result of `go test -race ./...`: all passing.

Manual checks:
- `gopherdis --help` -> usage with all 8 flags and defaults, exit 0
- `gopherdis --port 70000 --loglevel loud --dir /nope` -> one message naming all three problems, exit 2
- `gopherdis --port 7000 --dir /tmp` -> `level=INFO msg="server started" address=127.0.0.1 port=7000`;
  `redis-cli -p 7000 CONFIG GET port` -> `port 7000`
- `gopherdis --port 6400 --dir /tmp/s10 --loglevel debug`, then `SET`, `CONFIG GET port|dir|nosuch`,
  `SAVE`, `INFO` -> all as expected; dump written to `/tmp/s10/dump.gob`; the log shows
  connect/disconnect lines, `save finished ... keys=1`, and a WARN for garbage sent with `nc`

"Done when" checklist:
- [x] `./gopherdis --port 7000 --dir /tmp` works, and `--help` is clear.
- [x] Logs show the events above. INFO works in `redis-cli`. (The shutdown log line comes with
  Step 11, which adds signal handling; until then the process is simply killed.)
- [x] Commit: `Step 10: settings, logging, INFO`.

Problems / decisions:
- `CONFIG GET requirepass` returns the password, as real Redis does. After Step 12 only a client
  that has already authenticated can run CONFIG.
- `last_save_time` starts as the server start time, like Redis' `LASTSAVE`, until the first save.

Commit: a8630b8 "Step 10: settings, logging, INFO"

## Step 11: Graceful shutdown and auto-save
Status: DONE

What I built:
- Signals: `main` catches SIGINT (Ctrl+C) and SIGTERM. `run()` then: stops accepting clients,
  lets every client finish the command it's running and any commands it had already sent
  (idle clients are woken by moving their read deadline to now), stops the cleanup and
  auto-save jobs, saves, closes connections, logs `shutdown complete`, exits 0. A second
  Ctrl+C during shutdown kills the process straight away, the usual escape hatch.
- `SHUTDOWN [NOSAVE|SAVE]`: asks `run()` to do the same; NOSAVE skips the save. As in Redis,
  there's no reply: the connection is closed, and commands pipelined after SHUTDOWN don't run.
- Time limit: clients get up to 5 s to finish, leaving the rest of the 10 s for the save. If the
  whole shutdown passes 10 s, it logs a warning and exits anyway (code 1). The dump file is
  replaced atomically, so exiting in the middle of a save leaves the previous file intact.
- Change counter in the store: every successful write (SET, DEL that deleted something, INCR/DECR,
  LPUSH/RPUSH, EXPIRE, PERSIST, FLUSHALL) adds one. A snapshot records the count it matches, so
  `Saver.Dirty()` knows exactly whether anything changed since the last good save. Loading
  a file counts as saved.
- Auto-save: every `--save-interval` (default 60s, 0 = off), if dirty, a background save.
- `BGSAVE` -> `+Background saving started`; `LASTSAVE` -> Unix time of the last good save.
  Only one save runs at a time (a one-slot channel): BGSAVE or SAVE while a save is running
  gets `-ERR Background save already in progress`; the shutdown save waits its turn instead.

Files changed:
- `cmd/gopherdis/main.go` (signals, `run`, `shutdown`), `internal/server/server.go` (`Shutdown`,
  `CloseConn`), `internal/persist/persist.go` (Saver: one save at a time, `BackgroundSave`, `SaveWait`,
  `Dirty`, `StartAutoSave`), `internal/store/*.go` (change counter), `internal/commands/{commands,persist}.go`
- tests: `cmd/gopherdis/main_test.go`, `internal/persist/saver_test.go`, `internal/server/server_test.go`,
  `internal/commands/admin_test.go`, `tests/persist_test.go`, small updates for the new `Snapshot` signature

Tests:
- Added: shutdown saves (SHUTDOWN, and SIGINT and SIGTERM through `run`) and the data is back after a
  restart; SHUTDOWN NOSAVE writes nothing; the real binary built and stopped with real SIGINT and
  SIGTERM (exit 0, dump written); auto-save writes nothing while unchanged and saves after a SET;
  dirty tracking (reads and failed writes don't count); two BGSAVEs at once -> the second is refused
  (in the saver with a held save, and over the network with a pipeline); SAVE refused during
  BGSAVE; SaveWait waits for the running save; the store keeps answering while a save is held
  half-way; a client SETs during a BGSAVE of 50,000 keys; Server.Shutdown wakes idle clients at
  once, answers commands already sent, refuses new clients; SHUTDOWN closes with no reply and the
  pipelined PING after it doesn't run; LASTSAVE moves after a save and not after a failed one.
- Result of `go test -race ./...`: all passing.

Manual checks (port 6400):
- `SET x 1`, Ctrl+C (SIGINT) -> exit 0, log `shutting down reason="signal interrupt"`, `save finished keys=1`,
  `shutdown complete`; restart -> `data loaded keys=1`, `GET x` -> `1`
- `SET y 2`, SIGTERM -> exit 0; restart, `GET y` -> `2`
- `SET z 3`, `SHUTDOWN` -> exit 0; restart, `GET z` -> `3`
- `SET w 4`, `SHUTDOWN NOSAVE` -> exit 0, no save logged; restart, `EXISTS w` -> `0`
- `BGSAVE` -> `Background saving started`; `LASTSAVE` -> `1792947026`

"Done when" checklist:
- [x] Ctrl+C, SIGTERM and SHUTDOWN all save and exit cleanly.
- [x] Auto-save and BGSAVE work, and tests pass with `-race`.
- [x] Commit: `Step 11: graceful shutdown and auto-save`.

Problems / decisions:
- If the shutdown save fails (disk full, folder gone), Gopherdis logs the error and exits with
  code 1. Real Redis refuses to stop in that case, but by then we have already stopped serving
  clients. Exit code 1 makes a supervisor (systemd, Docker) notice.
- On Windows only Ctrl+C can be caught; SIGTERM isn't something Windows sends. The process test
  skips itself there. Everything here was tested on Linux (WSL).

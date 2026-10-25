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

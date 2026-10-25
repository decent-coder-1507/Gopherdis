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

Commit: "Step 0: project setup"

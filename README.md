# Gopherdis

Gopherdis is a small in-memory key-value server written in Go that speaks the Redis protocol,
so `redis-cli`, `redis-benchmark` and ordinary Redis client libraries work with it unchanged.
It stores strings and lists, supports key expiry, saves to disk, and has a password option,
safety limits and a clean shutdown.

It uses only the Go standard library; the [go-redis](https://github.com/redis/go-redis) client
appears in the tests and nowhere else.

## Build and run

You need Go 1.22 or newer and `make`.

```bash
make build
./bin/gopherdis
```

The server listens on `127.0.0.1:6379` and keeps its data in `./dump.gob`. Try it from another
terminal:

```bash
redis-cli SET city Kolkata
redis-cli GET city
```

Stop it with Ctrl+C. It finishes the commands it is running, saves, and exits.

Without `make`: `go build -o bin/gopherdis ./cmd/gopherdis`.

### With Docker

```bash
make docker
docker run -p 6379:6379 -v "$(pwd)/data:/data" gopherdis
```

Inside the container the server listens on all interfaces and saves to `/data`, so the data
outlives the container. `docker stop` sends SIGTERM, which saves before exiting.

Extra flags go after the image name, for example
`docker run -p 6379:6379 gopherdis --requirepass s3cret`.

## Settings

Every setting is a flag with a default, so none are needed.

| Flag | Default | Meaning |
|------|---------|---------|
| `--port` | `6379` | Port to listen on |
| `--bind` | `127.0.0.1` | Address to listen on. The default accepts connections from this computer only. |
| `--dir` | `.` | Folder for the save file (must exist) |
| `--dbfilename` | `dump.gob` | Save file name, inside `--dir` |
| `--save-interval` | `60s` | How often to save if anything changed, e.g. `30s`, `5m`. `0` turns it off. |
| `--requirepass` | (empty) | Password clients must send with `AUTH`. Empty means no password. |
| `--maxclients` | `10000` | Most clients connected at once |
| `--loglevel` | `info` | `debug`, `info`, `warn` or `error` |
| `--version` | | Print the version and exit |

`./bin/gopherdis --help` prints the same list. A running server reports its settings with
`CONFIG GET <name>` (glob patterns work: `CONFIG GET *`).

## Commands

Command names are not case sensitive.

| Command | Reply |
|---------|-------|
| `PING [msg]` | `PONG`, or `msg` |
| `ECHO msg` | `msg` |
| `SET key value [EX s \| PX ms \| EXAT unix-s \| PXAT unix-ms]` | `OK` |
| `GET key` | the value, or nil |
| `MGET key [key ...]` | array of values, nil for missing or non-string keys |
| `INCR key` / `DECR key` | the new value (a missing key starts at 0) |
| `EXISTS key [key ...]` | how many exist (a key named twice counts twice) |
| `DEL key [key ...]` | how many were deleted |
| `KEYS pattern` | matching keys; `*`, `?`, `[abc]`, `[a-z]`, `[^a]`, `\` escapes |
| `DBSIZE` | number of keys |
| `FLUSHALL [ASYNC \| SYNC]` | `OK`, after deleting every key |
| `LPUSH key value [value ...]` / `RPUSH ...` | the new list length |
| `LRANGE key start stop` | items in the range; negative indexes count from the end |
| `LLEN key` | list length, 0 if missing |
| `EXPIRE key seconds` | 1 if set, 0 if no such key |
| `TTL key` / `PTTL key` | seconds / milliseconds left; -1 no expiry, -2 no key |
| `PERSIST key` | 1 if an expiry was removed, else 0 |
| `SAVE` | `OK`, once the data is on disk |
| `BGSAVE` | `Background saving started`; the save runs in the background |
| `LASTSAVE` | Unix time of the last good save |
| `SHUTDOWN [NOSAVE \| SAVE]` | no reply; the server saves (unless NOSAVE) and stops |
| `INFO` | version, uptime, clients, commands processed, keys, last save |
| `CONFIG GET pattern [pattern ...]` | `[name, value]` pairs for matching settings |
| `AUTH password` / `AUTH default password` | `OK` |
| `COMMAND [...]` | an empty array (lets `redis-cli` start in interactive mode) |

Errors use the same text as Redis, for example
`WRONGTYPE Operation against a key holding the wrong kind of value`,
`ERR value is not an integer or out of range`, `NOAUTH Authentication required.` and
`ERR max number of clients reached`.

Commands can be pipelined: replies come back in order, in as few writes as possible. Plain
text commands also work, so `printf 'PING\r\n' | nc localhost 6379` answers `+PONG`.

## How data is kept safe

- `SAVE`, `BGSAVE`, the auto-save and shutdown all write `dump.gob.tmp` first, sync it to disk and
  then rename it over `dump.gob`. A crash during a save leaves the previous file whole.
- Only one save runs at a time. While it writes, clients keep working: the data is copied under
  a lock that is released before the slow disk write.
- On start-up the file is loaded before any client can connect. Keys whose expiry passed while
  the server was down are skipped. A damaged file stops the server with a clear message instead
  of starting empty.
- Ctrl+C, SIGTERM and `SHUTDOWN` stop accepting clients, let running commands finish, save and
  exit. If that takes more than 10 seconds, the server gives up and exits.

## Limits

- Bulk strings over 512 MB and commands with more than 1,048,576 parts are refused from their
  header, before any of the data is read. The client gets `ERR Protocol error` and is disconnected.
- Bytes that aren't valid RESP get the same treatment. Only that client is affected.
- Passwords are compared in constant time.

## Speed

Measured with `redis-benchmark -t set,get -n 100000 -q` (50 clients) against real Redis 7.2 on the
same machine (WSL2, 8 CPUs), middle of three runs.

| Test | Real Redis (req/s) | Gopherdis (req/s) | Gopherdis vs Redis |
|------|-------------------:|------------------:|-------------------:|
| SET | 124,378 | 74,405 | 60% |
| GET | 131,406 | 70,423 | 54% |
| SET, 16 per batch (`-P 16`) | 724,638 | 769,231 | 106% |
| GET, 16 per batch (`-P 16`) | 806,452 | 884,956 | 110% |

One command per round trip is limited by system calls and goroutine wake-ups more than by the
code. With pipelining that cost is shared by 16 commands, and Gopherdis can use several cores.
`make bench` runs the same test against a fresh server (it needs `redis-benchmark`).
[PROGRESS.md](PROGRESS.md) has the full measurements and profiling notes.

## What it doesn't do (compared with Redis)

- Data types: only strings and lists. No hashes, sets, sorted sets, streams, bitmaps or
  HyperLogLog.
- Only some commands: no `LPOP`, `RPOP`, `INCRBY`, `APPEND`, `SCAN`, `TYPE`, `RENAME`, ... (see the
  table above for what exists).
- One database: no `SELECT`.
- No append-only file (AOF), no replication, no cluster, no Pub/Sub, no transactions
  (`MULTI`/`EXEC`), no Lua scripting, no memory limit or eviction.
- RESP2 only: `HELLO 3` is answered with an error and clients fall back to RESP2.
- Only the `default` user: no ACLs.
- `SET` has no `NX`, `XX`, `GET` or `KEEPTTL` options. `EXPIRE` has no `NX`/`XX`/`GT`/`LT`.
- `CONFIG SET` isn't supported; settings come from flags only.
- Plain-text (inline) commands are split on spaces only; quoted arguments aren't understood.
- The save file is Gopherdis' own format (Go's `encoding/gob`), not Redis' RDB.

## Development

```bash
make test     # go test -race ./...
make vet      # go vet ./...
make bench    # redis-benchmark against a fresh server
```

| Folder | What it holds |
|--------|---------------|
| `cmd/gopherdis` | `main`: flags, start-up, signals, shutdown |
| `internal/resp` | reading and writing RESP messages |
| `internal/server` | listening, one goroutine per client, pipelining, limits |
| `internal/commands` | one handler per command |
| `internal/store` | the in-memory data, lists, expiry |
| `internal/persist` | saving and loading the dump file |
| `tests` | end-to-end tests through a real Redis client |

CI (GitHub Actions) runs `gofmt`, `go vet`, `go test -race` and a build on every push, and
builds the Docker image and checks that it keeps data in `/data`.

[PROGRESS.md](PROGRESS.md) records how the project was built, step by step.

## Credits

Based on the [Build Your Own Redis Server](https://codingchallenges.fyi/challenges/challenge-redis)
challenge by John Crickett (Coding Challenges).

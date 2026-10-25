BINARY  := bin/gopherdis
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)
IMAGE   ?= gopherdis

# Port the bench target starts the server on; change it if it's taken.
BENCH_PORT ?= 6390

.PHONY: build test vet run bench docker

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/gopherdis

test:
	go test -race ./...

vet:
	go vet ./...

run: build
	./$(BINARY)

# The Step 8 speed test: starts a fresh server, runs redis-benchmark, stops it.
bench: build
	@dir=$$(mktemp -d); \
	./$(BINARY) --port $(BENCH_PORT) --dir $$dir --save-interval 0 --loglevel warn & pid=$$!; \
	sleep 1; \
	redis-benchmark -p $(BENCH_PORT) -t set,get -n 100000 -q; \
	redis-benchmark -p $(BENCH_PORT) -t set,get -n 100000 -P 16 -q; \
	kill $$pid; wait $$pid; rm -rf $$dir

docker:
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE) .

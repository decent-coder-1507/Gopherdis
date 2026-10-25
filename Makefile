BINARY := bin/gopherdis

.PHONY: build test run vet

build:
	go build -o $(BINARY) ./cmd/gopherdis

test:
	go test -race ./...

run:
	go run ./cmd/gopherdis

vet:
	go vet ./...

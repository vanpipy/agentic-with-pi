# awp — Makefile

VERSION  ?= 0.0.0-dev
COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
DATE     := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
BINARY   := awp
PKG      := ./cmd/awp
LDFLAGS  := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: all build release test test-race lint fmt tidy clean install run

all: build

build:
	go build -o $(BINARY) $(PKG)

release:
	go build -ldflags="$(LDFLAGS)" -o $(BINARY) $(PKG)

test:
	go test -count=1 -timeout=90s ./test/...

test-race:
	go test -count=1 -race -timeout=120s ./test/...

lint:
	go vet ./...
	go test -count=1 -race -timeout=120s ./test/...

fmt:
	gofmt -w .

tidy:
	go mod tidy

clean:
	rm -f $(BINARY)

install: build
	install -m 0755 $(BINARY) $(GOPATH)/bin/$(BINARY)

run: build
	./$(BINARY)

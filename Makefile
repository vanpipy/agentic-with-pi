# awp — Makefile

VERSION  ?= 0.0.0-dev
COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
DATE     := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
BINARY   := awp
PKG      := ./cmd/awp
LDFLAGS  := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: all build release test test-race lint coverage fmt tidy clean install run

all: build

build:
	go build -o $(BINARY) $(PKG)

release:
	go build -ldflags="$(LDFLAGS)" -o $(BINARY) $(PKG)

test:
	go test -count=1 -timeout=90s ./test/...

test-race:
	go test -count=1 -race -timeout=180s ./test/...

lint:
	gofmt -l .
	@if [ -n "$$(gofmt -l .)" ]; then echo "gofmt: files need formatting (run 'make fmt')"; exit 1; fi
	go vet ./...
	go test -count=1 -race -timeout=180s ./test/...

coverage:
	go test -count=1 -race -timeout=180s -coverpkg=./internal/... -coverprofile=coverage.out ./test/...
	go tool cover -func=coverage.out | tail -50
	@echo ""
	@echo "HTML report: coverage.html (open in browser)"

fmt:
	gofmt -w .

tidy:
	go mod tidy

clean:
	rm -f $(BINARY) coverage.out coverage.html

install: build
	install -m 0755 $(BINARY) $(GOPATH)/bin/$(BINARY)

run: build
	./$(BINARY)

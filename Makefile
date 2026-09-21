.PHONY: build test fmt fmt-check vet lint check hooks-install

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  = -s -w \
	-X github.com/newtosh/timeshare/internal/version.Version=$(VERSION) \
	-X github.com/newtosh/timeshare/internal/version.Commit=$(COMMIT) \
	-X github.com/newtosh/timeshare/internal/version.Date=$(DATE)

build:
	go build -ldflags "$(LDFLAGS)" -o bin/timeshare ./cmd/timeshare
	go build -ldflags "$(LDFLAGS)" -o bin/timesharedd ./cmd/timesharedd

test:
	go test -race -count=1 ./...

fmt:
	gofmt -w .

fmt-check:
	@out="$$(gofmt -l .)"; \
	if [ -n "$$out" ]; then \
		echo "gofmt needs to be run on:"; echo "$$out"; exit 1; \
	fi

vet:
	go vet ./...

lint:
	golangci-lint run

check: fmt-check vet lint test

hooks-install:
	lefthook install

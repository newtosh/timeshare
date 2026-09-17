.PHONY: build test fmt fmt-check vet lint check hooks-install

build:
	go build -o bin/timeshare ./cmd/timeshare
	go build -o bin/timesharedd ./cmd/timesharedd

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

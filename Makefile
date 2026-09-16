.PHONY: build test

build:
	go build -o bin/timeshare ./cmd/timeshare
	go build -o bin/timesharedd ./cmd/timesharedd

test:
	go test ./...

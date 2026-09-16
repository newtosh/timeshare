package main

import (
	"flag"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"

	"timeshare/internal/backend"
	"timeshare/internal/cache"
	"timeshare/internal/daemon"
)

func main() {
	sockPath := flag.String("socket", "", "unix socket path to listen on")
	flag.Parse()

	if *sockPath == "" {
		log.Fatal("timesharedd: --socket is required")
	}
	if err := os.MkdirAll(filepath.Dir(*sockPath), 0o700); err != nil {
		log.Fatalf("timesharedd: creating socket dir: %v", err)
	}
	os.Remove(*sockPath) // clear a stale socket from a previous crashed run

	ln, err := net.Listen("unix", *sockPath)
	if err != nil {
		log.Fatalf("timesharedd: listen: %v", err)
	}
	if err := os.Chmod(*sockPath, 0o600); err != nil {
		log.Fatalf("timesharedd: chmod socket: %v", err)
	}

	srv := &daemon.Server{
		Cache: cache.New(time.Now),
		// Backend dispatch by request.Mode is added in Task 11 once both
		// OnePassword backends exist; until then this daemon build is not
		// wired to a real backend selector.
		Backend: backend.Backend(nil),
	}

	log.Printf("timesharedd: listening on %s", *sockPath)
	log.Fatal(srv.Serve(ln))
}

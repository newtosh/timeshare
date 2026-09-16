package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"

	"timeshare/internal/backend"
	"timeshare/internal/cache"
	"timeshare/internal/config"
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
		Backend: &daemon.ModeDispatcher{
			Biometric: backend.NewOnePasswordBiometric(),
			ServiceAccount: func(token string) interface {
				Resolve(ctx context.Context, cfg config.Config, secretName string) (string, time.Duration, error)
			} {
				return backend.NewOnePasswordServiceAccount(token)
			},
			TokenForVault: func(vault string) (string, error) {
				return "", fmt.Errorf("service-account token storage not yet implemented")
			},
		},
	}

	log.Printf("timesharedd: listening on %s", *sockPath)
	log.Fatal(srv.Serve(ln))
}

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"

	"github.com/newtosh/timeshare/internal/backend"
	"github.com/newtosh/timeshare/internal/cache"
	"github.com/newtosh/timeshare/internal/config"
	"github.com/newtosh/timeshare/internal/daemon"
	"github.com/newtosh/timeshare/internal/tokenstore"
	"github.com/newtosh/timeshare/internal/version"
)

func main() {
	sockPath := flag.String("socket", "", "unix socket path to listen on")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.String())
		return
	}

	if *sockPath == "" {
		log.Fatal("timesharedd: --socket is required")
	}
	if err := os.MkdirAll(filepath.Dir(*sockPath), 0o700); err != nil {
		log.Fatalf("timesharedd: creating socket dir: %v", err)
	}

	// Guard the double-spawn race: two CLI invocations racing to spawn a
	// daemon must not both win. An exclusive, non-blocking flock on a
	// lockfile beside the socket means only one daemon ever holds the
	// socket at a time; a second one backs off instead of stealing it out
	// from under a first daemon that's still holding decrypted secrets in
	// memory.
	lockFile, err := os.OpenFile(*sockPath+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		log.Fatalf("timesharedd: opening lockfile: %v", err)
	}
	if err := unix.Flock(int(lockFile.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		log.Print("timesharedd: another daemon already holds the lock for this socket, exiting")
		return
	}
	defer func() { _ = lockFile.Close() }()

	_ = os.Remove(*sockPath) // clear a stale socket from a previous crashed run; ok if it never existed

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
			TokenForVault: tokenstore.Lookup,
		},
		IdleTimeout: 30 * time.Minute,
	}

	log.Printf("timesharedd: listening on %s", *sockPath)
	if err := srv.Serve(ln); err != nil {
		if errors.Is(err, daemon.ErrIdleTimeout) {
			log.Print("timesharedd: idle timeout reached, exiting")
			os.Exit(0)
		}
		log.Fatalf("timesharedd: %v", err)
	}
}

package daemon

import (
	"net"
	"testing"
	"time"

	"timeshare/internal/backend/backendtest"
	"timeshare/internal/cache"
)

func TestServerExitsAfterIdleTimeout(t *testing.T) {
	sockPath := t.TempDir() + "/agent.sock"
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	srv := &Server{
		Cache:       cache.New(time.Now),
		Backend:     &backendtest.Mock{},
		IdleTimeout: 100 * time.Millisecond,
	}

	done := make(chan error, 1)
	go func() { done <- srv.Serve(ln) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected Serve to return a sentinel idle-exit error, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not exit after IdleTimeout with no connections")
	}
}

// TestServerNeverExitsWithZeroIdleTimeout proves the documented contract
// that IdleTimeout: 0 (unset) disables self-exit entirely.
func TestServerNeverExitsWithZeroIdleTimeout(t *testing.T) {
	sockPath := t.TempDir() + "/agent.sock"
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	srv := &Server{
		Cache:   cache.New(time.Now),
		Backend: &backendtest.Mock{},
		// IdleTimeout left zero.
	}

	done := make(chan error, 1)
	go func() { done <- srv.Serve(ln) }()

	select {
	case err := <-done:
		t.Fatalf("Serve exited with IdleTimeout unset, want it to block indefinitely: %v", err)
	case <-time.After(200 * time.Millisecond):
		// Expected: Serve is still blocked. Closing the listener below
		// causes Serve's Accept loop to error out and return, cleaning up
		// the goroutine.
	}
}

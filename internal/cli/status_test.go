package cli

import (
	"context"
	"net"
	"testing"

	"github.com/newtosh/timeshare/internal/client"
)

func TestCheckDaemonReachableErrorsWhenUnreachable(t *testing.T) {
	c := &client.Client{SocketPath: t.TempDir() + "/agent.sock"}

	if err := checkDaemonReachable(context.Background(), c); err == nil {
		t.Fatal("expected an error when the daemon is unreachable")
	}
}

func TestCheckDaemonReachableSucceedsWhenReachable(t *testing.T) {
	sockPath := t.TempDir() + "/agent.sock"
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	c := &client.Client{SocketPath: sockPath}
	if err := checkDaemonReachable(context.Background(), c); err != nil {
		t.Fatalf("expected nil error when daemon is reachable, got: %v", err)
	}
}

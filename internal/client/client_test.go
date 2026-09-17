package client

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/newtosh/timeshare/internal/daemon"
)

// startFakeDaemon listens on a temp socket and answers exactly one request
// with a canned response, simulating an already-running timesharedd.
func startFakeDaemon(t *testing.T, resp daemon.Response) string {
	t.Helper()
	sockPath := t.TempDir() + "/agent.sock"
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		var req daemon.Request
		_ = daemon.ReadMessage(conn, &req)
		_ = daemon.WriteMessage(conn, resp)
	}()

	return sockPath
}

func TestReadAgainstLiveDaemon(t *testing.T) {
	sockPath := startFakeDaemon(t, daemon.Response{Value: "the-secret"})
	c := &Client{SocketPath: sockPath}

	val, err := c.Read(context.Background(), daemon.Request{SecretName: "X"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "the-secret" {
		t.Fatalf("got %q", val)
	}
}

func TestReadSurfacesDaemonError(t *testing.T) {
	sockPath := startFakeDaemon(t, daemon.Response{Error: "item not allowed"})
	c := &Client{SocketPath: sockPath}

	_, err := c.Read(context.Background(), daemon.Request{SecretName: "X"})
	if err == nil {
		t.Fatal("expected error from daemon response.Error")
	}
}

func TestReadFailsFastWithoutSpawnBinaryConfigured(t *testing.T) {
	c := &Client{SocketPath: "/tmp/definitely-not-a-real-socket-" + time.Now().Format("150405")}
	_, err := c.Read(context.Background(), daemon.Request{SecretName: "X"})
	if err == nil {
		t.Fatal("expected connection error when no daemon is running and DaemonBinary is unset")
	}
}

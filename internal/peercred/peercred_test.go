package peercred

import (
	"net"
	"testing"
)

// dialPair listens on network (unix path or "tcp"), accepts one connection,
// and returns (serverConn, clientConn). Caller must close both.
func dialPair(t *testing.T, network, addr string) (server, client net.Conn) {
	t.Helper()
	ln, err := net.Listen(network, addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	connCh := make(chan net.Conn, 1)
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			connCh <- conn
		}
	}()

	client, err = net.Dial(network, ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	server = <-connCh
	return server, client
}

// TestVerifyAcceptsSameUID proves the happy path: a same-process (hence
// same-UID) connection over a real unix socket is accepted, not
// rejected. Cross-UID rejection isn't tested here — a single-user CI
// container can't simulate a second UID connecting, matching the
// existing daemon peer-verification tests' own documented limitation.
func TestVerifyAcceptsSameUID(t *testing.T) {
	server, client := dialPair(t, "unix", t.TempDir()+"/peercred-test.sock")
	defer server.Close()
	defer client.Close()

	if err := Verify(server); err != nil {
		t.Fatalf("expected same-UID connection to be accepted, got: %v", err)
	}
}

func TestVerifyRejectsNonUnixConn(t *testing.T) {
	server, client := dialPair(t, "tcp", "127.0.0.1:0")
	defer server.Close()
	defer client.Close()

	if err := Verify(server); err == nil {
		t.Fatal("expected a non-unix-socket connection to be rejected")
	}
}

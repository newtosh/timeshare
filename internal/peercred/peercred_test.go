package peercred

import (
	"net"
	"testing"
)

// TestVerifyAcceptsSameUID proves the happy path: a same-process (hence
// same-UID) connection over a real unix socket is accepted, not
// rejected. Cross-UID rejection isn't tested here — a single-user CI
// container can't simulate a second UID connecting, matching the
// existing daemon peer-verification tests' own documented limitation.
func TestVerifyAcceptsSameUID(t *testing.T) {
	sockPath := t.TempDir() + "/peercred-test.sock"
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	connCh := make(chan net.Conn, 1)
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			connCh <- conn
		}
	}()

	client, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	server := <-connCh
	defer server.Close()

	if err := Verify(server); err != nil {
		t.Fatalf("expected same-UID connection to be accepted, got: %v", err)
	}
}

func TestVerifyRejectsNonUnixConn(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	connCh := make(chan net.Conn, 1)
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			connCh <- conn
		}
	}()

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	server := <-connCh
	defer server.Close()

	if err := Verify(server); err == nil {
		t.Fatal("expected a non-unix-socket connection to be rejected")
	}
}

//go:build darwin

package daemon

import (
	"net"
	"testing"
	"time"

	"timeshare/internal/backend/backendtest"
	"timeshare/internal/cache"
)

// TestVerifyPeerAcceptsSameUID proves the happy path end to end: a same-UID
// client's request is served rather than rejected by VerifyPeer. Cross-UID
// rejection is untested here for the same reason it's untested on Linux: a
// single-user CI container can't simulate a second UID connecting.
func TestVerifyPeerAcceptsSameUID(t *testing.T) {
	sockPath := t.TempDir() + "/agent.sock"
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	mock := &backendtest.Mock{ValueFor: map[string]string{"X": "v"}, TTL: time.Hour}
	srv := &Server{Cache: cache.New(time.Now), Backend: mock}
	go srv.Serve(ln)

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	req := Request{ProjectID: "p", SecretName: "X", AllowedItems: []string{"X"}, TTL: time.Hour}
	if err := WriteMessage(conn, req); err != nil {
		t.Fatal(err)
	}

	var resp Response
	if err := ReadMessage(conn, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != "" {
		t.Fatalf("expected same-UID request to succeed, got error: %s", resp.Error)
	}
	if resp.Value != "v" {
		t.Fatalf("got value %q", resp.Value)
	}
}

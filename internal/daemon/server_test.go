package daemon

import (
	"net"
	"testing"
	"time"

	"timeshare/internal/backend/backendtest"
	"timeshare/internal/cache"
	"timeshare/internal/config"
)

func dialServer(t *testing.T, srv *Server) net.Conn {
	t.Helper()
	sockPath := t.TempDir() + "/agent.sock"
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	go srv.Serve(ln)

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestResolvesAllowedItem(t *testing.T) {
	mock := &backendtest.Mock{
		ValueFor: map[string]string{"DATABASE_URL": "postgres://x"},
		TTL:      time.Hour,
	}
	srv := &Server{Cache: cache.New(time.Now), Backend: mock}
	conn := dialServer(t, srv)

	req := Request{
		ProjectID:    "proj1",
		SecretName:   "DATABASE_URL",
		Vault:        "v",
		Mode:         config.ModeServiceAccount,
		TTL:          time.Hour,
		AllowedItems: []string{"DATABASE_URL"},
	}
	if err := WriteMessage(conn, req); err != nil {
		t.Fatal(err)
	}

	var resp Response
	if err := ReadMessage(conn, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.Value != "postgres://x" {
		t.Fatalf("got value %q", resp.Value)
	}
	if len(mock.Calls) != 1 {
		t.Fatalf("expected exactly one backend call, got %d", len(mock.Calls))
	}
}

func TestRejectsItemNotInAllowList(t *testing.T) {
	mock := &backendtest.Mock{ValueFor: map[string]string{"SECRET": "value"}, TTL: time.Hour}
	srv := &Server{Cache: cache.New(time.Now), Backend: mock}
	conn := dialServer(t, srv)

	req := Request{
		ProjectID:    "proj1",
		SecretName:   "SECRET",
		AllowedItems: []string{"OTHER_ITEM"}, // SECRET not listed
		TTL:          time.Hour,
	}
	_ = WriteMessage(conn, req)

	var resp Response
	if err := ReadMessage(conn, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error == "" {
		t.Fatal("expected rejection error for item outside allow-list")
	}
	if len(mock.Calls) != 0 {
		t.Fatal("backend must not be called for a disallowed item")
	}
}

func TestSecondRequestIsServedFromCacheNotBackend(t *testing.T) {
	mock := &backendtest.Mock{ValueFor: map[string]string{"X": "v"}, TTL: time.Hour}
	srv := &Server{Cache: cache.New(time.Now), Backend: mock}

	req := Request{ProjectID: "p", SecretName: "X", AllowedItems: []string{"X"}, TTL: time.Hour}

	conn1 := dialServer(t, srv)
	_ = WriteMessage(conn1, req)
	var r1 Response
	_ = ReadMessage(conn1, &r1)

	conn2 := dialServer(t, srv)
	_ = WriteMessage(conn2, req)
	var r2 Response
	_ = ReadMessage(conn2, &r2)

	if len(mock.Calls) != 1 {
		t.Fatalf("expected backend called exactly once across both requests, got %d", len(mock.Calls))
	}
	if r1.Value != r2.Value {
		t.Fatalf("cached value mismatch: %q vs %q", r1.Value, r2.Value)
	}
}

func TestBackendErrorIsSurfacedNotCached(t *testing.T) {
	mock := &backendtest.Mock{Err: backendErrAuthFailed(t)}
	srv := &Server{Cache: cache.New(time.Now), Backend: mock}
	req := Request{ProjectID: "p", SecretName: "X", AllowedItems: []string{"X"}, TTL: time.Hour}

	conn := dialServer(t, srv)
	_ = WriteMessage(conn, req)
	var resp Response
	_ = ReadMessage(conn, &resp)

	if resp.Error == "" {
		t.Fatal("expected error surfaced to caller")
	}
	if _, ok := srv.Cache.Get("p\x00X"); ok {
		t.Fatal("a failed resolve must not populate the cache")
	}
}

func TestCrossProjectCacheIsolation(t *testing.T) {
	mock := &backendtest.Mock{ValueFor: map[string]string{"DATABASE_URL": "a-value"}, TTL: time.Hour}
	srv := &Server{Cache: cache.New(time.Now), Backend: mock}

	reqA := Request{ProjectID: "proj-a", SecretName: "DATABASE_URL", AllowedItems: []string{"DATABASE_URL"}, TTL: time.Hour}
	connA := dialServer(t, srv)
	_ = WriteMessage(connA, reqA)
	var respA Response
	_ = ReadMessage(connA, &respA)

	mock.ValueFor["DATABASE_URL"] = "b-value"
	reqB := Request{ProjectID: "proj-b", SecretName: "DATABASE_URL", AllowedItems: []string{"DATABASE_URL"}, TTL: time.Hour}
	connB := dialServer(t, srv)
	_ = WriteMessage(connB, reqB)
	var respB Response
	_ = ReadMessage(connB, &respB)

	if respA.Value == respB.Value {
		t.Fatal("expected different projects with same secret name to resolve independently")
	}
	if len(mock.Calls) != 2 {
		t.Fatalf("expected a backend call per distinct project, got %d", len(mock.Calls))
	}
}

func TestConfiguredTTLOverridesLongerBackendTTL(t *testing.T) {
	// Regression test for the resolve() precedence bug: a backend TTL of
	// an hour must not shadow a five-minute req.TTL (the config/--ttl
	// value). We drive a fake clock directly through the Cache (see
	// cache_test.go's pattern) rather than dialServer's hardcoded
	// cache.New(time.Now), since dialServer takes an already-built
	// *Server and never constructs the Cache itself.
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }

	mock := &backendtest.Mock{ValueFor: map[string]string{"X": "v"}, TTL: time.Hour}
	srv := &Server{Cache: cache.New(clock), Backend: mock}
	conn := dialServer(t, srv)

	req := Request{ProjectID: "p", SecretName: "X", AllowedItems: []string{"X"}, TTL: 5 * time.Minute}
	if err := WriteMessage(conn, req); err != nil {
		t.Fatal(err)
	}
	var resp Response
	if err := ReadMessage(conn, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}

	// Still within the 5-minute req.TTL: cache hit.
	now = now.Add(4 * time.Minute)
	if _, ok := srv.Cache.Get("p\x00X"); !ok {
		t.Fatal("expected cache hit before the shorter configured TTL elapses")
	}

	// Past the 5-minute req.TTL but well within the backend's 1-hour TTL:
	// if the backend TTL had won (the bug), this would still be a hit.
	now = now.Add(2 * time.Minute) // total 6 minutes since Set
	if _, ok := srv.Cache.Get("p\x00X"); ok {
		t.Fatal("expected entry expired at the shorter configured TTL, not the longer backend TTL")
	}
}

func TestStatusOpReturnsNoError(t *testing.T) {
	mock := &backendtest.Mock{ValueFor: map[string]string{"X": "v"}, TTL: time.Hour}
	srv := &Server{Cache: cache.New(time.Now), Backend: mock}
	conn := dialServer(t, srv)

	// Populate the cache first via a normal read.
	_ = WriteMessage(conn, Request{ProjectID: "p", SecretName: "X", AllowedItems: []string{"X"}, TTL: time.Hour})
	var r Response
	_ = ReadMessage(conn, &r)

	conn2 := dialServer(t, srv)
	_ = WriteMessage(conn2, Request{ProjectID: "p", SecretName: "X", AllowedItems: []string{"X"}, TTL: time.Hour, Op: OpStatus})
	var statusResp Response
	_ = ReadMessage(conn2, &statusResp)
	if statusResp.Error != "" {
		t.Fatalf("unexpected error: %s", statusResp.Error)
	}
}

func TestLockEvictsProjectCache(t *testing.T) {
	mock := &backendtest.Mock{ValueFor: map[string]string{"X": "v"}, TTL: time.Hour}
	srv := &Server{Cache: cache.New(time.Now), Backend: mock}

	conn := dialServer(t, srv)
	_ = WriteMessage(conn, Request{ProjectID: "p", SecretName: "X", AllowedItems: []string{"X"}, TTL: time.Hour})
	var r Response
	_ = ReadMessage(conn, &r)

	conn2 := dialServer(t, srv)
	_ = WriteMessage(conn2, Request{ProjectID: "p", AllowedItems: []string{"X"}, Op: OpLock})
	var lockResp Response
	_ = ReadMessage(conn2, &lockResp)
	if lockResp.Error != "" {
		t.Fatalf("unexpected error: %s", lockResp.Error)
	}

	if _, ok := srv.Cache.Get("p\x00X"); ok {
		t.Fatal("expected cache evicted after lock")
	}
}

func backendErrAuthFailed(t *testing.T) error {
	t.Helper()
	return errAuthFailedForTest
}

var errAuthFailedForTest = &testAuthError{}

type testAuthError struct{}

func (*testAuthError) Error() string { return "backend authentication failed" }

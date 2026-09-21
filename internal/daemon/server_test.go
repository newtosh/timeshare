package daemon

import (
	"net"
	"testing"
	"time"

	"github.com/newtosh/timeshare/internal/backend/backendtest"
	"github.com/newtosh/timeshare/internal/cache"
	"github.com/newtosh/timeshare/internal/config"
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
	if _, ok := srv.Cache.Get("p\x00X\x00"); ok {
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

func TestConfiguredTTLWins(t *testing.T) {
	// Configured/override TTL always wins over the backend suggestion —
	// shorter and longer. Fake clock via Cache (see cache_test.go).
	cases := []struct {
		name    string
		backend time.Duration
		req     time.Duration
		advance time.Duration
		wantHit bool
	}{
		{
			name:    "shorter configured expires before longer backend",
			backend: time.Hour,
			req:     5 * time.Minute,
			advance: 6 * time.Minute,
			wantHit: false,
		},
		{
			name:    "longer configured survives past shorter backend default",
			backend: 10 * time.Minute,
			req:     8 * time.Hour,
			advance: 30 * time.Minute,
			wantHit: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
			clock := func() time.Time { return now }
			mock := &backendtest.Mock{ValueFor: map[string]string{"X": "v"}, TTL: tc.backend}
			srv := &Server{Cache: cache.New(clock), Backend: mock}
			conn := dialServer(t, srv)

			req := Request{ProjectID: "p", SecretName: "X", AllowedItems: []string{"X"}, TTL: tc.req}
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

			now = now.Add(tc.advance)
			_, hit := srv.Cache.Get("p\x00X\x00")
			if hit != tc.wantHit {
				t.Fatalf("cache hit=%v, want %v after %v", hit, tc.wantHit, tc.advance)
			}
		})
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

	if _, ok := srv.Cache.Get("p\x00X\x00"); ok {
		t.Fatal("expected cache evicted after lock")
	}
}

func TestCacheKeyIncludesField(t *testing.T) {
	// Hand-editing field: notesPlain after a password resolve must not
	// reuse the cached password for the remainder of the TTL.
	mock := &backendtest.Mock{ValueFor: map[string]string{"TOKEN": "password-value"}, TTL: time.Hour}
	srv := &Server{Cache: cache.New(time.Now), Backend: mock}

	reqPassword := Request{ProjectID: "p", SecretName: "TOKEN", AllowedItems: []string{"TOKEN"}, TTL: time.Hour}
	conn := dialServer(t, srv)
	_ = WriteMessage(conn, reqPassword)
	var resp Response
	_ = ReadMessage(conn, &resp)
	if resp.Value != "password-value" {
		t.Fatalf("got %q", resp.Value)
	}

	mock.ValueFor["TOKEN"] = "notes-value"
	reqNotes := Request{ProjectID: "p", SecretName: "TOKEN", Field: "notesPlain", AllowedItems: []string{"TOKEN"}, TTL: time.Hour}
	conn2 := dialServer(t, srv)
	_ = WriteMessage(conn2, reqNotes)
	var resp2 Response
	_ = ReadMessage(conn2, &resp2)
	if resp2.Value != "notes-value" {
		t.Fatalf("field override reused password cache: got %q", resp2.Value)
	}
	if len(mock.Calls) != 2 {
		t.Fatalf("expected a backend call per field, got %d", len(mock.Calls))
	}
}

func backendErrAuthFailed(t *testing.T) error {
	t.Helper()
	return errAuthFailedForTest
}

var errAuthFailedForTest = &testAuthError{}

type testAuthError struct{}

func (*testAuthError) Error() string { return "backend authentication failed" }

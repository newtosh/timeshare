//go:build linux || darwin

package peercred

import "testing"

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

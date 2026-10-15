package cli

import "testing"

func TestReplaceEnvOverridesEarlierAssignment(t *testing.T) {
	got := replaceEnv([]string{"SSH_AUTH_SOCK=/old", "PATH=/bin", "SSH_AUTH_SOCK=/also-old"}, "SSH_AUTH_SOCK", "/proxy.sock")
	if len(got) != 2 || got[0] != "PATH=/bin" || got[1] != "SSH_AUTH_SOCK=/proxy.sock" {
		t.Fatalf("got %q", got)
	}
}

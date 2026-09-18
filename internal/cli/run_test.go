package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpstreamAgentSocketPathPrefersEnvVar(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "/tmp/custom-agent.sock")
	if got := upstreamAgentSocketPath(); got != "/tmp/custom-agent.sock" {
		t.Fatalf("got %q, want /tmp/custom-agent.sock", got)
	}
}

func TestUpstreamAgentSocketPathFallsBackTo1Password(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	want := filepath.Join(home, ".1password", "agent.sock")
	if got := upstreamAgentSocketPath(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSSHProxySocketDirPrefersXDGRuntimeDir(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	if got := sshProxySocketDir(); got != "/run/user/1000" {
		t.Fatalf("got %q, want /run/user/1000", got)
	}
}

func TestSSHProxySocketDirFallsBackToTempDir(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	if got := sshProxySocketDir(); got != os.TempDir() {
		t.Fatalf("got %q, want %q", got, os.TempDir())
	}
}

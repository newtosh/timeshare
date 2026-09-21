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

func TestUpstreamAgentSocketPathPrefersMacDesktopAgent(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	home := t.TempDir()
	t.Setenv("HOME", home)

	desktop := filepath.Join(home, "Library", "Group Containers", "2BUA8C4S2C.com.1password", "t", "agent.sock")
	if err := os.MkdirAll(filepath.Dir(desktop), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(desktop, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	// Also plant the CLI path — desktop must still win when both exist.
	cliPath := filepath.Join(home, ".1password", "agent.sock")
	if err := os.MkdirAll(filepath.Dir(cliPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cliPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if got := upstreamAgentSocketPath(); got != desktop {
		t.Fatalf("got %q, want desktop agent %q", got, desktop)
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

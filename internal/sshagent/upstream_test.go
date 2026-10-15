package sshagent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpstreamSocketPathPrefersEnvVar(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "/tmp/custom-agent.sock")
	if got := UpstreamSocketPath(); got != "/tmp/custom-agent.sock" {
		t.Fatalf("got %q, want /tmp/custom-agent.sock", got)
	}
}

func TestUpstreamSocketPathFallsBackTo1Password(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	want := filepath.Join(home, ".1password", "agent.sock")
	if got := UpstreamSocketPath(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestUpstreamSocketPathPrefersMacDesktopAgent(t *testing.T) {
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
	cliPath := filepath.Join(home, ".1password", "agent.sock")
	if err := os.MkdirAll(filepath.Dir(cliPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cliPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if got := UpstreamSocketPath(); got != desktop {
		t.Fatalf("got %q, want desktop agent %q", got, desktop)
	}
}

func TestProxySocketPathUsesRuntimeDir(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	got := ProxySocketPath("abc")
	want := "/run/user/1000/timeshare/ssh-abc.sock"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestProxySocketPathFallsBackWithoutXDG(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	got := ProxySocketPath("abc")
	if !filepath.IsAbs(got) || filepath.Base(got) != "ssh-abc.sock" {
		t.Fatalf("got %q", got)
	}
}

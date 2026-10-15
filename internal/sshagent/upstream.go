package sshagent

import (
	"fmt"
	"os"
	"path/filepath"
)

// UpstreamSocketPath returns where to reach the real SSH agent.
// Order: SSH_AUTH_SOCK if set, else the first existing well-known
// 1Password agent socket (macOS desktop Group Containers, then
// ~/.1password/agent.sock). If none exist yet, returns the traditional
// ~/.1password path so dial errors still name a concrete location.
func UpstreamSocketPath() string {
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		return sock
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	desktop := filepath.Join(home, "Library", "Group Containers", "2BUA8C4S2C.com.1password", "t", "agent.sock")
	cliPath := filepath.Join(home, ".1password", "agent.sock")
	for _, path := range []string{desktop, cliPath} {
		if st, err := os.Stat(path); err == nil && !st.IsDir() {
			return path
		}
	}
	return cliPath
}

// ProxySocketPath is the daemon-owned filtering proxy socket for one
// project. It lives for the grant TTL, not for a single run process.
// Example: $XDG_RUNTIME_DIR/timeshare/ssh-<projectid>.sock.
func ProxySocketPath(projectID string) string {
	return filepath.Join(runtimeDir(), "timeshare", "ssh-"+projectID+".sock")
}

func runtimeDir() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return dir
	}
	if dir := os.TempDir(); dir != "" {
		return filepath.Join(dir, fmt.Sprintf("timeshare-%d", os.Getuid()))
	}
	return filepath.Join("/tmp", fmt.Sprintf("timeshare-%d", os.Getuid()))
}

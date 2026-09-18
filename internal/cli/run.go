package cli

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/newtosh/timeshare/internal/client"
	"github.com/newtosh/timeshare/internal/daemon"
	"github.com/newtosh/timeshare/internal/sshagent"

	"golang.org/x/crypto/ssh/agent"

	"github.com/spf13/cobra"
)

func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run -- <command> [args...]",
		Short: "Resolve every item in .timeshare.yml and exec a subprocess with them injected as env vars",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			cfg, projectID, err := LoadProjectContext(cwd)
			if err != nil {
				return err
			}

			c := &client.Client{
				SocketPath:   client.DefaultSocketPath(),
				DaemonBinary: daemonBinaryPath(),
			}

			env := os.Environ()
			for _, name := range cfg.Items {
				value, err := c.Read(cmd.Context(), daemon.Request{
					ProjectID:    projectID,
					SecretName:   name,
					Vault:        cfg.Vault,
					Mode:         cfg.Mode,
					TTL:          cfg.TTL,
					AllowedItems: cfg.Items,
				})
				if err != nil {
					return fmt.Errorf("resolving %s: %w", name, err)
				}
				env = append(env, name+"="+value)
			}

			if len(cfg.SSHKeys) > 0 {
				sockPath, cleanup, err := startSSHProxy(cfg.SSHKeys, cfg.TTL)
				if err != nil {
					return fmt.Errorf("starting SSH key proxy: %w", err)
				}
				defer cleanup()
				env = append(env, "SSH_AUTH_SOCK="+sockPath)
			}

			child := exec.Command(args[0], args[1:]...) //nolint:gosec // args come from the user's own command line, exactly like `env`/`op run`
			child.Env = env
			child.Stdin = os.Stdin
			child.Stdout = os.Stdout
			child.Stderr = os.Stderr
			if err := child.Run(); err != nil {
				var ee *exec.ExitError
				if errors.As(err, &ee) {
					os.Exit(ee.ExitCode())
				}
				return err
			}
			return nil
		},
	}
}

// upstreamAgentSocketPath returns where to reach the real SSH agent: the
// standard SSH_AUTH_SOCK env var if set, else 1Password's own default
// agent socket path. Checking SSH_AUTH_SOCK first keeps this working for
// any agent, not just 1Password's; falling back to the well-known
// 1Password path covers users whose ssh config uses a per-host
// IdentityAgent override instead of exporting SSH_AUTH_SOCK at all (see
// the spec's "IdentityAgent precedence gotcha" section).
func upstreamAgentSocketPath() string {
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		return sock
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".1password", "agent.sock")
}

// sshProxySocketDir returns where to place the proxy's own temp socket:
// $XDG_RUNTIME_DIR if set (the Linux convention for exactly this kind of
// per-user runtime state), else the OS temp dir.
func sshProxySocketDir() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return dir
	}
	return os.TempDir()
}

// startSSHProxy resolves sshKeys to fingerprints, starts a filtering SSH
// agent proxy on a fresh temp socket bound by ttl, and returns the
// socket path plus a cleanup func that stops the proxy and removes the
// socket file. The caller must call cleanup exactly once (e.g. via
// defer).
func startSSHProxy(sshKeys []string, ttl time.Duration) (string, func(), error) {
	upstreamPath := upstreamAgentSocketPath()
	if upstreamPath == "" {
		return "", nil, fmt.Errorf("could not determine the upstream SSH agent socket path (set SSH_AUTH_SOCK)")
	}
	upstreamConn, err := net.Dial("unix", upstreamPath)
	if err != nil {
		return "", nil, fmt.Errorf("connecting to upstream SSH agent at %s: %w", upstreamPath, err)
	}

	fingerprints, err := sshagent.ResolveFingerprints(sshKeys)
	if err != nil {
		_ = upstreamConn.Close()
		return "", nil, err
	}

	proxy := sshagent.NewProxy(agent.NewClient(upstreamConn), fingerprints, time.Now().Add(ttl))

	sockPath := filepath.Join(sshProxySocketDir(), fmt.Sprintf("timeshare-ssh-%d-%d.sock", os.Getpid(), time.Now().UnixNano()))
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		_ = upstreamConn.Close()
		return "", nil, fmt.Errorf("creating SSH proxy socket: %w", err)
	}
	if err := os.Chmod(sockPath, 0o600); err != nil {
		_ = ln.Close()
		_ = upstreamConn.Close()
		return "", nil, fmt.Errorf("securing SSH proxy socket: %w", err)
	}

	go func() { _ = sshagent.Serve(ln, proxy) }()

	cleanup := func() {
		_ = ln.Close()
		_ = upstreamConn.Close()
		_ = os.Remove(sockPath)
	}
	return sockPath, cleanup, nil
}

package client

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/newtosh/timeshare/internal/daemon"
	"github.com/newtosh/timeshare/internal/trace"
)

// Client is the CLI's connection to timesharedd.
type Client struct {
	SocketPath string
	// DaemonBinary, if set, is exec'd (detached) to spawn the daemon when
	// SocketPath is unreachable. Left empty in tests that only exercise an
	// already-running fake daemon.
	DaemonBinary string
}

// DefaultSocketPath returns the platform-standard runtime path for the
// daemon socket. The path embeds a protocol generation so a CLI that
// speaks a newer request shape (e.g. per-item Field) never silently
// dials an older timesharedd that would ignore unknown JSON fields.
func DefaultSocketPath() string {
	const protocolGen = "p3" // bump when the request wire format gains a required op (ensure_ssh)
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "timeshare", protocolGen, "agent.sock")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("timeshare-%d", os.Getuid()), protocolGen, "agent.sock")
}

// Do sends req to the daemon, spawning it first if the socket is
// unreachable and DaemonBinary is configured, and returns the response.
// A non-empty Response.Error is returned as an error; the response is
// still populated so callers can read fields alongside the failure.
func (c *Client) Do(ctx context.Context, req daemon.Request) (daemon.Response, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return daemon.Response{}, fmt.Errorf("connecting to timesharedd: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if err := daemon.WriteMessage(conn, req); err != nil {
		return daemon.Response{}, fmt.Errorf("sending request: %w", err)
	}

	var resp daemon.Response
	if err := daemon.ReadMessage(conn, &resp); err != nil {
		return daemon.Response{}, fmt.Errorf("reading response: %w", err)
	}
	if resp.Error != "" {
		return resp, errors.New(resp.Error)
	}
	return resp, nil
}

// Read sends req to the daemon and returns the resolved secret value.
func (c *Client) Read(ctx context.Context, req daemon.Request) (string, error) {
	resp, err := c.Do(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Value, nil
}

// PingSocket dials the daemon socket without spawning one if it's absent,
// for diagnostic use (timeshare doctor).
func (c *Client) PingSocket(ctx context.Context) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "unix", c.SocketPath)
}

func (c *Client) dial(ctx context.Context) (net.Conn, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", c.SocketPath)
	if err == nil {
		return conn, nil
	}
	if c.DaemonBinary == "" {
		return nil, err
	}

	// A leftover socket file from a crashed daemon dials as "connection
	// refused". Unlink it before spawn so the new daemon's Listen isn't
	// racing a dead inode (timesharedd also Removes on start; this just
	// makes recovery reliable when spawn is about to run).
	if isConnRefused(err) {
		_ = os.Remove(c.SocketPath)
	}

	if spawnErr := c.spawnDaemon(); spawnErr != nil {
		return nil, fmt.Errorf("daemon unreachable and spawn failed: %w", spawnErr)
	}

	// Poll briefly for the freshly spawned daemon's socket to come up.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err = d.DialContext(ctx, "unix", c.SocketPath)
		if err == nil {
			return conn, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil, fmt.Errorf("daemon did not become reachable after spawn: %w", err)
}

func isConnRefused(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		err = opErr.Err
	}
	return errors.Is(err, syscall.ECONNREFUSED)
}

func (c *Client) spawnDaemon() error {
	if err := os.MkdirAll(filepath.Dir(c.SocketPath), 0o700); err != nil {
		return err
	}
	cmd := exec.Command(c.DaemonBinary, "--socket", c.SocketPath) //nolint:gosec // DaemonBinary is admin-configured (defaults to the daemon installed alongside this binary), not attacker-controlled input
	// Detach: new session, no controlling terminal, so the daemon outlives
	// this short-lived CLI process (ssh-agent-style, spec: Key decisions).
	cmd.SysProcAttr = detachedSysProcAttr()
	cmd.Stdout = nil
	// Inherit stderr so a sign failure or a debug trace is visible in the
	// terminal that started the daemon. Routine logs go through trace and
	// stay quiet unless tracing is on.
	cmd.Stderr = os.Stderr
	if trace.Enabled() && !trace.EnvEnabled() {
		cmd.Env = append(os.Environ(), "TIMESHARE_DEBUG=1")
	}
	return cmd.Start()
}

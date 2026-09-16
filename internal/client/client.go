package client

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"timeshare/internal/daemon"
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
// daemon socket.
func DefaultSocketPath() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "timeshare", "agent.sock")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("timeshare-%d", os.Getuid()), "agent.sock")
}

// Read sends req to the daemon, spawning it first if the socket is
// unreachable and DaemonBinary is configured, and returns the resolved
// secret value.
func (c *Client) Read(ctx context.Context, req daemon.Request) (string, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return "", fmt.Errorf("connecting to timesharedd: %w", err)
	}
	defer conn.Close()

	if err := daemon.WriteMessage(conn, req); err != nil {
		return "", fmt.Errorf("sending request: %w", err)
	}

	var resp daemon.Response
	if err := daemon.ReadMessage(conn, &resp); err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}
	if resp.Error != "" {
		return "", errors.New(resp.Error)
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

func (c *Client) spawnDaemon() error {
	if err := os.MkdirAll(filepath.Dir(c.SocketPath), 0o700); err != nil {
		return err
	}
	cmd := exec.Command(c.DaemonBinary, "--socket", c.SocketPath)
	// Detach: new session, no controlling terminal, so the daemon outlives
	// this short-lived CLI process (ssh-agent-style, spec: Key decisions).
	cmd.SysProcAttr = detachedSysProcAttr()
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Start()
}

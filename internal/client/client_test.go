package client

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/newtosh/timeshare/internal/daemon"

	"golang.org/x/sys/unix"
)

// startFakeDaemon listens on a temp socket and answers exactly one request
// with a canned response, simulating an already-running timesharedd.
func startFakeDaemon(t *testing.T, resp daemon.Response) string {
	t.Helper()
	sockPath := t.TempDir() + "/agent.sock"
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		var req daemon.Request
		_ = daemon.ReadMessage(conn, &req)
		_ = daemon.WriteMessage(conn, resp)
	}()

	return sockPath
}

func TestReadAgainstLiveDaemon(t *testing.T) {
	sockPath := startFakeDaemon(t, daemon.Response{Value: "the-secret"})
	c := &Client{SocketPath: sockPath}

	val, err := c.Read(context.Background(), daemon.Request{SecretName: "X"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "the-secret" {
		t.Fatalf("got %q", val)
	}
}

func TestReadSurfacesDaemonError(t *testing.T) {
	sockPath := startFakeDaemon(t, daemon.Response{Error: "item not allowed"})
	c := &Client{SocketPath: sockPath}

	_, err := c.Read(context.Background(), daemon.Request{SecretName: "X"})
	if err == nil {
		t.Fatal("expected error from daemon response.Error")
	}
}

func TestReadFailsFastWithoutSpawnBinaryConfigured(t *testing.T) {
	c := &Client{SocketPath: "/tmp/definitely-not-a-real-socket-" + time.Now().Format("150405")}
	_, err := c.Read(context.Background(), daemon.Request{SecretName: "X"})
	if err == nil {
		t.Fatal("expected connection error when no daemon is running and DaemonBinary is unset")
	}
}

func TestDialUnlinksStaleSocketBeforeSpawn(t *testing.T) {
	dir := t.TempDir()
	sockPath := dir + "/agent.sock"
	makeStaleUnixSocket(t, sockPath)

	// Spawn binary that records whether the socket path still existed
	// when it started, then exits. We only care that dial unlinked
	// before spawn — not that a real daemon comes up.
	probe := dir + "/probe.sh"
	marker := dir + "/was-gone"
	script := "#!/bin/sh\n" +
		"sock=\n" +
		"while [ $# -gt 0 ]; do\n" +
		"  case \"$1\" in --socket) sock=$2; shift 2;; *) shift;; esac\n" +
		"done\n" +
		"if [ ! -e \"$sock\" ]; then touch '" + marker + "'; fi\n"
	if err := os.WriteFile(probe, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	c := &Client{SocketPath: sockPath, DaemonBinary: probe}
	_, _ = c.dial(context.Background()) // expect eventual failure — probe exits

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("spawn probe did not see socket already unlinked")
}

// makeStaleUnixSocket binds a unix stream socket and closes the fd without
// unlinking, leaving a socket inode nothing listens on — the classic
// "daemon crashed" leftover that dials as ECONNREFUSED. net.Listen's
// Close would remove the file on this platform, so we use the raw syscall.
func makeStaleUnixSocket(t *testing.T, path string) {
	t.Helper()
	fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.Bind(fd, &unix.SockaddrUnix{Name: path}); err != nil {
		_ = unix.Close(fd)
		t.Fatal(err)
	}
	if err := unix.Close(fd); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected stale socket file to remain: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })
}

func TestDefaultSocketPathIncludesProtocolGen(t *testing.T) {
	// New request fields (e.g. Field) must not dial an older daemon that
	// ignores unknown JSON; the path generation isolates the wire formats.
	got := DefaultSocketPath()
	if !strings.Contains(got, string(filepath.Separator)+"p2"+string(filepath.Separator)) {
		t.Fatalf("DefaultSocketPath %q missing protocol generation segment", got)
	}
}

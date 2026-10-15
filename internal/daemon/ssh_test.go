package daemon

import (
	"bytes"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/newtosh/timeshare/internal/backend/backendtest"
	"github.com/newtosh/timeshare/internal/cache"
)

func TestEnsureSSHResolvesFingerprintOnceAcrossCalls(t *testing.T) {
	runtime := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtime)
	agentSock := filepath.Join(t.TempDir(), "upstream.sock")
	ln := listenHold(t, agentSock)
	defer ln.Close()
	t.Setenv("SSH_AUTH_SOCK", agentSock)

	var calls int
	srv := &Server{
		Cache: cache.New(time.Now),
		ResolveFingerprints: func(refs []string) ([]string, error) {
			calls++
			if len(refs) != 1 || refs[0] != "Private/deploy-key" {
				t.Errorf("refs = %v", refs)
			}
			return []string{"SHA256:cached-fingerprint"}, nil
		},
	}
	req := Request{
		Op:        OpEnsureSSH,
		ProjectID: "proj",
		TTL:       4 * time.Hour,
		SSHKeys:   []string{"Private/deploy-key"},
	}

	var first Response
	for i := 0; i < 10; i++ {
		conn := dialServer(t, srv)
		if err := WriteMessage(conn, req); err != nil {
			t.Fatal(err)
		}
		var resp Response
		if err := ReadMessage(conn, &resp); err != nil {
			t.Fatal(err)
		}
		if resp.Error != "" {
			t.Fatalf("call %d: %s", i, resp.Error)
		}
		if resp.SocketPath == "" || resp.ExpiresAt.IsZero() {
			t.Fatalf("call %d: missing socket or expiry: %+v", i, resp)
		}
		if i == 0 {
			first = resp
		} else if resp.SocketPath != first.SocketPath || !resp.ExpiresAt.Equal(first.ExpiresAt) {
			t.Fatalf("call %d reused a different grant: %+v vs %+v", i, resp, first)
		}
	}
	if calls != 1 {
		t.Fatalf("fingerprint resolutions = %d, want 1", calls)
	}

	secret := "super-secret-value"
	if err := walkContains(runtime, secret); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureSSHConcurrentCallsResolveOnce(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	agentSock := filepath.Join(t.TempDir(), "upstream.sock")
	up := listenHold(t, agentSock)
	defer up.Close()
	t.Setenv("SSH_AUTH_SOCK", agentSock)

	var mu sync.Mutex
	n := 0
	srv := &Server{
		Cache: cache.New(time.Now),
		ResolveFingerprints: func(refs []string) ([]string, error) {
			mu.Lock()
			n++
			mu.Unlock()
			time.Sleep(40 * time.Millisecond)
			return []string{"SHA256:one"}, nil
		},
	}
	sock := filepath.Join(t.TempDir(), "daemon.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go srv.Serve(ln)

	req := Request{Op: OpEnsureSSH, ProjectID: "proj", TTL: time.Hour, SSHKeys: []string{"Private/k"}}
	errCh := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := dialReady(sock)
			if err != nil {
				errCh <- err
				return
			}
			defer conn.Close()
			if err := WriteMessage(conn, req); err != nil {
				errCh <- err
				return
			}
			var resp Response
			if err := ReadMessage(conn, &resp); err != nil {
				errCh <- err
				return
			}
			if resp.Error != "" || resp.SocketPath == "" {
				errCh <- fmt.Errorf("response %+v", resp)
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
	if n != 1 {
		t.Fatalf("fingerprint resolutions = %d, want 1", n)
	}
}

func dialReady(sock string) (net.Conn, error) {
	deadline := time.Now().Add(2 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		conn, err := net.Dial("unix", sock)
		if err == nil {
			return conn, nil
		}
		last = err
		time.Sleep(10 * time.Millisecond)
	}
	return nil, last
}

func TestEnsureSSHCachesFingerprintWhenUpstreamIsDown(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Setenv("SSH_AUTH_SOCK", filepath.Join(t.TempDir(), "missing.sock"))

	var calls int
	srv := &Server{
		Cache: cache.New(time.Now),
		ResolveFingerprints: func(refs []string) ([]string, error) {
			calls++
			return []string{"SHA256:cached"}, nil
		},
	}
	req := Request{Op: OpEnsureSSH, ProjectID: "proj", TTL: time.Hour, SSHKeys: []string{"Private/k"}}
	for i := 0; i < 2; i++ {
		conn := dialServer(t, srv)
		_ = WriteMessage(conn, req)
		var resp Response
		_ = ReadMessage(conn, &resp)
		if resp.Error == "" || !bytes.Contains([]byte(resp.Error), []byte("unreachable")) || !bytes.Contains([]byte(resp.Error), []byte("Retry is safe")) {
			t.Fatalf("call %d error = %q", i, resp.Error)
		}
	}
	if calls != 1 {
		t.Fatalf("fingerprint resolutions = %d, want 1 (approval is cached even if the agent is down)", calls)
	}
}

func TestEnsureSSHDismissedPromptIsNotCached(t *testing.T) {
	var calls int
	srv := &Server{
		Cache: cache.New(time.Now),
		ResolveFingerprints: func(refs []string) ([]string, error) {
			calls++
			return nil, fmt.Errorf("authorization prompt dismissed, please try again")
		},
	}
	req := Request{Op: OpEnsureSSH, ProjectID: "proj", TTL: time.Hour, SSHKeys: []string{"Private/k"}}
	conn := dialServer(t, srv)
	_ = WriteMessage(conn, req)
	var resp Response
	_ = ReadMessage(conn, &resp)
	if !bytes.Contains([]byte(resp.Error), []byte("prompt was dismissed")) || !bytes.Contains([]byte(resp.Error), []byte("Retry is safe")) {
		t.Fatalf("error = %q", resp.Error)
	}
	conn2 := dialServer(t, srv)
	_ = WriteMessage(conn2, req)
	var resp2 Response
	_ = ReadMessage(conn2, &resp2)
	if calls != 2 {
		t.Fatalf("a dismissed prompt must not open a pass, resolutions = %d", calls)
	}
}

func TestStatusAndLockCoverItemsAndSSH(t *testing.T) {
	runtime := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtime)
	agentSock := filepath.Join(t.TempDir(), "upstream.sock")
	ln := listenHold(t, agentSock)
	defer ln.Close()
	t.Setenv("SSH_AUTH_SOCK", agentSock)

	var calls int
	mock := &backendtest.Mock{ValueFor: map[string]string{"DATABASE_URL": "super-secret-value"}, TTL: time.Hour}
	srv := &Server{
		Cache:   cache.New(time.Now),
		Backend: mock,
		ResolveFingerprints: func(refs []string) ([]string, error) {
			calls++
			return []string{"SHA256:cached-fingerprint"}, nil
		},
	}

	conn := dialServer(t, srv)
	_ = WriteMessage(conn, Request{ProjectID: "proj", SecretName: "DATABASE_URL", AllowedItems: []string{"DATABASE_URL"}, TTL: 4 * time.Hour})
	var read Response
	_ = ReadMessage(conn, &read)
	if read.Value != "super-secret-value" {
		t.Fatalf("read: %+v", read)
	}

	conn = dialServer(t, srv)
	_ = WriteMessage(conn, Request{Op: OpEnsureSSH, ProjectID: "proj", TTL: 4 * time.Hour, SSHKeys: []string{"Private/deploy-key"}})
	var ssh Response
	_ = ReadMessage(conn, &ssh)
	if ssh.Error != "" {
		t.Fatal(ssh.Error)
	}

	conn = dialServer(t, srv)
	_ = WriteMessage(conn, Request{Op: OpStatus, ProjectID: "proj"})
	var status Response
	_ = ReadMessage(conn, &status)
	if status.Error != "" {
		t.Fatal(status.Error)
	}
	if len(status.Passes) != 2 {
		t.Fatalf("passes = %+v", status.Passes)
	}
	seen := map[string]bool{}
	for _, p := range status.Passes {
		seen[p.Kind+" "+p.Name] = true
		if p.ExpiresAt.IsZero() {
			t.Fatalf("pass %s has no expiry", p.Name)
		}
		if p.Kind == "item" && p.Name == "super-secret-value" {
			t.Fatal("status included the secret value")
		}
	}
	if !seen["item DATABASE_URL"] || !seen["ssh Private/deploy-key"] {
		t.Fatalf("passes = %+v", status.Passes)
	}

	if err := walkContains(runtime, "super-secret-value"); err != nil {
		t.Fatal(err)
	}

	conn = dialServer(t, srv)
	_ = WriteMessage(conn, Request{Op: OpLock, ProjectID: "proj"})
	var lock Response
	_ = ReadMessage(conn, &lock)
	if lock.Error != "" {
		t.Fatal(lock.Error)
	}

	conn = dialServer(t, srv)
	_ = WriteMessage(conn, Request{Op: OpStatus, ProjectID: "proj"})
	var after Response
	_ = ReadMessage(conn, &after)
	if len(after.Passes) != 0 {
		t.Fatalf("lock left passes: %+v", after.Passes)
	}

	conn = dialServer(t, srv)
	_ = WriteMessage(conn, Request{Op: OpEnsureSSH, ProjectID: "proj", TTL: 4 * time.Hour, SSHKeys: []string{"Private/deploy-key"}})
	var again Response
	_ = ReadMessage(conn, &again)
	if again.Error != "" {
		t.Fatal(again.Error)
	}
	if calls != 2 {
		t.Fatalf("lock should force a new approval, resolutions = %d", calls)
	}
}

func TestSSHGrantExpires(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	agentSock := filepath.Join(t.TempDir(), "upstream.sock")
	ln := listenHold(t, agentSock)
	defer ln.Close()
	t.Setenv("SSH_AUTH_SOCK", agentSock)

	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	var calls int
	srv := &Server{
		Cache: cache.New(func() time.Time { return now }),
		Now:   func() time.Time { return now },
		ResolveFingerprints: func(refs []string) ([]string, error) {
			calls++
			return []string{"SHA256:cached"}, nil
		},
	}
	req := Request{Op: OpEnsureSSH, ProjectID: "proj", TTL: time.Hour, SSHKeys: []string{"Private/k"}}
	conn := dialServer(t, srv)
	_ = WriteMessage(conn, req)
	var resp Response
	_ = ReadMessage(conn, &resp)
	if resp.Error != "" {
		t.Fatal(resp.Error)
	}
	now = now.Add(2 * time.Hour)
	conn = dialServer(t, srv)
	_ = WriteMessage(conn, req)
	_ = ReadMessage(conn, &resp)
	if resp.Error != "" {
		t.Fatal(resp.Error)
	}
	if calls != 2 {
		t.Fatalf("expired grant should resolve again, calls = %d", calls)
	}
}

func TestIdleDoesNotExitWhilePassIsLive(t *testing.T) {
	sockPath := t.TempDir() + "/agent.sock"
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	c := cache.New(time.Now)
	c.Set("p\x00X", "v", time.Hour)
	srv := &Server{
		Cache:       c,
		Backend:     &backendtest.Mock{},
		IdleTimeout: 40 * time.Millisecond,
	}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ln) }()

	select {
	case err := <-done:
		t.Fatalf("daemon exited while a pass was live: %v", err)
	case <-time.After(180 * time.Millisecond):
	}
}

func listenHold(t *testing.T, path string) net.Listener {
	t.Helper()
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 256)
				for {
					if _, err := c.Read(buf); err != nil {
						return
					}
				}
			}(c)
		}
	}()
	return ln
}

func walkContains(root, secret string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Type()&os.ModeSocket != 0 {
			return nil
		}
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if bytes.Contains(b, []byte(secret)) {
			return fmt.Errorf("secret written to %s", path)
		}
		return nil
	})
}

package cli

import (
	"bytes"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestTenRunsOneApproval is the issue #43 acceptance check: ten separate
// `timeshare run` processes share one fingerprint resolution and one
// secret read. A fake `op` and a fake agent stand in for 1Password.
func TestTenRunsOneApproval(t *testing.T) {
	bins := builtBins(t)
	runtime := t.TempDir()
	repo := t.TempDir()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	yml := "vault: Private\nmode: biometric\nttl: 4h\nitems: [DATABASE_URL]\nssh_keys: [Private/deploy-key]\n"
	if err := os.WriteFile(filepath.Join(repo, ".timeshare.yml"), []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}

	agentSock := filepath.Join(t.TempDir(), "upstream.sock")
	ln, err := net.Listen("unix", agentSock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go acceptAndHold(ln)

	opLog := filepath.Join(t.TempDir(), "op.log")
	if err := os.WriteFile(filepath.Join(bins, "op"), []byte(fakeOpScript), 0o755); err != nil {
		t.Fatal(err)
	}

	env := []string{
		"PATH=" + bins + ":/usr/bin:/bin",
		"HOME=" + home,
		"XDG_RUNTIME_DIR=" + runtime,
		"SSH_AUTH_SOCK=" + agentSock,
		"TIMESHARE_OP_LOG=" + opLog,
		"TIMESHARE_DEBUG=1",
		"TMPDIR=" + t.TempDir(),
	}
	sockPath := clientSocket(runtime)
	t.Cleanup(func() { killDaemon(sockPath) })

	bin := filepath.Join(bins, "timeshare")
	traceLog := stderrFile(t)
	var firstSock string
	for i := 0; i < 10; i++ {
		out, err := runCmd(bin, repo, env, traceLog, "run", "--", "/bin/sh", "-c", `printf %s "$SSH_AUTH_SOCK"`)
		if err != nil {
			t.Fatalf("run %d: %v\nstderr:\n%s", i, err, readFile(t, traceLog))
		}
		if i == 0 {
			firstSock = strings.TrimSpace(out)
			if firstSock == "" || !strings.Contains(firstSock, "ssh-") {
				t.Fatalf("run 0 socket = %q\nstderr:\n%s", out, readFile(t, traceLog))
			}
		} else if strings.TrimSpace(out) != firstSock {
			t.Fatalf("run %d socket %q, want %q", i, out, firstSock)
		}
	}
	traceText := readFile(t, traceLog)
	if !strings.Contains(traceText, "timeshare: op ") {
		t.Fatalf("trace missing op calls:\n%s", traceText)
	}
	if !strings.Contains(traceText, "ssh grant reused") {
		t.Fatalf("trace missing grant reuse:\n%s", traceText)
	}
	fp, reveal := opCounts(t, opLog)
	if fp != 1 || reveal != 1 {
		t.Fatalf("after 10 runs: fingerprint calls=%d reveal calls=%d\nlog:\n%s", fp, reveal, mustRead(t, opLog))
	}
	if err := diskHasSecret(runtime, "super-secret-value"); err != nil {
		t.Fatal(err)
	}
	if err := diskHasSecret(repo, "super-secret-value"); err != nil {
		t.Fatal(err)
	}

	statusOut, err := runCmd(bin, repo, env, traceLog, "status")
	if err != nil {
		t.Fatalf("status: %v\n%s\n%s", err, statusOut, readFile(t, traceLog))
	}
	for _, want := range []string{"active pass:", "item DATABASE_URL", "ssh Private/deploy-key", "expires "} {
		if !strings.Contains(statusOut, want) {
			t.Fatalf("status missing %q:\n%s", want, statusOut)
		}
	}
	if strings.Contains(statusOut, "super-secret-value") {
		t.Fatalf("status printed the secret:\n%s", statusOut)
	}

	lockOut, err := runCmd(bin, repo, env, traceLog, "lock")
	if err != nil {
		t.Fatalf("lock: %v\n%s", err, lockOut)
	}
	if !strings.Contains(lockOut, "Pass revoked") {
		t.Fatalf("lock output: %s", lockOut)
	}
	statusOut, err = runCmd(bin, repo, env, traceLog, "status")
	if err != nil {
		t.Fatalf("status after lock: %v\n%s", err, statusOut)
	}
	if !strings.Contains(statusOut, "no active pass") {
		t.Fatalf("status after lock:\n%s", statusOut)
	}

	if _, err := runCmd(bin, repo, env, traceLog, "run", "--", "/bin/sh", "-c", `printf %s "$SSH_AUTH_SOCK"`); err != nil {
		t.Fatal(err)
	}
	fp, reveal = opCounts(t, opLog)
	if fp != 2 || reveal != 2 {
		t.Fatalf("after lock: fingerprint calls=%d reveal calls=%d", fp, reveal)
	}
}

func TestDoctorGreenForSSHKeysOnly(t *testing.T) {
	bins := builtBins(t)
	runtime := t.TempDir()
	repo := t.TempDir()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	yml := "vault: Private\nmode: biometric\nttl: 4h\nssh_keys: [Private/deploy-key]\n"
	if err := os.WriteFile(filepath.Join(repo, ".timeshare.yml"), []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	agentSock := filepath.Join(t.TempDir(), "upstream.sock")
	ln, err := net.Listen("unix", agentSock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go acceptAndHold(ln)
	if err := os.WriteFile(filepath.Join(bins, "op"), []byte(fakeOpScript), 0o755); err != nil {
		t.Fatal(err)
	}
	env := []string{
		"PATH=" + bins + ":/usr/bin:/bin",
		"HOME=" + home,
		"XDG_RUNTIME_DIR=" + runtime,
		"SSH_AUTH_SOCK=" + agentSock,
		"TIMESHARE_OP_LOG=" + filepath.Join(t.TempDir(), "op.log"),
		"TMPDIR=" + t.TempDir(),
	}
	t.Cleanup(func() { killDaemon(clientSocket(runtime)) })

	traceLog := stderrFile(t)
	out, err := runCmd(filepath.Join(bins, "timeshare"), repo, env, traceLog, "doctor")
	combined := out + "\n" + readFile(t, traceLog)
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, combined)
	}
	if strings.Contains(combined, "✗") {
		t.Fatalf("doctor not green:\n%s", combined)
	}
	if !strings.Contains(combined, "daemon socket reachable") {
		t.Fatalf("doctor output:\n%s", combined)
	}
}

const fakeOpScript = `#!/bin/sh
printf '%s\n' "$*" >> "$TIMESHARE_OP_LOG"
case "$*" in
  *fingerprint*)
    printf '%s\n' '{"value":"SHA256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}'
    ;;
  *reveal*)
    printf '%s\n' '{"value":"super-secret-value"}'
    ;;
  *)
    echo "unexpected op invocation: $*" >&2
    exit 1
    ;;
esac
`

var (
	binsOnce sync.Once
	binsDir  string
	binsErr  error
)

func builtBins(t *testing.T) string {
	t.Helper()
	binsOnce.Do(func() {
		binsDir, binsErr = os.MkdirTemp("", "timeshare-bins-")
		if binsErr != nil {
			return
		}
		root, err := moduleRoot()
		if err != nil {
			binsErr = err
			return
		}
		for _, pkg := range []struct{ name, path string }{
			{"timeshare", "./cmd/timeshare"},
			{"timesharedd", "./cmd/timesharedd"},
		} {
			cmd := exec.Command("go", "build", "-o", filepath.Join(binsDir, pkg.name), pkg.path)
			cmd.Dir = root
			out, err := cmd.CombinedOutput()
			if err != nil {
				binsErr = fmt.Errorf("build %s: %w\n%s", pkg.name, err, out)
				return
			}
		}
	})
	if binsErr != nil {
		t.Fatal(binsErr)
	}
	return binsDir
}

func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above %s", dir)
		}
		dir = parent
	}
}

func stderrFile(t *testing.T) *os.File {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stderr-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func readFile(t *testing.T, f *os.File) string {
	t.Helper()
	b, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// runCmd executes timeshare. stderr must be an *os.File: the auto-spawned
// daemon inherits it and outlives this process, and a pipe would keep
// os/exec.Wait blocked until that daemon exits.
func runCmd(bin, dir string, env []string, stderr *os.File, args ...string) (string, error) {
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = env
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	return stdout.String(), err
}

func opCounts(t *testing.T, path string) (fingerprint, reveal int) {
	t.Helper()
	b := mustRead(t, path)
	for _, line := range strings.Split(b, "\n") {
		if strings.Contains(line, "fingerprint") {
			fingerprint++
		}
		if strings.Contains(line, "reveal") {
			reveal++
		}
	}
	return fingerprint, reveal
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func clientSocket(runtime string) string {
	return filepath.Join(runtime, "timeshare", "p3", "agent.sock")
}

func diskHasSecret(root, secret string) error {
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

func acceptAndHold(ln net.Listener) {
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
}

func killDaemon(sock string) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return
	}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		cmd, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if err != nil {
			continue
		}
		if bytes.Contains(cmd, []byte("timesharedd")) && bytes.Contains(cmd, []byte(sock)) {
			_ = syscall.Kill(pid, syscall.SIGTERM)
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(sock); err != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

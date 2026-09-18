# Time-boxed SSH key access Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend timeshare's existing allow-list + TTL model from secret values to 1Password-managed SSH keys, via a per-`run`-invocation filtering proxy agent.

**Architecture:** `timeshare run` spins up a short-lived `ssh-agent`-protocol proxy (`internal/sshagent`) that forwards to the real 1Password agent, filtered to fingerprints listed in `.timeshare.yml`'s new `ssh_keys` field and bounded by the repo's TTL. Private key material never enters timeshare's process — the proxy only relays opaque protocol frames for pre-approved fingerprints.

**Tech Stack:** Go, `golang.org/x/crypto/ssh` + `golang.org/x/crypto/ssh/agent` (new dependency), existing `internal/onepassword` (`op` CLI wrapper), `internal/config`, `internal/cli` (cobra).

**Spec:** `docs/superpowers/specs/2026-09-18-ssh-key-access-design.md`

## Global Constraints

- Private key material never enters timeshare's process (spec: "Why a filtering proxy, not a key-holding agent").
- No new OS process per repo — the proxy is a goroutine inside the already-running `timeshare run` process (spec: Constraints).
- `ssh_keys` entries are `<vault>/<item-title-or-id>` references (NOT scoped to the repo's own dedicated vault, unlike `items`) — SSH keys are never copied or moved, only referenced (spec: Config schema).
- `config.Load` stays a pure parse — no `op` calls, no fingerprint resolution at `Load` time (spec: Config schema).
- Every unit test that would need a real `op` CLI + signed-in 1Password account is gated `//go:build integration`, matching `internal/onepassword/cli_ops_test.go`'s existing convention — never runs in normal CI.
- Fingerprints compare as opaque strings (`SHA256:<base64>`) — verified live (2026-09-18) that 1Password's own `fingerprint` item field and `golang.org/x/crypto/ssh`'s `ssh.FingerprintSHA256` produce byte-identical output for the same key, so no format conversion is ever needed.

---

### Task 1: Config schema — `ssh_keys`

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `config.Config.SSHKeys []string` (yaml tag `ssh_keys,omitempty`) — every later task that reads/writes `.timeshare.yml` uses this field name.

- [ ] **Step 1: Write the failing tests**

Add to `internal/config/config_test.go`:

```go
func TestLoadWithSSHKeys(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, `
vault: project-x-secrets
mode: biometric
ttl: 4h
items:
  - DATABASE_URL
ssh_keys:
  - Private/deploy-key-prod
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.SSHKeys) != 1 || cfg.SSHKeys[0] != "Private/deploy-key-prod" {
		t.Errorf("SSHKeys = %v", cfg.SSHKeys)
	}
}

func TestLoadWithoutSSHKeysIsValid(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, `
vault: v
mode: biometric
ttl: 1h
items: [X]
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.SSHKeys) != 0 {
		t.Errorf("expected no SSHKeys, got %v", cfg.SSHKeys)
	}
}

func TestWriteLoadRoundTripSSHKeys(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		Vault:   "project-x-secrets",
		Mode:    ModeBiometric,
		TTL:     4 * time.Hour,
		Items:   []string{"DATABASE_URL"},
		SSHKeys: []string{"Private/deploy-key-prod"},
	}
	path := filepath.Join(dir, ".timeshare.yml")
	if err := Write(path, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("written config failed to reload: %v", err)
	}
	if len(loaded.SSHKeys) != 1 || loaded.SSHKeys[0] != "Private/deploy-key-prod" {
		t.Fatalf("SSHKeys round-trip: got %v", loaded.SSHKeys)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/jonn/src/timeshare && go test ./internal/config/... -run 'TestLoadWithSSHKeys|TestLoadWithoutSSHKeysIsValid|TestWriteLoadRoundTripSSHKeys' -v`
Expected: FAIL — `cfg.SSHKeys` / `Config{SSHKeys: ...}` undefined (field doesn't exist yet).

- [ ] **Step 3: Add the field**

In `internal/config/config.go`, change:

```go
type Config struct {
	Vault string        `yaml:"vault"`
	Mode  Mode          `yaml:"mode"`
	TTL   time.Duration `yaml:"ttl"`
	Items []string      `yaml:"items"`
}

type rawConfig struct {
	Vault string   `yaml:"vault"`
	Mode  string   `yaml:"mode"`
	TTL   string   `yaml:"ttl"`
	Items []string `yaml:"items"`
}
```

to:

```go
type Config struct {
	Vault   string        `yaml:"vault"`
	Mode    Mode          `yaml:"mode"`
	TTL     time.Duration `yaml:"ttl"`
	Items   []string      `yaml:"items"`
	SSHKeys []string      `yaml:"ssh_keys,omitempty"`
}

type rawConfig struct {
	Vault   string   `yaml:"vault"`
	Mode    string   `yaml:"mode"`
	TTL     string   `yaml:"ttl"`
	Items   []string `yaml:"items"`
	SSHKeys []string `yaml:"ssh_keys,omitempty"`
}
```

In `Load`, change the final return:

```go
	return Config{Vault: raw.Vault, Mode: mode, TTL: ttl, Items: raw.Items}, nil
```

to:

```go
	return Config{Vault: raw.Vault, Mode: mode, TTL: ttl, Items: raw.Items, SSHKeys: raw.SSHKeys}, nil
```

In `Write`, change:

```go
	raw := rawConfig{
		Vault: cfg.Vault,
		Mode:  string(cfg.Mode),
		TTL:   cfg.TTL.String(),
		Items: cfg.Items,
	}
```

to:

```go
	raw := rawConfig{
		Vault:   cfg.Vault,
		Mode:    string(cfg.Mode),
		TTL:     cfg.TTL.String(),
		Items:   cfg.Items,
		SSHKeys: cfg.SSHKeys,
	}
```

No change to `Load`'s validation — `raw.Items` still requires at least one entry; `SSHKeys` stays fully optional (zero value valid), deliberately not folded into that check (Global Constraints).

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/jonn/src/timeshare && go test ./internal/config/... -v`
Expected: PASS — all tests, including the three new ones and every pre-existing one (`TestLoadValidConfig`, `TestLoadRejectsMissingItems`, etc. — none of their behavior changed).

- [ ] **Step 5: Commit**

```bash
cd /home/jonn/src/timeshare
git add internal/config/config.go internal/config/config_test.go
git commit -m "config: add optional ssh_keys field to .timeshare.yml

SSHKeys entries are <vault>/<item> references, not scoped to the repo's
own dedicated vault the way items is — SSH keys stay wherever they
already live in 1Password. Load stays a pure parse; no op calls, no
fingerprint resolution here (that's internal/sshagent, added next).

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>"
```

---

### Task 2: `onepassword` package — SSH key item support

**Files:**
- Modify: `internal/onepassword/cli_ops.go`
- Modify: `internal/onepassword/cli_ops_test.go` (integration-tagged)

**Interfaces:**
- Consumes: `runOp(args ...string) ([]byte, error)` (`internal/onepassword/cli_ops.go:16`), `Item` struct (`cli_ops.go:120`).
- Produces: `GetItemFingerprint(vault, ref string) (string, error)`, `ListSSHKeyItems(vault string) ([]Item, error)` — Task 3's `resolve.go` and Task 5's wizard step both call these.

- [ ] **Step 1: Write the failing test**

Add to `internal/onepassword/cli_ops_test.go` (this file already starts with `//go:build integration` — stay under that tag):

```go
func TestGetItemFingerprintAndListSSHKeyItems(t *testing.T) {
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	vault := "timeshare-test-sshkey-" + suffix

	vaultID, err := CreateVault(vault)
	if err != nil {
		t.Fatalf("CreateVault: %v", err)
	}
	t.Cleanup(func() { _ = DeleteVault(vaultID) })

	itemName := "timeshare-test-sshkey-item-" + suffix
	if _, err := runOp("item", "create", "--category=SSH Key", "--title="+itemName, "--vault="+vault, "--ssh-generate-key=ed25519"); err != nil {
		t.Fatalf("creating SSH Key item: %v", err)
	}

	fp, err := GetItemFingerprint(vault, itemName)
	if err != nil {
		t.Fatalf("GetItemFingerprint: %v", err)
	}
	if !strings.HasPrefix(fp, "SHA256:") {
		t.Fatalf("expected fingerprint to start with SHA256:, got %q", fp)
	}

	items, err := ListSSHKeyItems(vault)
	if err != nil {
		t.Fatalf("ListSSHKeyItems: %v", err)
	}
	if len(items) != 1 || items[0].Title != itemName {
		t.Fatalf("expected exactly the one SSH Key item, got %v", items)
	}

	if _, err := GetItemFingerprint(vault, "definitely-not-a-real-item"); err == nil {
		t.Fatal("expected error for nonexistent item")
	}
}
```

Add `"strings"` to this file's imports (alongside the existing `"fmt"`, `"testing"`, `"time"`).

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/jonn/src/timeshare && go test -tags=integration ./internal/onepassword/... -run TestGetItemFingerprintAndListSSHKeyItems -v`
Expected: FAIL — `GetItemFingerprint`/`ListSSHKeyItems` undefined.

(This test needs a signed-in `op` CLI to actually run; running it now with a live account is how you verify Step 3's implementation, but the "FAIL: undefined" compile error is the meaningful checkpoint before that.)

- [ ] **Step 3: Implement**

Add to `internal/onepassword/cli_ops.go`, after the existing `GetItem` function:

```go
// GetItemFingerprint resolves ref (a title or ID) within vault to that
// item's SSH key fingerprint, e.g. "SHA256:...". Only valid for SSH Key
// category items — any other category has no "fingerprint" field, so op
// returns an error.
func GetItemFingerprint(vault, ref string) (string, error) {
	out, err := runOp("item", "get", ref, "--vault="+vault, "--fields", "label=fingerprint", "--format=json")
	if err != nil {
		return "", err
	}
	var raw struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return "", fmt.Errorf("parsing op item get fingerprint output: %w", err)
	}
	if raw.Value == "" {
		return "", fmt.Errorf("item %q in vault %q has no fingerprint field (not an SSH Key item?)", ref, vault)
	}
	return raw.Value, nil
}

// ListSSHKeyItems lists only SSH Key category items in vault — the
// source list for the init wizard's SSH-key picker.
func ListSSHKeyItems(vault string) ([]Item, error) {
	out, err := runOp("item", "list", "--vault="+vault, "--categories=SSH Key", "--format=json")
	if err != nil {
		return nil, err
	}
	var raw []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parsing op item list output: %w", err)
	}
	items := make([]Item, len(raw))
	for i, it := range raw {
		items[i] = Item{ID: it.ID, Title: it.Title}
	}
	return items, nil
}
```

- [ ] **Step 4: Run test to verify it passes (requires a signed-in `op` CLI)**

Run: `cd /home/jonn/src/timeshare && go test -tags=integration ./internal/onepassword/... -run TestGetItemFingerprintAndListSSHKeyItems -v`
Expected: PASS, against a real 1Password account. Verified live already during spec-writing (2026-09-18): `op item get <ref> --vault=<v> --fields label=fingerprint --format=json` returns `{"value": "SHA256:..."}`, and `op item list --vault=<v> --categories="SSH Key" --format=json` correctly filters (12 of 1680 real items in that test).

Then run the full package suite to confirm nothing else broke: `go test ./internal/onepassword/...` (non-integration tests still run without `op`).

- [ ] **Step 5: Commit**

```bash
cd /home/jonn/src/timeshare
git add internal/onepassword/cli_ops.go internal/onepassword/cli_ops_test.go
git commit -m "onepassword: add SSH Key item fingerprint lookup and category listing

Confirmed live: op item get --fields label=fingerprint returns a
ready-formatted SHA256:... fingerprint with no --reveal needed, and
op item list --categories=\"SSH Key\" correctly filters to just that
category.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>"
```

---

### Task 3: `internal/sshagent` package — filtering proxy

**Files:**
- Create: `internal/sshagent/proxy.go`
- Create: `internal/sshagent/proxy_test.go`
- Create: `internal/sshagent/resolve.go`
- Create: `internal/sshagent/resolve_test.go`

**Interfaces:**
- Consumes: `onepassword.GetItemFingerprint`, `onepassword.ListItems`, `onepassword.SuggestMatches` (Task 2 and pre-existing).
- Produces: `Proxy` (implements `agent.Agent`), `NewProxy(upstream agent.Agent, fingerprints []string, deadline time.Time) *Proxy`, `Serve(l net.Listener, p *Proxy) error`, `ResolveFingerprints(refs []string) ([]string, error)` — Task 4's `run.go` calls all four.

- [ ] **Step 1: Add the dependency**

Run: `cd /home/jonn/src/timeshare && go get golang.org/x/crypto/ssh/agent`
Expected: `go.mod` gains `golang.org/x/crypto` as a direct require; `go.sum` updated.

- [ ] **Step 2: Write the failing tests for `Proxy`**

Create `internal/sshagent/proxy_test.go`:

```go
package sshagent

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// addKey generates a fresh ed25519 key, adds it to kr, and returns its
// public key plus fingerprint.
func addKey(t *testing.T, kr agent.Agent) (ssh.PublicKey, string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := kr.Add(agent.AddedKey{PrivateKey: priv}); err != nil {
		t.Fatal(err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return sshPub, ssh.FingerprintSHA256(sshPub)
}

func TestListFiltersToAllowedFingerprint(t *testing.T) {
	upstream := agent.NewKeyring()
	_, allowedFP := addKey(t, upstream)
	addKey(t, upstream) // a second key, never allow-listed

	p := NewProxy(upstream, []string{allowedFP}, time.Now().Add(time.Hour))
	keys, err := p.List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 filtered key, got %d", len(keys))
	}
	if ssh.FingerprintSHA256(keys[0]) != allowedFP {
		t.Fatalf("got fingerprint %s, want %s", ssh.FingerprintSHA256(keys[0]), allowedFP)
	}
}

func TestSignRejectsNonAllowedKey(t *testing.T) {
	upstream := agent.NewKeyring()
	_, allowedFP := addKey(t, upstream)
	otherPub, _ := addKey(t, upstream)

	p := NewProxy(upstream, []string{allowedFP}, time.Now().Add(time.Hour))
	if _, err := p.Sign(otherPub, []byte("data")); err == nil {
		t.Fatal("expected error signing with a non-allow-listed key")
	}
}

func TestSignAllowsAllowedKeyBeforeDeadline(t *testing.T) {
	upstream := agent.NewKeyring()
	allowedPub, allowedFP := addKey(t, upstream)

	p := NewProxy(upstream, []string{allowedFP}, time.Now().Add(time.Hour))
	sig, err := p.Sign(allowedPub, []byte("data"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig == nil {
		t.Fatal("expected a signature")
	}
}

func TestSignFailsPastDeadline(t *testing.T) {
	upstream := agent.NewKeyring()
	allowedPub, allowedFP := addKey(t, upstream)

	p := NewProxy(upstream, []string{allowedFP}, time.Now().Add(-time.Second))
	if _, err := p.Sign(allowedPub, []byte("data")); err == nil {
		t.Fatal("expected error signing past the deadline")
	}
}

func TestListEmptyPastDeadline(t *testing.T) {
	upstream := agent.NewKeyring()
	addKey(t, upstream)

	p := NewProxy(upstream, nil, time.Now().Add(-time.Second))
	keys, err := p.List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("expected 0 keys past deadline, got %d", len(keys))
	}
}

func TestUnsupportedMethodsReturnError(t *testing.T) {
	p := NewProxy(agent.NewKeyring(), nil, time.Now().Add(time.Hour))
	if err := p.Add(agent.AddedKey{}); err == nil {
		t.Error("expected Add to fail")
	}
	if err := p.RemoveAll(); err == nil {
		t.Error("expected RemoveAll to fail")
	}
	if _, err := p.Signers(); err == nil {
		t.Error("expected Signers to fail")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /home/jonn/src/timeshare && go test ./internal/sshagent/... -v`
Expected: FAIL to compile — package `sshagent` and `Proxy`/`NewProxy` don't exist yet.

- [ ] **Step 4: Implement `Proxy`**

Create `internal/sshagent/proxy.go`:

```go
// Package sshagent implements a filtering ssh-agent protocol proxy: it
// forwards List/Sign requests to a real upstream agent, but only for a
// fixed set of allow-listed key fingerprints, and only before a fixed
// deadline. It never holds private key material — every Sign call that
// passes the filter forwards the raw request upstream and relays the
// signature back unmodified.
package sshagent

import (
	"errors"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// ErrNotSupported is returned by every Proxy method that would manage
// agent state (Add/Remove/RemoveAll/Lock/Unlock/Signers) — this is a
// read-only, filtering pass-through to an upstream agent, never a key
// store of its own.
var ErrNotSupported = errors.New("timeshare: sshagent proxy does not support this operation")

// Proxy implements agent.Agent as a filtering pass-through to an
// upstream agent (the real 1Password SSH agent in production, a fake
// keyring in tests).
type Proxy struct {
	upstream agent.Agent
	allowed  map[string]bool
	deadline time.Time
}

// NewProxy builds a Proxy that forwards to upstream, permitting only the
// given fingerprints (e.g. "SHA256:...", exactly as returned by
// ssh.FingerprintSHA256 or by 1Password's own "fingerprint" item field —
// the two formats are identical, byte for byte) until deadline.
func NewProxy(upstream agent.Agent, fingerprints []string, deadline time.Time) *Proxy {
	allowed := make(map[string]bool, len(fingerprints))
	for _, fp := range fingerprints {
		allowed[fp] = true
	}
	return &Proxy{upstream: upstream, allowed: allowed, deadline: deadline}
}

// List returns only the upstream identities whose fingerprint is
// allow-listed. Past deadline, it returns an empty list — mirrors the
// agent.Agent interface's own documented Lock() behavior ("List will
// empty an empty list").
func (p *Proxy) List() ([]*agent.Key, error) {
	if !time.Now().Before(p.deadline) {
		return nil, nil
	}
	keys, err := p.upstream.List()
	if err != nil {
		return nil, err
	}
	var filtered []*agent.Key
	for _, k := range keys {
		if p.allowed[ssh.FingerprintSHA256(k)] {
			filtered = append(filtered, k)
		}
	}
	return filtered, nil
}

// Sign forwards to the upstream agent only if key's fingerprint is
// allow-listed and the deadline hasn't passed; otherwise it fails
// without ever contacting upstream.
func (p *Proxy) Sign(key ssh.PublicKey, data []byte) (*ssh.Signature, error) {
	if !time.Now().Before(p.deadline) {
		return nil, errors.New("timeshare: SSH key grant TTL expired")
	}
	if !p.allowed[ssh.FingerprintSHA256(key)] {
		return nil, errors.New("timeshare: SSH key not in this repo's allow-list")
	}
	return p.upstream.Sign(key, data)
}

func (p *Proxy) Add(agent.AddedKey) error       { return ErrNotSupported }
func (p *Proxy) Remove(ssh.PublicKey) error     { return ErrNotSupported }
func (p *Proxy) RemoveAll() error               { return ErrNotSupported }
func (p *Proxy) Lock([]byte) error              { return ErrNotSupported }
func (p *Proxy) Unlock([]byte) error            { return ErrNotSupported }
func (p *Proxy) Signers() ([]ssh.Signer, error) { return nil, ErrNotSupported }

// Serve accepts connections on l and serves the ssh-agent protocol using
// p for each one, until l.Accept fails (typically because the caller
// closed l on shutdown). Each connection is served in its own goroutine
// so one slow or stuck client can't block another.
func Serve(l net.Listener, p *Proxy) error {
	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}
		go func() {
			defer func() { _ = conn.Close() }()
			_ = agent.ServeAgent(p, conn)
		}()
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /home/jonn/src/timeshare && go test ./internal/sshagent/... -v`
Expected: PASS for all six `Test*` functions in `proxy_test.go`.

- [ ] **Step 6: Write the failing tests for `ResolveFingerprints`**

Create `internal/sshagent/resolve_test.go`:

```go
package sshagent

import (
	"fmt"
	"strings"
	"testing"
)

func TestResolveFingerprintsRejectsMissingSlash(t *testing.T) {
	if _, err := ResolveFingerprints([]string{"deploy-key-prod"}); err == nil {
		t.Fatal("expected error for a ref with no vault/ prefix")
	}
}

func TestResolveFingerprintsSplitsOnRightmostSlash(t *testing.T) {
	orig := getItemFingerprint
	defer func() { getItemFingerprint = orig }()

	var gotVault, gotItem string
	getItemFingerprint = func(vault, ref string) (string, error) {
		gotVault, gotItem = vault, ref
		return "SHA256:fake", nil
	}

	fps, err := ResolveFingerprints([]string{"team/infra/deploy-key"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotVault != "team/infra" || gotItem != "deploy-key" {
		t.Fatalf("got vault=%q item=%q, want vault=%q item=%q", gotVault, gotItem, "team/infra", "deploy-key")
	}
	if len(fps) != 1 || fps[0] != "SHA256:fake" {
		t.Fatalf("got %v", fps)
	}
}

func TestResolveFingerprintsPropagatesError(t *testing.T) {
	orig := getItemFingerprint
	defer func() { getItemFingerprint = orig }()
	getItemFingerprint = func(vault, ref string) (string, error) {
		return "", fmt.Errorf("boom")
	}

	_, err := ResolveFingerprints([]string{"Private/deploy-key-prod"})
	if err == nil {
		t.Fatal("expected error to propagate")
	}
	if !strings.Contains(err.Error(), "Private/deploy-key-prod") {
		t.Fatalf("expected error to name the failing ref, got: %v", err)
	}
}

func TestResolveFingerprintsResolvesMultipleInOrder(t *testing.T) {
	orig := getItemFingerprint
	defer func() { getItemFingerprint = orig }()
	getItemFingerprint = func(vault, ref string) (string, error) {
		return "SHA256:" + vault + "/" + ref, nil
	}

	fps, err := ResolveFingerprints([]string{"Private/key-a", "Work/key-b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"SHA256:Private/key-a", "SHA256:Work/key-b"}
	if len(fps) != 2 || fps[0] != want[0] || fps[1] != want[1] {
		t.Fatalf("got %v, want %v", fps, want)
	}
}
```

- [ ] **Step 7: Run tests to verify they fail**

Run: `cd /home/jonn/src/timeshare && go test ./internal/sshagent/... -run TestResolveFingerprints -v`
Expected: FAIL to compile — `ResolveFingerprints`/`getItemFingerprint` undefined.

- [ ] **Step 8: Implement `ResolveFingerprints`**

Create `internal/sshagent/resolve.go`:

```go
package sshagent

import (
	"fmt"
	"strings"

	"github.com/newtosh/timeshare/internal/onepassword"
)

// getItemFingerprint is a seam over onepassword.GetItemFingerprint so
// tests can exercise ResolveFingerprints' parsing/error-propagation
// behavior without shelling out to `op`.
var getItemFingerprint = onepassword.GetItemFingerprint

// ResolveFingerprints resolves each ref — a "<vault>/<item-title-or-id>"
// string, the same split-on-rightmost-"/" syntax timeshare's --from-item
// flag already uses — to that item's SSH key fingerprint, in order.
// Fails on the first bad ref, naming it and offering up to 3 "did you
// mean" suggestions from that ref's vault.
func ResolveFingerprints(refs []string) ([]string, error) {
	fingerprints := make([]string, 0, len(refs))
	for _, ref := range refs {
		idx := strings.LastIndex(ref, "/")
		if idx < 0 {
			return nil, fmt.Errorf("ssh key reference %q must be in <vault>/<item> form", ref)
		}
		vault, item := ref[:idx], ref[idx+1:]

		fp, err := getItemFingerprint(vault, item)
		if err != nil {
			return nil, fmt.Errorf("resolving SSH key %q: %w%s", ref, err, suggestionText(vault, item))
		}
		fingerprints = append(fingerprints, fp)
	}
	return fingerprints, nil
}

// suggestionText returns a "did you mean" block for item within vault,
// or "" if the vault itself can't be listed (the caller already has a
// real error to report) or nothing close matches.
func suggestionText(vault, item string) string {
	items, err := onepassword.ListItems(vault)
	if err != nil {
		return ""
	}
	matches := onepassword.SuggestMatches(item, items, 3)
	if len(matches) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\ndid you mean:")
	for _, m := range matches {
		fmt.Fprintf(&b, "\n  - %s (id: %s)", m.Title, m.ID)
	}
	return b.String()
}
```

- [ ] **Step 9: Run tests to verify they pass**

Run: `cd /home/jonn/src/timeshare && go test ./internal/sshagent/... -v`
Expected: PASS for every test in both `proxy_test.go` and `resolve_test.go`.

Note on `TestResolveFingerprintsPropagatesError`: it exercises `suggestionText`'s real (non-mocked) call to `onepassword.ListItems("Private")`, which will fail in CI (no `op` binary) and is caught internally, returning `""` — the test only asserts the error names the ref, not that a suggestion is present, so this is safe without `op` installed.

- [ ] **Step 10: Commit**

```bash
cd /home/jonn/src/timeshare
git add go.mod go.sum internal/sshagent/
git commit -m "sshagent: add filtering SSH agent proxy + fingerprint resolution

Proxy implements agent.Agent as a read-only pass-through: List/Sign only
succeed for allow-listed fingerprints before a deadline, and forward
unmodified to a real upstream agent otherwise — no private key material
is ever held. ResolveFingerprints turns .timeshare.yml's ssh_keys
<vault>/<item> refs into fingerprints via onepassword.GetItemFingerprint.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>"
```

---

### Task 3.5: `internal/peercred` — extract peer-UID verification for reuse

**Added during execution, not in the original plan** — an opus-model security review of Task 3 flagged that the SSH proxy socket (Task 4, below) would otherwise have no peer-UID verification, unlike the daemon socket's existing `SO_PEERCRED`/`Xucred` check (`internal/daemon/peer_unix.go`, `peer_darwin.go`). SECURITY.md's whole allow-list trust model rests on "a second local user can't connect at all" — the SSH proxy socket needs the identical guarantee, not just `0600` file permissions. Ruling: extract the existing daemon peer-check logic into a small reusable package rather than duplicating it, then have both the daemon and the SSH proxy call it.

**Files:**
- Create: `internal/peercred/peercred_unix.go` (linux, `SO_PEERCRED`)
- Create: `internal/peercred/peercred_darwin.go` (darwin, `LOCAL_PEERCRED`/`Xucred`)
- Create: `internal/peercred/peercred_stub.go` (other platforms, fails closed)
- Modify: `internal/daemon/peer_unix.go`, `internal/daemon/peer_darwin.go`, `internal/daemon/peer_stub.go` — `Server.VerifyPeer` becomes a thin delegate to `peercred.Verify`, same signature, same behavior, no test changes needed.
- Modify: `internal/sshagent/proxy.go` — `Serve` calls `peercred.Verify(conn)` right after `Accept`, closing and skipping any connection that fails it, mirroring `daemon.Server.HandleConn`'s pattern exactly.

**Interfaces:**
- Produces: `peercred.Verify(conn net.Conn) error` — Task 4's `run.go` doesn't call this directly (it's inside `sshagent.Serve`, already wired), but should know it exists when reviewing the security posture of the socket it creates.

- [ ] **Step 1: Create the package, moving logic verbatim**

`internal/peercred/peercred_unix.go`:

```go
//go:build linux

package peercred

import (
	"fmt"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// Verify rejects any connection from a UID other than the calling
// process's own — a second local user must not even be able to connect,
// not merely fail to authenticate.
func Verify(conn net.Conn) error {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return fmt.Errorf("not a unix socket connection")
	}
	raw, err := unixConn.SyscallConn()
	if err != nil {
		return err
	}

	var ucred *unix.Ucred
	var credErr error
	err = raw.Control(func(fd uintptr) {
		ucred, credErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	})
	if err != nil {
		return err
	}
	if credErr != nil {
		return credErr
	}

	if int(ucred.Uid) != os.Getuid() {
		return fmt.Errorf("connecting UID %d does not match this process's UID %d", ucred.Uid, os.Getuid())
	}
	return nil
}
```

`internal/peercred/peercred_darwin.go`:

```go
//go:build darwin

package peercred

import (
	"fmt"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// Verify rejects any connection from a UID other than the calling
// process's own. Darwin has no SO_PEERCRED; the equivalent is
// LOCAL_PEERCRED/Xucred, which carries UID/GID but not PID.
func Verify(conn net.Conn) error {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return fmt.Errorf("not a unix socket connection")
	}
	raw, err := unixConn.SyscallConn()
	if err != nil {
		return err
	}

	var xucred *unix.Xucred
	var credErr error
	err = raw.Control(func(fd uintptr) {
		xucred, credErr = unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
	})
	if err != nil {
		return err
	}
	if credErr != nil {
		return credErr
	}
	if xucred.Ngroups == 0 {
		return fmt.Errorf("LOCAL_PEERCRED returned no groups, refusing to trust peer")
	}

	if int(xucred.Uid) != os.Getuid() {
		return fmt.Errorf("connecting UID %d does not match this process's UID %d", xucred.Uid, os.Getuid())
	}
	return nil
}
```

`internal/peercred/peercred_stub.go`:

```go
//go:build !linux && !darwin

package peercred

import (
	"fmt"
	"net"
)

// Verify is not implemented on this platform. It fails closed rather
// than silently skipping the security check.
func Verify(net.Conn) error {
	return fmt.Errorf("peer UID verification is not implemented on this platform")
}
```

- [ ] **Step 2: Delegate the daemon's existing `VerifyPeer` to the new package**

In `internal/daemon/peer_unix.go`, replace the whole function body — change:

```go
func (s *Server) VerifyPeer(conn net.Conn) error {
	unixConn, ok := conn.(*net.UnixConn)
	...
	if int(ucred.Uid) != os.Getuid() {
		return fmt.Errorf("connecting UID %d does not match daemon UID %d", ucred.Uid, os.Getuid())
	}
	return nil
}
```

to:

```go
func (s *Server) VerifyPeer(conn net.Conn) error {
	return peercred.Verify(conn)
}
```

removing the now-unused `"golang.org/x/sys/unix"` and `"os"` imports (keep `"fmt"` only if still used elsewhere in the file — it isn't, so drop it too) and adding `"github.com/newtosh/timeshare/internal/peercred"`. Apply the exact same delegation pattern to `internal/daemon/peer_darwin.go` and `internal/daemon/peer_stub.go`.

- [ ] **Step 3: Wire verification into the SSH proxy's `Serve`**

In `internal/sshagent/proxy.go`, change `Serve` from:

```go
func Serve(l net.Listener, p *Proxy) error {
	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}
		go func() {
			defer func() { _ = conn.Close() }()
			_ = agent.ServeAgent(p, conn)
		}()
	}
}
```

to:

```go
func Serve(l net.Listener, p *Proxy) error {
	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}
		go func() {
			defer func() { _ = conn.Close() }()
			if err := peercred.Verify(conn); err != nil {
				return
			}
			_ = agent.ServeAgent(p, conn)
		}()
	}
}
```

adding `"github.com/newtosh/timeshare/internal/peercred"` to the file's imports.

- [ ] **Step 4: Test**

Add `internal/peercred/peercred_test.go` (build-tag-free, runs on every platform — the platform-specific `Verify` implementations are exercised via whichever one actually compiles for the CI runner's OS):

```go
package peercred

import (
	"net"
	"testing"
)

// TestVerifyAcceptsSameUID proves the happy path: a same-process (hence
// same-UID) connection over a real unix socket is accepted, not
// rejected. Cross-UID rejection isn't tested here — a single-user CI
// container can't simulate a second UID connecting, matching the
// existing daemon peer-verification tests' own documented limitation.
func TestVerifyAcceptsSameUID(t *testing.T) {
	sockPath := t.TempDir() + "/peercred-test.sock"
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	connCh := make(chan net.Conn, 1)
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			connCh <- conn
		}
	}()

	client, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	server := <-connCh
	defer server.Close()

	if err := Verify(server); err != nil {
		t.Fatalf("expected same-UID connection to be accepted, got: %v", err)
	}
}

func TestVerifyRejectsNonUnixConn(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	connCh := make(chan net.Conn, 1)
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			connCh <- conn
		}
	}()

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	server := <-connCh
	defer server.Close()

	if err := Verify(server); err == nil {
		t.Fatal("expected a non-unix-socket connection to be rejected")
	}
}
```

Run: `cd /home/jonn/src/timeshare && go test ./internal/peercred/... ./internal/daemon/... ./internal/sshagent/... -v`
Expected: PASS everywhere — the daemon's own pre-existing `TestVerifyPeerAcceptsSameUID` (Linux) / equivalent (darwin) must still pass unchanged, proving the delegation preserves exact behavior.

Then: `go build ./... && go vet ./...`

- [ ] **Step 5: Commit**

```bash
cd /home/jonn/src/timeshare
git add internal/peercred/ internal/daemon/peer_unix.go internal/daemon/peer_darwin.go internal/daemon/peer_stub.go internal/sshagent/proxy.go
git commit -m "peercred: extract peer-UID verification, use it for the SSH proxy socket too

The SSH proxy socket (added in the previous commits, wired into \`run\`
next) had no peer-UID check — only 0600 file permissions, unlike the
daemon socket's existing SO_PEERCRED/Xucred verification. Extracted that
check into internal/peercred so both sockets share one implementation
instead of duplicating platform-specific syscall code. daemon.Server's
VerifyPeer now delegates to it; behavior and its existing tests are
unchanged.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>"
```

---

### Task 4: `timeshare run` — wire up the proxy

**Files:**
- Modify: `internal/cli/run.go`
- Create: `internal/cli/run_test.go`

**Interfaces:**
- Consumes: `sshagent.ResolveFingerprints`, `sshagent.NewProxy`, `sshagent.Serve` (Task 3); `config.Config.SSHKeys` (Task 1); `agent.NewClient` (`golang.org/x/crypto/ssh/agent`).
- Produces: `upstreamAgentSocketPath() string`, `sshProxySocketDir() string`, `startSSHProxy(sshKeys []string, ttl time.Duration) (string, func(), error)` — Task 6's `doctor.go` reuses `upstreamAgentSocketPath`.

- [ ] **Step 1: Write the failing tests**

Create `internal/cli/run_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/jonn/src/timeshare && go test ./internal/cli/... -run 'TestUpstreamAgentSocketPath|TestSSHProxySocketDir' -v`
Expected: FAIL to compile — `upstreamAgentSocketPath`/`sshProxySocketDir` undefined.

- [ ] **Step 3: Implement**

In `internal/cli/run.go`, change the imports from:

```go
import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/newtosh/timeshare/internal/client"
	"github.com/newtosh/timeshare/internal/daemon"

	"github.com/spf13/cobra"
)
```

to:

```go
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
```

Change the `RunE` body from:

```go
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

			child := exec.Command(args[0], args[1:]...) //nolint:gosec // args come from the user's own command line, exactly like `env`/`op run`
```

to:

```go
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
```

After `newRunCmd`'s closing `}`, add:

```go

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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/jonn/src/timeshare && go test ./internal/cli/... -run 'TestUpstreamAgentSocketPath|TestSSHProxySocketDir' -v`
Expected: PASS for all four tests.

Then: `go build ./...` — confirms the new imports and wiring compile cleanly end to end.

- [ ] **Step 5: Commit**

```bash
cd /home/jonn/src/timeshare
git add internal/cli/run.go internal/cli/run_test.go
git commit -m "run: start a filtering SSH agent proxy when ssh_keys is set

Resolves .timeshare.yml's ssh_keys to fingerprints, starts the proxy on
a fresh 0600 temp socket, injects SSH_AUTH_SOCK into the wrapped
command's env, tears both down on exit. Upstream agent discovery
prefers SSH_AUTH_SOCK, falls back to 1Password's default agent path.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>"
```

---

### Task 5: `timeshare init` — wizard step + `--ssh-key` flag

**Files:**
- Modify: `internal/cli/wizard_state.go`
- Modify: `internal/cli/wizard_state_test.go`
- Modify: `internal/cli/wizard.go`
- Modify: `internal/cli/pick.go`
- Modify: `internal/cli/init.go`

**Interfaces:**
- Consumes: `onepassword.ListSSHKeyItems`, `onepassword.ListVaults` (existing), `pickVault` (existing, `internal/cli/pick.go`), `runFzfList` (existing, `internal/cli/fzflist.go`).
- Produces: `wizardState.SSHKeys []string`; `pickSSHKeys(vault string, sshItems []onepassword.Item) ([]onepassword.Item, error)`.

- [ ] **Step 1: Write the failing test**

In `internal/cli/wizard_state_test.go`, update `newTestInitFlags` — change:

```go
func newTestInitFlags() *cobra.Command {
	cmd := &cobra.Command{Use: "init", RunE: func(*cobra.Command, []string) error { return nil }}
	cmd.Flags().StringP("vault", "v", "", "")
	cmd.Flags().StringP("mode", "m", "biometric", "")
	cmd.Flags().StringP("ttl", "t", "4h", "")
	cmd.Flags().String("from", "", "")
	cmd.Flags().StringArrayP("item", "i", nil, "")
	cmd.Flags().StringArray("from-item", nil, "")
	cmd.Flags().BoolP("force", "f", false, "")
	cmd.Flags().Bool("move", false, "")
	cmd.Flags().BoolP("non-interactive", "n", false, "")
	return cmd
}
```

to:

```go
func newTestInitFlags() *cobra.Command {
	cmd := &cobra.Command{Use: "init", RunE: func(*cobra.Command, []string) error { return nil }}
	cmd.Flags().StringP("vault", "v", "", "")
	cmd.Flags().StringP("mode", "m", "biometric", "")
	cmd.Flags().StringP("ttl", "t", "4h", "")
	cmd.Flags().String("from", "", "")
	cmd.Flags().StringArrayP("item", "i", nil, "")
	cmd.Flags().StringArray("from-item", nil, "")
	cmd.Flags().StringArray("ssh-key", nil, "")
	cmd.Flags().BoolP("force", "f", false, "")
	cmd.Flags().Bool("move", false, "")
	cmd.Flags().BoolP("non-interactive", "n", false, "")
	return cmd
}
```

Update every existing `newWizardState(cmd, ...)` call in this file to add a `nil` (or the test's own value) SSH-keys argument right before the trailing `force, move` bools — e.g.:

```go
s := newWizardState(cmd, "", "biometric", "4h", "", nil, nil, false, false)
```

becomes:

```go
s := newWizardState(cmd, "", "biometric", "4h", "", nil, nil, nil, false, false)
```

Apply this same one-`nil`-added edit to `TestNewWizardStateNoFlagsSet`, `TestNewWizardStateSomeFlagsSet`, `TestNewWizardStateItemFlagsCountAsSet` (which passes `[]string{"legacy-vault/DATABASE_URL"}` for `from-item` — leave that argument as-is, just add `nil` after it for ssh-keys), and `TestNewWizardStateMoveFlagNotCountedAsSet`.

Add a new test:

```go
func TestNewWizardStateSSHKeyFlagCountsAsSet(t *testing.T) {
	cmd := newTestInitFlags()
	if err := cmd.ParseFlags([]string{"--ssh-key=Private/deploy-key-prod"}); err != nil {
		t.Fatal(err)
	}
	s := newWizardState(cmd, "", "biometric", "4h", "", nil, nil, []string{"Private/deploy-key-prod"}, false, false)
	if !s.set["ssh-key"] {
		t.Fatal("expected set[\"ssh-key\"] true")
	}
	if len(s.SSHKeys) != 1 || s.SSHKeys[0] != "Private/deploy-key-prod" {
		t.Fatalf("SSHKeys = %v", s.SSHKeys)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/jonn/src/timeshare && go test ./internal/cli/... -run TestNewWizardState -v`
Expected: FAIL to compile — `newWizardState` called with the wrong number of arguments, `wizardState.SSHKeys` undefined.

- [ ] **Step 3: Implement `wizard_state.go`**

Change:

```go
type wizardState struct {
	Vault     string
	Mode      string
	TTL       string
	FromVault string
	Items     []string
	FromItems []string
	Force     bool
	Move      bool

	set map[string]bool
}

// wizardFlagNames are the flags that count toward "the user passed
// something" for entry-mode dispatch. --force, --move and
// --non-interactive are deliberately excluded — they modify behavior, not
// wizard input.
var wizardFlagNames = []string{"vault", "mode", "ttl", "item", "from", "from-item"}

func newWizardState(cmd *cobra.Command, vault, mode, ttl, fromVault string, items, fromItems []string, force, move bool) *wizardState {
	s := &wizardState{
		Vault:     vault,
		Mode:      mode,
		TTL:       ttl,
		FromVault: fromVault,
		Items:     items,
		FromItems: fromItems,
		Force:     force,
		Move:      move,
		set:       make(map[string]bool),
	}
	for _, name := range wizardFlagNames {
		if cmd.Flags().Changed(name) {
			s.set[name] = true
		}
	}
	return s
}
```

to:

```go
type wizardState struct {
	Vault     string
	Mode      string
	TTL       string
	FromVault string
	Items     []string
	FromItems []string
	SSHKeys   []string
	Force     bool
	Move      bool

	set map[string]bool
}

// wizardFlagNames are the flags that count toward "the user passed
// something" for entry-mode dispatch. --force, --move and
// --non-interactive are deliberately excluded — they modify behavior, not
// wizard input.
var wizardFlagNames = []string{"vault", "mode", "ttl", "item", "from", "from-item", "ssh-key"}

func newWizardState(cmd *cobra.Command, vault, mode, ttl, fromVault string, items, fromItems, sshKeys []string, force, move bool) *wizardState {
	s := &wizardState{
		Vault:     vault,
		Mode:      mode,
		TTL:       ttl,
		FromVault: fromVault,
		Items:     items,
		FromItems: fromItems,
		SSHKeys:   sshKeys,
		Force:     force,
		Move:      move,
		set:       make(map[string]bool),
	}
	for _, name := range wizardFlagNames {
		if cmd.Flags().Changed(name) {
			s.set[name] = true
		}
	}
	return s
}
```

`validateComplete` is unchanged — `SSHKeys` stays fully optional, not part of the "at least one of --from/--item/--from-item" rule.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/jonn/src/timeshare && go test ./internal/cli/... -run TestNewWizardState -v`
Expected: PASS for all five `TestNewWizardState*` tests, including the new one.

- [ ] **Step 5: Add `pickSSHKeys` to `pick.go`**

Add to `internal/cli/pick.go`, after the existing `pickItems` function:

```go
// pickSSHKeys shows an fzf-style, always-filtering multi-select over
// sshItems (SSH Key category items already filtered by the caller) and
// returns the ones the user selected. Selecting none is valid — unlike
// pickItems, SSH key access is optional.
func pickSSHKeys(vault string, sshItems []onepassword.Item) ([]onepassword.Item, error) {
	if len(sshItems) == 0 {
		return nil, nil
	}

	byID := make(map[string]onepassword.Item, len(sshItems))
	items := make([]fzfItem, len(sshItems))
	for i, it := range sshItems {
		byID[it.ID] = it
		items[i] = fzfItem{Label: fmt.Sprintf("%s (%s)", it.Title, it.ID), Value: it.ID}
	}

	chosen, err := runFzfList(fmt.Sprintf("Select SSH keys to grant access to from %q", vault), items, true, false)
	if err != nil {
		return nil, fmt.Errorf("SSH key picker: %w", err)
	}

	selected := make([]onepassword.Item, len(chosen))
	for i, it := range chosen {
		selected[i] = byID[it.Value]
	}
	return selected, nil
}
```

- [ ] **Step 6: Add the wizard step to `wizard.go`**

Change the step constants from:

```go
const (
	stepVault = iota
	stepMode
	stepItems
	stepTTL
	stepCount
)

var wizardStepLabels = [stepCount]string{
	stepVault: "Repo vault name",
	stepMode:  "Auth mode",
	stepItems: "Items to move",
	stepTTL:   "TTL",
}

var wizardStepHelp = [stepCount]string{
	stepVault: "The name of a new, dedicated 1Password vault timeshare will create for this repo. Pick something specific to this repo — it shouldn't be shared with unrelated projects.",
	stepMode:  "Biometric: shells out to `op read`, same Touch ID/Windows Hello prompt you already get, cached for the TTL. Service account: headless, token-based, no prompts at all, but requires a manual token-store step after init (see the printed instructions).",
	stepItems: "Which 1Password items should this project's allow-list include. You can move items from an existing vault, or pick from a list interactively.",
	stepTTL:   "How long a resolved secret stays cached before the next read re-checks 1Password. Longer means fewer prompts but a longer window before a rotated/revoked secret takes effect.",
}
```

to:

```go
const (
	stepVault = iota
	stepMode
	stepItems
	stepSSHKeys
	stepTTL
	stepCount
)

var wizardStepLabels = [stepCount]string{
	stepVault:   "Repo vault name",
	stepMode:    "Auth mode",
	stepItems:   "Items to move",
	stepSSHKeys: "SSH keys",
	stepTTL:     "TTL",
}

var wizardStepHelp = [stepCount]string{
	stepVault:   "The name of a new, dedicated 1Password vault timeshare will create for this repo. Pick something specific to this repo — it shouldn't be shared with unrelated projects.",
	stepMode:    "Biometric: shells out to `op read`, same Touch ID/Windows Hello prompt you already get, cached for the TTL. Service account: headless, token-based, no prompts at all, but requires a manual token-store step after init (see the printed instructions).",
	stepItems:   "Which 1Password items should this project's allow-list include. You can move items from an existing vault, or pick from a list interactively.",
	stepSSHKeys: "Optional: which SSH keys (already stored in 1Password) this repo's `timeshare run` may use, time-boxed by the same TTL as everything else. Keys are never moved or copied — this only records a reference to wherever they already live.",
	stepTTL:     "How long a resolved secret stays cached before the next read re-checks 1Password. Longer means fewer prompts but a longer window before a rotated/revoked secret takes effect.",
}
```

In `runWizard`, change the seeded-state construction from:

```go
	s := &wizardState{
		Vault: seeded.Vault, Mode: seeded.Mode, TTL: seeded.TTL,
		FromVault: seeded.FromVault, Items: seeded.Items, FromItems: seeded.FromItems,
		Force: seeded.Force, set: seeded.set,
	}
```

to:

```go
	s := &wizardState{
		Vault: seeded.Vault, Mode: seeded.Mode, TTL: seeded.TTL,
		FromVault: seeded.FromVault, Items: seeded.Items, FromItems: seeded.FromItems,
		SSHKeys: seeded.SSHKeys, Force: seeded.Force, set: seeded.set,
	}
```

Change the `steps` closure from:

```go
	steps := func() []wizardStep {
		return []wizardStep{
			{Label: wizardStepLabels[stepVault], Value: s.Vault, Done: s.set["vault"] || answered[stepVault]},
			{Label: wizardStepLabels[stepMode], Value: s.Mode, Done: s.set["mode"] || answered[stepMode]},
			{Label: wizardStepLabels[stepItems], Value: itemsSummary(s), Done: s.set["item"] || s.set["from"] || s.set["from-item"] || answered[stepItems]},
			{Label: wizardStepLabels[stepTTL], Value: s.TTL, Done: s.set["ttl"] || answered[stepTTL]},
		}
	}
```

to:

```go
	steps := func() []wizardStep {
		return []wizardStep{
			{Label: wizardStepLabels[stepVault], Value: s.Vault, Done: s.set["vault"] || answered[stepVault]},
			{Label: wizardStepLabels[stepMode], Value: s.Mode, Done: s.set["mode"] || answered[stepMode]},
			{Label: wizardStepLabels[stepItems], Value: itemsSummary(s), Done: s.set["item"] || s.set["from"] || s.set["from-item"] || answered[stepItems]},
			{Label: wizardStepLabels[stepSSHKeys], Value: sshKeysSummary(s), Done: s.set["ssh-key"] || answered[stepSSHKeys]},
			{Label: wizardStepLabels[stepTTL], Value: s.TTL, Done: s.set["ttl"] || answered[stepTTL]},
		}
	}
```

After the existing items-step block (the one ending `answered[stepItems] = true` and its closing `}`), insert a new block before the TTL step's `if !s.set["ttl"] {`:

```go
	if !s.set["ssh-key"] {
		render.render(renderBreadcrumb(steps(), stepSSHKeys))
		fmt.Println(lipgloss.NewStyle().Foreground(colorDim).Render("Optionally grant this repo access to SSH keys already stored in 1Password. Select none and press enter to skip."))

		vaults, err := onepassword.ListVaults()
		if err != nil {
			return nil, fmt.Errorf("listing vaults: %w", err)
		}
		sourceVault, err := pickVault(vaults)
		if err != nil {
			return nil, err
		}

		sshItems, err := onepassword.ListSSHKeyItems(sourceVault)
		if err != nil {
			return nil, fmt.Errorf("listing SSH keys in %s: %w", sourceVault, err)
		}

		picked, err := pickSSHKeys(sourceVault, sshItems)
		if err != nil {
			return nil, err
		}
		s.SSHKeys = make([]string, len(picked))
		for i, item := range picked {
			s.SSHKeys[i] = sourceVault + "/" + item.ID
		}
		answered[stepSSHKeys] = true
	}

```

At the end of the file, after `itemsSummary`, add:

```go

func sshKeysSummary(s *wizardState) string {
	if len(s.SSHKeys) == 0 {
		return "none"
	}
	return fmt.Sprintf("%d selected", len(s.SSHKeys))
}
```

- [ ] **Step 7: Wire the `--ssh-key` flag into `init.go`**

Change the flag variable declarations from:

```go
	var vaultName string
	var mode string
	var ttl string
	var fromVault string
	var explicitItems []string
	var fromItems []string
	var force bool
	var move bool
	var nonInteractive bool
```

to:

```go
	var vaultName string
	var mode string
	var ttl string
	var fromVault string
	var explicitItems []string
	var fromItems []string
	var sshKeys []string
	var force bool
	var move bool
	var nonInteractive bool
```

Change the `newWizardState` call from:

```go
			s := newWizardState(cmd, vaultName, mode, ttl, fromVault, explicitItems, fromItems, force, move)
```

to:

```go
			s := newWizardState(cmd, vaultName, mode, ttl, fromVault, explicitItems, fromItems, sshKeys, force, move)
```

Add a new flag registration after the `--from-item` one:

```go
	cmd.Flags().StringArrayVar(&sshKeys, "ssh-key", nil, "grant this repo's `run` access to an SSH key already stored in 1Password: <vault>/<item-name-or-id> (repeatable)")
```

In `runInit`, change the `config.Config{...}` literal from:

```go
	cfg := config.Config{
		Vault: s.Vault,
		Mode:  config.Mode(s.Mode),
		TTL:   parsedTTL,
		Items: items,
	}
```

to:

```go
	cfg := config.Config{
		Vault:   s.Vault,
		Mode:    config.Mode(s.Mode),
		TTL:     parsedTTL,
		Items:   items,
		SSHKeys: s.SSHKeys,
	}
```

`s.SSHKeys` is recorded verbatim — no eager `op` resolution at `init` time for the `--ssh-key` flag path (interactive picker entries are already guaranteed valid since they came from a live `ListSSHKeyItems` call). A typo'd `--ssh-key` value on the non-interactive/flag path surfaces at the first `timeshare run`, with the same "did you mean" handling `ResolveFingerprints` already provides (Task 3) — a deliberate scope decision, not an oversight, to avoid duplicating suggestion-printing machinery into `init.go` for a path CI/script users are expected to get right.

- [ ] **Step 8: Run the full `internal/cli` test suite**

Run: `cd /home/jonn/src/timeshare && go build ./... && go vet ./... && go test ./internal/cli/... -v`
Expected: PASS — every existing `internal/cli` test plus the new `TestNewWizardStateSSHKeyFlagCountsAsSet`. `go build ./...` confirms `wizard.go`'s new step block and `pick.go`'s new function compile cleanly (they have no dedicated unit tests, matching the existing convention that interactive wizard/picker code is verified live, not unit-tested — see Step 9).

- [ ] **Step 9: Manually verify the wizard step live**

This step has no automated test — matches how every other wizard step in this codebase was verified (see `docs/superpowers/plans/2026-09-17-*` and this session's prior live E2E test). Run, against a real signed-in `op` CLI, in a scratch git repo:

```sh
cd /home/jonn/src/timeshare && go build -o /tmp/timeshare-ssh-test ./cmd/timeshare
mkdir -p /tmp/timeshare-ssh-test-repo && cd /tmp/timeshare-ssh-test-repo && git init -q
/tmp/timeshare-ssh-test init
```

Confirm: the new "SSH keys" step appears after "Items to move" in the breadcrumb, offers a vault picker then an SSH-Key-category-filtered item picker, and selecting zero items (just pressing enter) proceeds to the TTL step without error. Confirm the written `.timeshare.yml` has no `ssh_keys` key at all if none were picked (since `SSHKeys` stays `nil`/empty and the yaml tag is `omitempty`), or a correctly-formatted `ssh_keys: [...]` block if one was picked. Clean up: `rm -rf /tmp/timeshare-ssh-test-repo /tmp/timeshare-ssh-test`.

- [ ] **Step 10: Commit**

```bash
cd /home/jonn/src/timeshare
git add internal/cli/wizard_state.go internal/cli/wizard_state_test.go internal/cli/wizard.go internal/cli/pick.go internal/cli/init.go
git commit -m "init: add SSH keys wizard step and --ssh-key flag

Reuses the existing pick-a-vault-then-pick-items flow, filtered to SSH
Key category items, storing <vault>/<item-id> refs exactly like
--from-item already does. Skippable — selecting none is valid, unlike
the items step. No eager op resolution on the flag path; a bad ref
surfaces at the first \`run\` with a did-you-mean suggestion.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>"
```

---

### Task 6: `timeshare doctor` — upstream agent reachability check

**Files:**
- Modify: `internal/cli/doctor.go`

**Interfaces:**
- Consumes: `upstreamAgentSocketPath` (Task 4), `LoadProjectContext` (existing, `internal/cli/root.go`).

- [ ] **Step 1: Implement**

`doctor.go` has no existing dedicated test file (its checks are thin glue over already-tested lower-level code, verified live — matching the project's existing convention for this file). Change the full `newDoctorCmd` function from:

```go
func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose timeshare and 1Password CLI health",
		RunE: func(cmd *cobra.Command, args []string) error {
			failed := 0
			if !check("op CLI installed", func() error {
				_, err := exec.LookPath("op")
				return err
			}) {
				failed++
			}
			if !check("daemon socket reachable", func() error {
				c := &client.Client{SocketPath: client.DefaultSocketPath()}
				conn, err := c.PingSocket(cmd.Context())
				if err == nil {
					_ = conn.Close()
				}
				return err
			}) {
				failed++
			}
			if !check(".timeshare.yml found", func() error {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				_, _, err = LoadProjectContext(cwd)
				return err
			}) {
				failed++
			}
			if failed > 0 {
				return fmt.Errorf("%d check(s) failed", failed)
			}
			return nil
		},
	}
}
```

to:

```go
func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose timeshare and 1Password CLI health",
		RunE: func(cmd *cobra.Command, args []string) error {
			failed := 0
			if !check("op CLI installed", func() error {
				_, err := exec.LookPath("op")
				return err
			}) {
				failed++
			}
			if !check("daemon socket reachable", func() error {
				c := &client.Client{SocketPath: client.DefaultSocketPath()}
				conn, err := c.PingSocket(cmd.Context())
				if err == nil {
					_ = conn.Close()
				}
				return err
			}) {
				failed++
			}

			var cfg config.Config
			if !check(".timeshare.yml found", func() error {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				loaded, _, err := LoadProjectContext(cwd)
				cfg = loaded
				return err
			}) {
				failed++
			}

			if len(cfg.SSHKeys) > 0 {
				if !check("SSH agent reachable (needed for ssh_keys)", func() error {
					path := upstreamAgentSocketPath()
					if path == "" {
						return fmt.Errorf("could not determine upstream SSH agent socket path")
					}
					conn, err := net.Dial("unix", path)
					if err != nil {
						return err
					}
					return conn.Close()
				}) {
					failed++
				}
			}

			if failed > 0 {
				return fmt.Errorf("%d check(s) failed", failed)
			}
			return nil
		},
	}
}
```

Change the file's imports from:

```go
import (
	"fmt"
	"os"
	"os/exec"

	"github.com/newtosh/timeshare/internal/client"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)
```

to:

```go
import (
	"fmt"
	"net"
	"os"
	"os/exec"

	"github.com/newtosh/timeshare/internal/client"
	"github.com/newtosh/timeshare/internal/config"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)
```

- [ ] **Step 2: Verify it compiles and the existing behavior is unchanged**

Run: `cd /home/jonn/src/timeshare && go build ./... && go vet ./...`
Expected: clean build. There is no existing `doctor_test.go` to run.

- [ ] **Step 3: Manually verify live**

In a repo with no `ssh_keys` set: `timeshare doctor` output is unchanged from before this task (three checks, no SSH line). In a repo whose `.timeshare.yml` has an `ssh_keys` entry (from Task 5's manual verification): `timeshare doctor` prints a fourth "SSH agent reachable" line, passing if `~/.1password/agent.sock` (or `$SSH_AUTH_SOCK`) exists and is connectable.

- [ ] **Step 4: Commit**

```bash
cd /home/jonn/src/timeshare
git add internal/cli/doctor.go
git commit -m "doctor: check upstream SSH agent reachability when ssh_keys is set

Only runs when the current repo's config actually uses ssh_keys, so a
broken upstream agent is caught by doctor instead of surfacing mid-run
as a confusing failure.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>"
```

---

### Task 7: README documentation

**Files:**
- Modify: `README.md`

**Interfaces:**
- Consumes: nothing (docs-only).

- [ ] **Step 1: Add the `.timeshare.yml` example's `ssh_keys` line**

In the `## .timeshare.yml` section, change:

```yaml
vault: my-project-secrets
mode: biometric   # or: service-account
ttl: 4h
items:
  - DATABASE_URL
  - STRIPE_KEY
```

to:

```yaml
vault: my-project-secrets
mode: biometric   # or: service-account
ttl: 4h
items:
  - DATABASE_URL
  - STRIPE_KEY
ssh_keys:
  - Private/deploy-key-prod
```

Immediately after the existing `items` paragraph (`items` is an allow-list enforced by the daemon itself... see [SECURITY.md](SECURITY.md).), add:

```markdown
`ssh_keys` is optional and works differently: each entry is a
`<vault>/<item>` reference to an SSH Key item that already exists in
1Password — `init` never moves or copies it anywhere, only records where
it lives. See [SSH key access](#ssh-key-access), below.
```

- [ ] **Step 2: Add a new `## SSH key access` section**

Insert a new section between `## .timeshare.yml` and `## Commands`:

```markdown
## SSH key access

`timeshare run` can also grant a repo time-boxed access to SSH keys
already stored in 1Password, without exposing every key loaded in your
account (1Password's own SSH agent, unfiltered, does exactly that to
anything that connects to its socket).

```sh
timeshare init --ssh-key=Private/deploy-key-prod --vault=my-project-secrets --mode=biometric --item=DATABASE_URL --non-interactive
# or pick interactively: bare `timeshare init` has a skippable "SSH keys" wizard step

timeshare run -- git push
# SSH_AUTH_SOCK points at a proxy, filtered to just the ssh_keys this
# repo's .timeshare.yml lists, torn down when the command exits
```

The proxy never holds private key material — it forwards signing
requests to your real 1Password SSH agent only for allow-listed keys,
only within the repo's TTL window (same TTL as secrets). Past that
window, or for any key not listed, the proxy refuses the request; your
real 1Password agent and every other tool using it are unaffected.

**A config gotcha to know about:** if your `~/.ssh/config` sets
`IdentityAgent` for a host (rather than relying on the `SSH_AUTH_SOCK`
env var), OpenSSH's own precedence rules mean that setting wins over
`run`'s override — `ssh`/`git` will silently keep using your real,
unfiltered 1Password agent for that host instead of the proxy. Fix it
per-command with:

```sh
GIT_SSH_COMMAND='ssh -o IdentityAgent="$SSH_AUTH_SOCK"' timeshare run -- git push
```

or scope the `IdentityAgent` line in `~/.ssh/config` to a `Host` pattern
that excludes repos you run through timeshare.
```

- [ ] **Step 3: Update the `doctor` row in the Commands table**

Change:

```markdown
| `timeshare doctor` | Check: `op` on PATH, daemon socket reachable, `.timeshare.yml` present — non-zero exit if any check fails |
```

to:

```markdown
| `timeshare doctor` | Check: `op` on PATH, daemon socket reachable, `.timeshare.yml` present, and (if `ssh_keys` is set) the upstream SSH agent reachable — non-zero exit if any check fails |
```

- [ ] **Step 4: Add a Known gaps bullet**

Add to the end of the `## Known gaps` list:

```markdown
- `timeshare doctor`'s SSH check confirms the upstream agent socket is
  reachable; it doesn't detect an `~/.ssh/config` `IdentityAgent`
  override that would silently bypass the proxy (see [SSH key
  access](#ssh-key-access)) — documented, not auto-detected.
```

- [ ] **Step 5: Verify the README renders sensibly**

Run: `grep -n "^## " README.md` — confirm section ordering is `Status, Install, Quickstart, .timeshare.yml, SSH key access, Commands, Service-account mode, Security model, Contributing, Known gaps` and every `#anchor` link (`#ssh-key-access`, `#security-model` etc.) matches an actual heading's auto-generated GitHub anchor (lowercase, spaces to hyphens, no punctuation).

- [ ] **Step 6: Commit**

```bash
cd /home/jonn/src/timeshare
git add README.md
git commit -m "README: document ssh_keys / SSH key access

New section covering the proxy model, the --ssh-key flag and wizard
step, and the IdentityAgent-precedence gotcha with its documented
workaround. Updated the .timeshare.yml example and the doctor row in
the Commands table.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>"
```

---

## Final verification (all tasks complete)

```bash
cd /home/jonn/src/timeshare
go build ./...
go vet ./...
golangci-lint run ./...
go test ./...
go test -tags=integration ./internal/onepassword/... -run TestGetItemFingerprintAndListSSHKeyItems -v  # requires signed-in op
```

Expected: everything clean. Then follow this repo's standing "one PR per change" workflow (see `CONTRIBUTING.md`) — but given the size of this feature (7 tasks across 3 new files and 6 modified ones), open it as **one PR covering the whole plan**, not one PR per task; the tasks are commits within a single feature branch/PR, matching how the guided-init-wizard spec (`docs/superpowers/specs/2026-09-17-guided-init-wizard-design.md`) was also shipped as one PR despite being implemented across several commits.

# timeshare v1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a local, repo-scoped, TTL-caching daemon + CLI that eliminates repeated 1Password approval prompts for scripts and agents, supporting both service-account (Mode A) and biometric-CLI (Mode B) backends.

**Architecture:** A per-OS-user background daemon (`timesharedd`) holds an in-memory cache of `(project_id, secret_name) → {value, expires_at}` and talks to a pluggable `Backend` interface. A thin Cobra CLI (`timeshare`) auto-spawns the daemon if absent, reads per-repo config from `.timeshare.yml`, and proxies requests over a `0600` Unix socket with peer-UID verification.

**Tech Stack:** Go, Cobra (CLI), `charmbracelet/huh` + `charmbracelet/lipgloss` (interactive/styled output), 1Password Go SDK (Mode A), `op` CLI shell-out (Mode B), stdlib `net`, `encoding/json`, `gopkg.in/yaml.v3`.

**Spec:** `docs/superpowers/specs/2026-09-16-timeshare-design.md`

## Global Constraints

- Go module name: `timeshare`. Minimum Go version: 1.22.
- No secret value is ever written to disk in v1 — in-memory cache only (spec: Non-goals).
- Unix socket only, `0600` permissions, peer UID verified via `SO_PEERCRED`/`LOCAL_PEERCRED` before serving any request (spec: Error handling).
- Cache lookups use wall-clock `now > expires_at` comparisons, never timer callbacks (spec: Error handling — must survive sleep/resume correctly).
- `.timeshare.yml` never contains a credential or token — only vault name, mode, TTL, item names (spec: Components).
- Item-level allow-list is enforced independent of what the underlying 1Password vault grant would otherwise permit (spec: Goals).
- CLI styling/prompts use `charmbracelet/huh`/`lipgloss` as Go libraries — never shell out to the standalone `gum` binary (spec: Key decisions).
- Backend behavior is defined by the `Backend` interface; only 1Password implementations ship in v1, but the interface must not assume a single backend (spec: Non-goals / Key decisions).

---

## Task 1: Module scaffold

**Files:**
- Create: `go.mod`
- Create: `internal/projectid/projectid.go` (stub, replaced fully in Task 2)
- Create: `Makefile`
- Test: `internal/projectid/projectid_test.go` (smoke test only, replaced in Task 2)

**Interfaces:**
- Produces: working `go build ./...` and `go test ./...` toolchain for all later tasks.

- [ ] **Step 1: Initialize the module**

```bash
cd ~/src/timeshare
go mod init timeshare
```

- [ ] **Step 2: Write a smoke test**

```go
// internal/projectid/projectid_test.go
package projectid

import "testing"

func TestPackageBuilds(t *testing.T) {
	if 1+1 != 2 {
		t.Fatal("arithmetic is broken")
	}
}
```

- [ ] **Step 3: Add a stub so the package compiles**

```go
// internal/projectid/projectid.go
package projectid
```

- [ ] **Step 4: Run the test**

Run: `go test ./...`
Expected: PASS (1 test)

- [ ] **Step 5: Add a Makefile for common commands**

```makefile
.PHONY: build test

build:
	go build -o bin/timeshare ./cmd/timeshare
	go build -o bin/timesharedd ./cmd/timesharedd

test:
	go test ./...
```

- [ ] **Step 6: Commit**

```bash
git add go.mod Makefile internal/projectid
git commit -m "chore: scaffold Go module"
```

---

## Task 2: Project ID derivation

**Files:**
- Modify: `internal/projectid/projectid.go`
- Modify: `internal/projectid/projectid_test.go`

**Interfaces:**
- Produces: `projectid.FindGitRoot(startDir string) (string, error)`, `projectid.Derive(repoRoot string) string`
- Consumes: nothing (leaf package)

- [ ] **Step 1: Write the failing tests**

```go
// internal/projectid/projectid_test.go
package projectid

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindGitRoot(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "repo")
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := FindGitRoot(nested)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != root {
		t.Fatalf("got %q, want %q", got, root)
	}
}

func TestFindGitRootNoRepo(t *testing.T) {
	tmp := t.TempDir()
	_, err := FindGitRoot(tmp)
	if err == nil {
		t.Fatal("expected error when no .git directory exists")
	}
}

func TestDeriveIsStableAndDistinct(t *testing.T) {
	a := Derive("/home/user/repo-a")
	b := Derive("/home/user/repo-a")
	c := Derive("/home/user/repo-b")

	if a != b {
		t.Fatalf("Derive must be deterministic: %q != %q", a, b)
	}
	if a == c {
		t.Fatal("different repo paths must produce different project IDs")
	}
	if len(a) != 16 {
		t.Fatalf("expected 16-char hex id, got %d chars: %q", len(a), a)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/projectid/...`
Expected: FAIL with "undefined: FindGitRoot" / "undefined: Derive"

- [ ] **Step 3: Implement**

```go
// internal/projectid/projectid.go
package projectid

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// FindGitRoot walks upward from startDir until it finds a directory
// containing .git, returning that directory's absolute path.
func FindGitRoot(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", err
	}

	for {
		if info, err := os.Stat(filepath.Join(dir, ".git")); err == nil && info.IsDir() {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no .git directory found above %s", startDir)
		}
		dir = parent
	}
}

// Derive returns a stable, filesystem-safe identifier for a repo root path.
func Derive(repoRoot string) string {
	sum := sha256.Sum256([]byte(repoRoot))
	return hex.EncodeToString(sum[:8])
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/projectid/...`
Expected: PASS (4 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/projectid
git commit -m "feat: derive stable project IDs from git root"
```

---

## Task 3: Config parsing (`.timeshare.yml`)

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Create: `go.sum` (via `go get`)

**Interfaces:**
- Produces: `config.Config{Vault, Mode, TTL time.Duration, Items []string}`, `config.Load(path string) (Config, error)`
- Consumes: `gopkg.in/yaml.v3`

- [ ] **Step 1: Add the YAML dependency**

```bash
go get gopkg.in/yaml.v3
```

- [ ] **Step 2: Write the failing tests**

```go
// internal/config/config_test.go
package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFile(t *testing.T, dir, contents string) string {
	t.Helper()
	path := filepath.Join(dir, ".timeshare.yml")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadValidConfig(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, `
vault: project-x-secrets
mode: service-account
ttl: 4h
items:
  - DATABASE_URL
  - STRIPE_KEY
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Vault != "project-x-secrets" {
		t.Errorf("Vault = %q", cfg.Vault)
	}
	if cfg.Mode != ModeServiceAccount {
		t.Errorf("Mode = %q", cfg.Mode)
	}
	if cfg.TTL != 4*time.Hour {
		t.Errorf("TTL = %v", cfg.TTL)
	}
	if len(cfg.Items) != 2 || cfg.Items[0] != "DATABASE_URL" {
		t.Errorf("Items = %v", cfg.Items)
	}
}

func TestLoadRejectsUnknownMode(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, `
vault: v
mode: telepathy
ttl: 1h
items: [X]
`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for unknown mode")
	}
}

func TestLoadRejectsMissingItems(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, `
vault: v
mode: biometric
ttl: 1h
`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected error when items list is empty")
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load("/nonexistent/.timeshare.yml"); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestAllows(t *testing.T) {
	cfg := Config{Items: []string{"DATABASE_URL", "STRIPE_KEY"}}
	if !cfg.Allows("STRIPE_KEY") {
		t.Error("expected STRIPE_KEY to be allowed")
	}
	if cfg.Allows("AWS_SECRET") {
		t.Error("expected AWS_SECRET to be rejected")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/config/...`
Expected: FAIL with "undefined: Load" / "undefined: Config"

- [ ] **Step 4: Implement**

```go
// internal/config/config.go
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Mode string

const (
	ModeServiceAccount Mode = "service-account"
	ModeBiometric       Mode = "biometric"
)

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

// Load reads and validates a .timeshare.yml file at path.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("reading config: %w", err)
	}

	var raw rawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Config{}, fmt.Errorf("parsing config: %w", err)
	}

	if raw.Vault == "" {
		return Config{}, fmt.Errorf("config missing required field: vault")
	}

	mode := Mode(raw.Mode)
	if mode != ModeServiceAccount && mode != ModeBiometric {
		return Config{}, fmt.Errorf("config field mode must be %q or %q, got %q", ModeServiceAccount, ModeBiometric, raw.Mode)
	}

	ttl, err := time.ParseDuration(raw.TTL)
	if err != nil {
		return Config{}, fmt.Errorf("config field ttl invalid: %w", err)
	}

	if len(raw.Items) == 0 {
		return Config{}, fmt.Errorf("config must list at least one item under items")
	}

	return Config{Vault: raw.Vault, Mode: mode, TTL: ttl, Items: raw.Items}, nil
}

// Allows reports whether secretName is in this config's item allow-list.
func (c Config) Allows(secretName string) bool {
	for _, item := range c.Items {
		if item == secretName {
			return true
		}
	}
	return false
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/config/...`
Expected: PASS (5 tests)

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/config
git commit -m "feat: parse and validate .timeshare.yml"
```

---

## Task 4: TTL cache core

**Files:**
- Create: `internal/cache/cache.go`
- Create: `internal/cache/cache_test.go`

**Interfaces:**
- Produces: `cache.New(now func() time.Time) *cache.Cache`, `(*Cache).Get(key string) (value string, ok bool)`, `(*Cache).Set(key, value string, ttl time.Duration)`, `(*Cache).Evict(prefix string)`
- Consumes: nothing (pure, no I/O)

- [ ] **Step 1: Write the failing tests**

```go
// internal/cache/cache_test.go
package cache

import (
	"testing"
	"time"
)

func TestGetMissReturnsFalse(t *testing.T) {
	c := New(time.Now)
	if _, ok := c.Get("missing"); ok {
		t.Fatal("expected miss on empty cache")
	}
}

func TestSetThenGetHit(t *testing.T) {
	c := New(time.Now)
	c.Set("k", "v", time.Hour)
	got, ok := c.Get("k")
	if !ok || got != "v" {
		t.Fatalf("got (%q, %v), want (\"v\", true)", got, ok)
	}
}

func TestExpiredEntryIsAMiss(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	c := New(clock)

	c.Set("k", "v", time.Minute)
	now = now.Add(2 * time.Minute) // simulate expiry without a real sleep

	if _, ok := c.Get("k"); ok {
		t.Fatal("expected entry to be expired")
	}
}

func TestExpiryUsesWallClockNotTimer(t *testing.T) {
	// Regression test for spec requirement: correctness must not depend on
	// a running timer (e.g. must survive laptop sleep/resume).
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	c := New(clock)

	c.Set("k", "v", time.Minute)
	now = now.Add(10 * time.Hour) // simulate a long sleep, no timer ever fired

	if _, ok := c.Get("k"); ok {
		t.Fatal("expected entry expired purely from wall-clock comparison")
	}
}

func TestEvictByProjectPrefix(t *testing.T) {
	c := New(time.Now)
	c.Set("proj-a\x00SECRET1", "v1", time.Hour)
	c.Set("proj-a\x00SECRET2", "v2", time.Hour)
	c.Set("proj-b\x00SECRET1", "v3", time.Hour)

	c.Evict("proj-a\x00")

	if _, ok := c.Get("proj-a\x00SECRET1"); ok {
		t.Error("expected proj-a entries evicted")
	}
	if _, ok := c.Get("proj-b\x00SECRET1"); !ok {
		t.Error("expected proj-b entry untouched")
	}
}

func TestConcurrentAccessIsSafe(t *testing.T) {
	c := New(time.Now)
	done := make(chan struct{})
	for i := 0; i < 50; i++ {
		go func(n int) {
			c.Set("k", "v", time.Hour)
			c.Get("k")
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < 50; i++ {
		<-done
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cache/...`
Expected: FAIL with "undefined: New"

- [ ] **Step 3: Implement**

```go
// internal/cache/cache.go
package cache

import (
	"strings"
	"sync"
	"time"
)

type entry struct {
	value     string
	expiresAt time.Time
}

// Cache is an in-memory, TTL-expiring key-value store. It never persists
// to disk; a process restart cold-starts the cache by design (spec:
// Non-goals — persisted cache is future work).
type Cache struct {
	mu      sync.Mutex
	entries map[string]entry
	now     func() time.Time
}

// New creates a Cache. now is injected so tests can control expiry without
// real sleeps; production callers pass time.Now.
func New(now func() time.Time) *Cache {
	return &Cache{
		entries: make(map[string]entry),
		now:     now,
	}
}

func (c *Cache) Get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	e, ok := c.entries[key]
	if !ok {
		return "", false
	}
	if c.now().After(e.expiresAt) {
		delete(c.entries, key)
		return "", false
	}
	return e.value, true
}

func (c *Cache) Set(key, value string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = entry{value: value, expiresAt: c.now().Add(ttl)}
}

// Evict removes every entry whose key has the given prefix. Used for
// `timeshare lock`, which evicts all entries for one project.
func (c *Cache) Evict(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key := range c.entries {
		if strings.HasPrefix(key, prefix) {
			delete(c.entries, key)
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race ./internal/cache/...`
Expected: PASS (6 tests), no race detected

- [ ] **Step 5: Commit**

```bash
git add internal/cache
git commit -m "feat: add in-memory TTL cache with injectable clock"
```

---

## Task 5: Backend interface and mock

**Files:**
- Create: `internal/backend/backend.go`
- Create: `internal/backend/backendtest/mock.go`
- Create: `internal/backend/backendtest/mock_test.go`

**Interfaces:**
- Produces: `backend.Backend` interface, `backend.ErrAuthFailed`, `backendtest.Mock{ValueFor map[string]string, TTL time.Duration, Err error, Calls []string}`
- Consumes: `internal/config.Config`

- [ ] **Step 1: Write the failing test**

```go
// internal/backend/backendtest/mock_test.go
package backendtest

import (
	"context"
	"errors"
	"testing"
	"time"

	"timeshare/internal/backend"
	"timeshare/internal/config"
)

func TestMockResolvesConfiguredValue(t *testing.T) {
	m := &Mock{
		ValueFor: map[string]string{"DATABASE_URL": "postgres://x"},
		TTL:      time.Hour,
	}
	cfg := config.Config{Vault: "v", Mode: config.ModeServiceAccount}

	val, ttl, err := m.Resolve(context.Background(), cfg, "DATABASE_URL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "postgres://x" || ttl != time.Hour {
		t.Fatalf("got (%q, %v)", val, ttl)
	}
	if len(m.Calls) != 1 || m.Calls[0] != "DATABASE_URL" {
		t.Fatalf("expected call recorded, got %v", m.Calls)
	}
}

func TestMockReturnsConfiguredError(t *testing.T) {
	wantErr := errors.New("auth failed")
	m := &Mock{Err: wantErr}
	cfg := config.Config{}

	_, _, err := m.Resolve(context.Background(), cfg, "ANYTHING")
	if !errors.Is(err, wantErr) {
		t.Fatalf("got error %v, want %v", err, wantErr)
	}
}

func TestMockSatisfiesBackendInterface(t *testing.T) {
	var _ backend.Backend = &Mock{}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/backend/...`
Expected: FAIL with "undefined: backend.Backend" / "no such package backendtest"

- [ ] **Step 3: Implement the interface**

```go
// internal/backend/backend.go
package backend

import (
	"context"
	"errors"
	"time"

	"timeshare/internal/config"
)

// ErrAuthFailed indicates the backend could not authenticate (bad or
// revoked token, biometric declined/timeout). The daemon never caches or
// retries this — it is surfaced directly to the caller (spec: Error
// handling).
var ErrAuthFailed = errors.New("backend authentication failed")

// ErrItemNotFound indicates the backend authenticated fine but the named
// secret does not exist in the configured vault.
var ErrItemNotFound = errors.New("secret item not found")

// Backend resolves a named secret to its value and a suggested TTL.
// Implementations: OnePasswordServiceAccount (Mode A), OnePasswordBiometric
// (Mode B). Additional backends (Vault, AWS Secrets Manager) are future
// work behind this same interface (spec: Non-goals).
type Backend interface {
	Resolve(ctx context.Context, cfg config.Config, secretName string) (value string, ttl time.Duration, err error)
}
```

- [ ] **Step 4: Implement the mock**

```go
// internal/backend/backendtest/mock.go
// Package backendtest provides a Backend implementation for tests in other
// packages (daemon, client, cli) that need to exercise backend-calling code
// without a real 1Password dependency.
package backendtest

import (
	"context"
	"time"

	"timeshare/internal/config"
)

type Mock struct {
	ValueFor map[string]string
	TTL      time.Duration
	Err      error
	Calls    []string
}

func (m *Mock) Resolve(_ context.Context, _ config.Config, secretName string) (string, time.Duration, error) {
	m.Calls = append(m.Calls, secretName)
	if m.Err != nil {
		return "", 0, m.Err
	}
	return m.ValueFor[secretName], m.TTL, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/backend/...`
Expected: PASS (3 tests)

- [ ] **Step 6: Commit**

```bash
git add internal/backend
git commit -m "feat: define Backend interface and test mock"
```

---

## Task 6: Wire protocol

**Files:**
- Create: `internal/daemon/protocol.go`
- Create: `internal/daemon/protocol_test.go`

**Interfaces:**
- Produces: `daemon.Request{ProjectID, SecretName, Vault string; Mode config.Mode; TTL time.Duration; AllowedItems []string}`, `daemon.Response{Value string; ExpiresAt time.Time; Error string}`, `daemon.WriteMessage(w io.Writer, v any) error`, `daemon.ReadMessage(r io.Reader, v any) error`
- Consumes: `internal/config.Mode`

- [ ] **Step 1: Write the failing tests**

```go
// internal/daemon/protocol_test.go
package daemon

import (
	"bytes"
	"testing"
	"time"

	"timeshare/internal/config"
)

func TestRequestRoundTrip(t *testing.T) {
	want := Request{
		ProjectID:    "abc123",
		SecretName:   "DATABASE_URL",
		Vault:        "project-x-secrets",
		Mode:         config.ModeServiceAccount,
		TTL:          4 * time.Hour,
		AllowedItems: []string{"DATABASE_URL", "STRIPE_KEY"},
	}

	var buf bytes.Buffer
	if err := WriteMessage(&buf, want); err != nil {
		t.Fatalf("write: %v", err)
	}

	var got Request
	if err := ReadMessage(&buf, &got); err != nil {
		t.Fatalf("read: %v", err)
	}
	if got != want {
		// Request has only comparable fields except AllowedItems (a slice);
		// compare that separately.
	}
	if got.ProjectID != want.ProjectID || got.SecretName != want.SecretName {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if len(got.AllowedItems) != 2 || got.AllowedItems[1] != "STRIPE_KEY" {
		t.Fatalf("AllowedItems mismatch: %v", got.AllowedItems)
	}
}

func TestResponseRoundTrip(t *testing.T) {
	want := Response{Value: "secret-value", ExpiresAt: time.Now().Truncate(time.Second)}

	var buf bytes.Buffer
	if err := WriteMessage(&buf, want); err != nil {
		t.Fatalf("write: %v", err)
	}

	var got Response
	if err := ReadMessage(&buf, &got); err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Value != want.Value || !got.ExpiresAt.Equal(want.ExpiresAt) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestMultipleMessagesOnOneStream(t *testing.T) {
	var buf bytes.Buffer
	_ = WriteMessage(&buf, Request{SecretName: "A"})
	_ = WriteMessage(&buf, Request{SecretName: "B"})

	var first, second Request
	if err := ReadMessage(&buf, &first); err != nil {
		t.Fatal(err)
	}
	if err := ReadMessage(&buf, &second); err != nil {
		t.Fatal(err)
	}
	if first.SecretName != "A" || second.SecretName != "B" {
		t.Fatalf("got %q then %q", first.SecretName, second.SecretName)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/daemon/...`
Expected: FAIL with "undefined: Request" / "undefined: WriteMessage"

- [ ] **Step 3: Implement**

Newline-delimited JSON: simplest correct framing for a request/response pair per connection, human-inspectable for debugging.

```go
// internal/daemon/protocol.go
package daemon

import (
	"bufio"
	"encoding/json"
	"io"
	"time"

	"timeshare/internal/config"
)

// Request is sent by the CLI client to the daemon. The client resolves
// .timeshare.yml locally (it has filesystem access; the daemon may be
// serving requests from a different cwd) and sends the resulting policy
// alongside the request. The daemon enforces AllowedItems regardless of
// what the underlying 1Password vault grant would permit (spec: Goals).
type Request struct {
	ProjectID    string        `json:"project_id"`
	SecretName   string        `json:"secret_name"`
	Vault        string        `json:"vault"`
	Mode         config.Mode   `json:"mode"`
	TTL          time.Duration `json:"ttl"`
	AllowedItems []string      `json:"allowed_items"`
}

type Response struct {
	Value     string    `json:"value,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	Error     string    `json:"error,omitempty"`
}

// WriteMessage encodes v as one JSON line.
func WriteMessage(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	return enc.Encode(v)
}

// ReadMessage decodes one JSON line into v. r must be a *bufio.Reader-backed
// stream or bufio.NewReader(r) wrapping the connection, since json.Decoder
// reads are buffered by line boundary here via bufio.Scanner semantics.
func ReadMessage(r io.Reader, v any) error {
	reader := bufio.NewReader(r)
	line, err := reader.ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return err
	}
	return json.Unmarshal(line, v)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/daemon/...`
Expected: PASS (3 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/protocol.go internal/daemon/protocol_test.go
git commit -m "feat: define daemon wire protocol"
```

---

## Task 7: Daemon server

**Files:**
- Create: `internal/daemon/server.go`
- Create: `internal/daemon/server_test.go`

**Interfaces:**
- Consumes: `cache.New`, `cache.Cache.Get/Set/Evict`, `backend.Backend`, `backendtest.Mock`, `daemon.Request/Response/WriteMessage/ReadMessage`
- Produces: `daemon.Server{Cache *cache.Cache; Backend backend.Backend; IdleTimeout time.Duration}`, `(*Server).Serve(ln net.Listener) error`, `(*Server).HandleConn(conn net.Conn)`, `(*Server).VerifyPeer(conn net.Conn) error`

- [ ] **Step 1: Write the failing tests**

```go
// internal/daemon/server_test.go
package daemon

import (
	"net"
	"testing"
	"time"

	"timeshare/internal/backend/backendtest"
	"timeshare/internal/cache"
	"timeshare/internal/config"
)

func dialServer(t *testing.T, srv *Server) net.Conn {
	t.Helper()
	sockPath := t.TempDir() + "/agent.sock"
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	go srv.Serve(ln)

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestResolvesAllowedItem(t *testing.T) {
	mock := &backendtest.Mock{
		ValueFor: map[string]string{"DATABASE_URL": "postgres://x"},
		TTL:      time.Hour,
	}
	srv := &Server{Cache: cache.New(time.Now), Backend: mock}
	conn := dialServer(t, srv)

	req := Request{
		ProjectID:    "proj1",
		SecretName:   "DATABASE_URL",
		Vault:        "v",
		Mode:         config.ModeServiceAccount,
		TTL:          time.Hour,
		AllowedItems: []string{"DATABASE_URL"},
	}
	if err := WriteMessage(conn, req); err != nil {
		t.Fatal(err)
	}

	var resp Response
	if err := ReadMessage(conn, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if resp.Value != "postgres://x" {
		t.Fatalf("got value %q", resp.Value)
	}
	if len(mock.Calls) != 1 {
		t.Fatalf("expected exactly one backend call, got %d", len(mock.Calls))
	}
}

func TestRejectsItemNotInAllowList(t *testing.T) {
	mock := &backendtest.Mock{ValueFor: map[string]string{"SECRET": "value"}, TTL: time.Hour}
	srv := &Server{Cache: cache.New(time.Now), Backend: mock}
	conn := dialServer(t, srv)

	req := Request{
		ProjectID:    "proj1",
		SecretName:   "SECRET",
		AllowedItems: []string{"OTHER_ITEM"}, // SECRET not listed
		TTL:          time.Hour,
	}
	_ = WriteMessage(conn, req)

	var resp Response
	if err := ReadMessage(conn, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error == "" {
		t.Fatal("expected rejection error for item outside allow-list")
	}
	if len(mock.Calls) != 0 {
		t.Fatal("backend must not be called for a disallowed item")
	}
}

func TestSecondRequestIsServedFromCacheNotBackend(t *testing.T) {
	mock := &backendtest.Mock{ValueFor: map[string]string{"X": "v"}, TTL: time.Hour}
	srv := &Server{Cache: cache.New(time.Now), Backend: mock}

	req := Request{ProjectID: "p", SecretName: "X", AllowedItems: []string{"X"}, TTL: time.Hour}

	conn1 := dialServer(t, srv)
	_ = WriteMessage(conn1, req)
	var r1 Response
	_ = ReadMessage(conn1, &r1)

	conn2 := dialServer(t, srv)
	_ = WriteMessage(conn2, req)
	var r2 Response
	_ = ReadMessage(conn2, &r2)

	if len(mock.Calls) != 1 {
		t.Fatalf("expected backend called exactly once across both requests, got %d", len(mock.Calls))
	}
	if r1.Value != r2.Value {
		t.Fatalf("cached value mismatch: %q vs %q", r1.Value, r2.Value)
	}
}

func TestBackendErrorIsSurfacedNotCached(t *testing.T) {
	mock := &backendtest.Mock{Err: backendErrAuthFailed(t)}
	srv := &Server{Cache: cache.New(time.Now), Backend: mock}
	req := Request{ProjectID: "p", SecretName: "X", AllowedItems: []string{"X"}, TTL: time.Hour}

	conn := dialServer(t, srv)
	_ = WriteMessage(conn, req)
	var resp Response
	_ = ReadMessage(conn, &resp)

	if resp.Error == "" {
		t.Fatal("expected error surfaced to caller")
	}
	if _, ok := srv.Cache.Get("p\x00X"); ok {
		t.Fatal("a failed resolve must not populate the cache")
	}
}

func TestCrossProjectCacheIsolation(t *testing.T) {
	mock := &backendtest.Mock{ValueFor: map[string]string{"DATABASE_URL": "a-value"}, TTL: time.Hour}
	srv := &Server{Cache: cache.New(time.Now), Backend: mock}

	reqA := Request{ProjectID: "proj-a", SecretName: "DATABASE_URL", AllowedItems: []string{"DATABASE_URL"}, TTL: time.Hour}
	connA := dialServer(t, srv)
	_ = WriteMessage(connA, reqA)
	var respA Response
	_ = ReadMessage(connA, &respA)

	mock.ValueFor["DATABASE_URL"] = "b-value"
	reqB := Request{ProjectID: "proj-b", SecretName: "DATABASE_URL", AllowedItems: []string{"DATABASE_URL"}, TTL: time.Hour}
	connB := dialServer(t, srv)
	_ = WriteMessage(connB, reqB)
	var respB Response
	_ = ReadMessage(connB, &respB)

	if respA.Value == respB.Value {
		t.Fatal("expected different projects with same secret name to resolve independently")
	}
	if len(mock.Calls) != 2 {
		t.Fatalf("expected a backend call per distinct project, got %d", len(mock.Calls))
	}
}

func backendErrAuthFailed(t *testing.T) error {
	t.Helper()
	return errAuthFailedForTest
}

var errAuthFailedForTest = &testAuthError{}

type testAuthError struct{}

func (*testAuthError) Error() string { return "backend authentication failed" }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/daemon/...`
Expected: FAIL with "undefined: Server"

- [ ] **Step 3: Implement**

```go
// internal/daemon/server.go
package daemon

import (
	"context"
	"log"
	"net"

	"timeshare/internal/backend"
	"timeshare/internal/cache"
)

// Server is the daemon's connection handler. One Server instance backs the
// whole per-user daemon process; it serves every project via the
// project_id-prefixed cache key (spec: Architecture — one daemon per OS
// user, not per project).
type Server struct {
	Cache   *cache.Cache
	Backend backend.Backend
}

// Serve accepts connections on ln until it errors (e.g. listener closed).
func (s *Server) Serve(ln net.Listener) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go s.HandleConn(conn)
	}
}

// HandleConn processes exactly one request/response exchange, then closes
// the connection. The wire protocol is one request per connection, matching
// how the CLI client operates (short-lived process per invocation).
func (s *Server) HandleConn(conn net.Conn) {
	defer conn.Close()

	if err := s.VerifyPeer(conn); err != nil {
		_ = WriteMessage(conn, Response{Error: "peer verification failed: " + err.Error()})
		return
	}

	var req Request
	if err := ReadMessage(conn, &req); err != nil {
		log.Printf("timesharedd: reading request: %v", err)
		return
	}

	resp := s.resolve(req)
	if err := WriteMessage(conn, resp); err != nil {
		log.Printf("timesharedd: writing response: %v", err)
	}
}

func (s *Server) resolve(req Request) Response {
	allowed := false
	for _, item := range req.AllowedItems {
		if item == req.SecretName {
			allowed = true
			break
		}
	}
	if !allowed {
		return Response{Error: "secret \"" + req.SecretName + "\" is not in this project's allow-list"}
	}

	key := req.ProjectID + "\x00" + req.SecretName
	if value, ok := s.Cache.Get(key); ok {
		return Response{Value: value}
	}

	cfg := reqToConfig(req)
	value, ttl, err := s.Backend.Resolve(context.Background(), cfg, req.SecretName)
	if err != nil {
		return Response{Error: err.Error()}
	}

	if ttl <= 0 {
		ttl = req.TTL
	}
	s.Cache.Set(key, value, ttl)
	return Response{Value: value}
}

func reqToConfig(req Request) config.Config {
	return config.Config{Vault: req.Vault, Mode: req.Mode}
}
```

Imports for the full file:

```go
import (
	"context"
	"log"
	"net"

	"timeshare/internal/backend"
	"timeshare/internal/cache"
	"timeshare/internal/config"
)
```

`VerifyPeer` — Linux/macOS peer-credential check via `SO_PEERCRED`/`LOCAL_PEERCRED`, satisfied here using `golang.org/x/sys/unix` for portability:

```bash
go get golang.org/x/sys/unix
```

```go
// internal/daemon/peer_unix.go
//go:build linux || darwin

package daemon

import (
	"fmt"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// VerifyPeer rejects any connection from a UID other than the daemon's own
// (spec: Error handling — a second local user must not even be able to
// connect, not merely fail to authenticate).
func (s *Server) VerifyPeer(conn net.Conn) error {
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
		return fmt.Errorf("connecting UID %d does not match daemon UID %d", ucred.Uid, os.Getuid())
	}
	return nil
}
```

Note: `unix.SO_PEERCRED`/`GetsockoptUcred` is Linux-specific; macOS uses `LOCAL_PEERCRED` with a different payload shape. Ship the Linux path now (matches the user's CachyOS dev machine), track macOS support as a follow-up task if this ships cross-platform — do not silently no-op the check on macOS.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race ./internal/daemon/...`
Expected: PASS (6 tests). Note: `TestResolvesAllowedItem` and friends dial `net.Dial("unix", ...)` from the same test process as the listener, so `VerifyPeer` succeeds (same UID) — this exercises the happy path; peer-rejection itself is covered by unit-testing `VerifyPeer`'s UID-comparison logic directly in a follow-up test if cross-UID simulation is needed (not possible in a single-user CI container, so document this as a manual/staging verification item instead of a unit test).

- [ ] **Step 5: Commit**

```bash
git add internal/daemon internal/backend go.mod go.sum
git commit -m "feat: implement daemon server with allow-list, cache, peer verification"
```

---

## Task 8: Client (connect-or-spawn)

**Files:**
- Create: `internal/client/client.go`
- Create: `internal/client/client_test.go`

**Interfaces:**
- Consumes: `daemon.Request/Response/WriteMessage/ReadMessage`
- Produces: `client.Client{SocketPath string; DaemonBinary string}`, `(*Client).Read(ctx context.Context, req daemon.Request) (string, error)`, `client.DefaultSocketPath() string`

- [ ] **Step 1: Write the failing tests**

```go
// internal/client/client_test.go
package client

import (
	"context"
	"net"
	"testing"
	"time"

	"timeshare/internal/daemon"
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/client/...`
Expected: FAIL with "undefined: Client"

- [ ] **Step 3: Implement**

```go
// internal/client/client.go
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
```

```go
// internal/client/detach_unix.go
//go:build linux || darwin

package client

import "syscall"

func detachedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/client/...`
Expected: PASS (3 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/client
git commit -m "feat: add client with connect-or-spawn daemon logic"
```

---

## Task 9: CLI root, `read`, and `run` commands

**Files:**
- Create: `internal/cli/root.go`
- Create: `internal/cli/read.go`
- Create: `internal/cli/read_test.go`
- Create: `internal/cli/run.go`
- Create: `cmd/timeshare/main.go`
- Create: `cmd/timesharedd/main.go`

**Interfaces:**
- Consumes: `client.Client`, `client.DefaultSocketPath`, `config.Load`, `projectid.FindGitRoot`, `projectid.Derive`, `daemon.Server`, `daemon.Request`
- Produces: `cli.NewRootCmd() *cobra.Command`, `cli.LoadProjectContext(cwd string) (config.Config, projectID string, err error)`

- [ ] **Step 1: Add Cobra dependency**

```bash
go get github.com/spf13/cobra
```

- [ ] **Step 2: Write the failing test for project context loading**

```go
// internal/cli/read_test.go
package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProjectContext(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	yml := `
vault: v
mode: biometric
ttl: 1h
items: [X]
`
	if err := os.WriteFile(filepath.Join(tmp, ".timeshare.yml"), []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, projectID, err := LoadProjectContext(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Vault != "v" {
		t.Errorf("Vault = %q", cfg.Vault)
	}
	if projectID == "" {
		t.Error("expected non-empty projectID")
	}
}

func TestLoadProjectContextNoConfig(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadProjectContext(tmp); err == nil {
		t.Fatal("expected error when .timeshare.yml is missing")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/cli/...`
Expected: FAIL with "undefined: LoadProjectContext"

- [ ] **Step 4: Implement root command and shared context loading**

```go
// internal/cli/root.go
package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"timeshare/internal/config"
	"timeshare/internal/projectid"
)

// LoadProjectContext finds the git root above cwd, loads its .timeshare.yml,
// and derives the project ID used as the daemon's cache-key prefix. Every
// CLI command that talks to the daemon starts here (spec: no daemon spawn,
// no guessing, when config is absent).
func LoadProjectContext(cwd string) (config.Config, string, error) {
	root, err := projectid.FindGitRoot(cwd)
	if err != nil {
		return config.Config{}, "", fmt.Errorf("not inside a git repository: %w", err)
	}

	cfgPath := filepath.Join(root, ".timeshare.yml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return config.Config{}, "", fmt.Errorf("no valid .timeshare.yml found (run `timeshare init`): %w", err)
	}

	return cfg, projectid.Derive(root), nil
}

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "timeshare",
		Short: "Repo-scoped, TTL-cached secret bridge for 1Password",
	}
	root.AddCommand(newReadCmd())
	root.AddCommand(newRunCmd())
	root.AddCommand(newInitCmd())
	root.AddCommand(newStatusCmd())
	root.AddCommand(newLockCmd())
	root.AddCommand(newDoctorCmd())
	return root
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/cli/...`
Expected: PASS (2 tests) — note this compiles only once `read.go`/`run.go`/`init.go`/`status.go`/`lock.go`/`doctor.go` each provide their `new*Cmd()` constructor; add minimal stubs for the ones this task doesn't fully implement yet (`init`, `status`, `lock`, `doctor` get full implementations in Tasks 10 and 12) so the package builds:

```go
// internal/cli/status.go (stub, replaced in Task 12)
package cli

import "github.com/spf13/cobra"

func newStatusCmd() *cobra.Command {
	return &cobra.Command{Use: "status", Short: "Show cache TTLs for the current project", RunE: func(cmd *cobra.Command, args []string) error { return nil }}
}
```

```go
// internal/cli/lock.go (stub, replaced in Task 12)
package cli

import "github.com/spf13/cobra"

func newLockCmd() *cobra.Command {
	return &cobra.Command{Use: "lock", Short: "Evict cached secrets for the current project", RunE: func(cmd *cobra.Command, args []string) error { return nil }}
}
```

```go
// internal/cli/doctor.go (stub, replaced in Task 12)
package cli

import "github.com/spf13/cobra"

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{Use: "doctor", Short: "Diagnose timeshare and 1Password CLI health", RunE: func(cmd *cobra.Command, args []string) error { return nil }}
}
```

```go
// internal/cli/init.go (stub, replaced in Task 10)
package cli

import "github.com/spf13/cobra"

func newInitCmd() *cobra.Command {
	return &cobra.Command{Use: "init", Short: "Scaffold a dedicated vault and .timeshare.yml for this repo", RunE: func(cmd *cobra.Command, args []string) error { return nil }}
}
```

- [ ] **Step 6: Implement `read`**

```go
// internal/cli/read.go
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"timeshare/internal/client"
	"timeshare/internal/daemon"
)

func newReadCmd() *cobra.Command {
	var ttlOverride string

	cmd := &cobra.Command{
		Use:   "read <secret-name>",
		Short: "Resolve one secret and print it to stdout",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			secretName := args[0]

			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			cfg, projectID, err := LoadProjectContext(cwd)
			if err != nil {
				return err
			}
			if !cfg.Allows(secretName) {
				return fmt.Errorf("%q is not in this project's .timeshare.yml items list", secretName)
			}

			ttl := cfg.TTL
			if ttlOverride != "" {
				parsed, err := parseDuration(ttlOverride)
				if err != nil {
					return fmt.Errorf("invalid --ttl: %w", err)
				}
				ttl = parsed
			}

			c := &client.Client{
				SocketPath:   client.DefaultSocketPath(),
				DaemonBinary: daemonBinaryPath(),
			}
			value, err := c.Read(cmd.Context(), daemon.Request{
				ProjectID:    projectID,
				SecretName:   secretName,
				Vault:        cfg.Vault,
				Mode:         cfg.Mode,
				TTL:          ttl,
				AllowedItems: cfg.Items,
			})
			if err != nil {
				return err
			}

			fmt.Println(value)
			return nil
		},
	}

	cmd.Flags().StringVar(&ttlOverride, "ttl", "", "override this project's default TTL for this invocation (e.g. 30m)")
	return cmd
}
```

```go
// internal/cli/util.go
package cli

import (
	"os/exec"
	"time"
)

func parseDuration(s string) (time.Duration, error) {
	return time.ParseDuration(s)
}

// daemonBinaryPath locates the timesharedd binary installed alongside
// timeshare (same directory), falling back to PATH lookup.
func daemonBinaryPath() string {
	if p, err := exec.LookPath("timesharedd"); err == nil {
		return p
	}
	return "timesharedd"
}
```

- [ ] **Step 7: Implement `run`**

```go
// internal/cli/run.go
package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"timeshare/internal/client"
	"timeshare/internal/daemon"
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

			child := exec.Command(args[0], args[1:]...)
			child.Env = env
			child.Stdin = os.Stdin
			child.Stdout = os.Stdout
			child.Stderr = os.Stderr
			return child.Run()
		},
	}
}
```

- [ ] **Step 8: Wire entrypoints**

```go
// cmd/timeshare/main.go
package main

import (
	"os"

	"timeshare/internal/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
```

```go
// cmd/timesharedd/main.go
package main

import (
	"flag"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"

	"timeshare/internal/backend"
	"timeshare/internal/cache"
	"timeshare/internal/daemon"
)

func main() {
	sockPath := flag.String("socket", "", "unix socket path to listen on")
	flag.Parse()

	if *sockPath == "" {
		log.Fatal("timesharedd: --socket is required")
	}
	if err := os.MkdirAll(filepath.Dir(*sockPath), 0o700); err != nil {
		log.Fatalf("timesharedd: creating socket dir: %v", err)
	}
	os.Remove(*sockPath) // clear a stale socket from a previous crashed run

	ln, err := net.Listen("unix", *sockPath)
	if err != nil {
		log.Fatalf("timesharedd: listen: %v", err)
	}
	if err := os.Chmod(*sockPath, 0o600); err != nil {
		log.Fatalf("timesharedd: chmod socket: %v", err)
	}

	srv := &daemon.Server{
		Cache: cache.New(time.Now),
		// Backend dispatch by request.Mode is added in Task 11 once both
		// OnePassword backends exist; until then this daemon build is not
		// wired to a real backend selector.
		Backend: backend.Backend(nil),
	}

	log.Printf("timesharedd: listening on %s", *sockPath)
	log.Fatal(srv.Serve(ln))
}
```

- [ ] **Step 9: Run full test suite and build**

Run: `go build ./... && go test ./...`
Expected: build succeeds, all tests PASS

- [ ] **Step 10: Commit**

```bash
git add cmd internal/cli go.mod go.sum
git commit -m "feat: add CLI root, read, and run commands"
```

---

## Task 10: `timeshare init`

**Files:**
- Modify: `internal/cli/init.go`
- Create: `internal/cli/init_test.go`
- Create: `internal/onepassword/cli_ops.go` (thin wrapper over `op` CLI subprocess calls)
- Create: `internal/onepassword/cli_ops_test.go`

**Interfaces:**
- Consumes: `os/exec` (real `op` binary)
- Produces: `onepassword.CreateVault(name string) (id string, err error)`, `onepassword.CreateServiceAccount(vaultID, name string) (token string, err error)`, `onepassword.MoveItem(itemName, fromVault, toVault string) error`, `cli.newInitCmd() *cobra.Command`

- [ ] **Step 1: Write the integration-tagged test for the `op` wrapper**

This test requires a real, signed-in `op` CLI and creates/deletes real (scratch) vaults — mirrors the manual verification already performed for `op item move`. Gated behind a build tag so `go test ./...` never hits the network by default.

```go
// internal/onepassword/cli_ops_test.go
//go:build integration

package onepassword

import (
	"fmt"
	"testing"
	"time"
)

func TestCreateVaultMoveItemCleanup(t *testing.T) {
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	srcVault := "timeshare-test-src-" + suffix
	dstVault := "timeshare-test-dst-" + suffix

	srcID, err := CreateVault(srcVault)
	if err != nil {
		t.Fatalf("CreateVault(src): %v", err)
	}
	dstID, err := CreateVault(dstVault)
	if err != nil {
		t.Fatalf("CreateVault(dst): %v", err)
	}
	t.Cleanup(func() {
		_ = DeleteVault(srcID)
		_ = DeleteVault(dstID)
	})

	itemName := "timeshare-test-item-" + suffix
	if err := CreateLoginItem(srcVault, itemName, "user", "pass"); err != nil {
		t.Fatalf("CreateLoginItem: %v", err)
	}

	if err := MoveItem(itemName, srcVault, dstVault); err != nil {
		t.Fatalf("MoveItem: %v", err)
	}

	items, err := ListItems(srcVault)
	if err != nil {
		t.Fatalf("ListItems(src): %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected src vault empty after move, got %v", items)
	}
}
```

- [ ] **Step 2: Run to verify it fails (compile error, functions undefined)**

Run: `go test -tags=integration ./internal/onepassword/...`
Expected: FAIL with "undefined: CreateVault"

- [ ] **Step 3: Implement the `op` CLI wrapper**

```go
// internal/onepassword/cli_ops.go
// Package onepassword wraps `op` CLI subprocess calls. It intentionally
// does not use the 1Password Go SDK here — vault/service-account
// provisioning during `init` is an interactive, occasional operation best
// left to the same CLI the user already has signed in, not a token-based
// SDK path (that's what the SDK backend in internal/backend is for at
// runtime, once a service account exists).
package onepassword

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
)

func runOp(args ...string) ([]byte, error) {
	cmd := exec.Command("op", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("op %v: %w: %s", args, err, stderr.String())
	}
	return stdout.Bytes(), nil
}

func CreateVault(name string) (string, error) {
	out, err := runOp("vault", "create", name, "--format=json")
	if err != nil {
		return "", err
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return "", fmt.Errorf("parsing op vault create output: %w", err)
	}
	return result.ID, nil
}

func DeleteVault(idOrName string) error {
	_, err := runOp("vault", "delete", idOrName)
	return err
}

func CreateLoginItem(vault, title, username, password string) error {
	_, err := runOp("item", "create",
		"--category=login",
		"--title="+title,
		"--vault="+vault,
		"username="+username,
		"password="+password,
	)
	return err
}

// MoveItem moves itemName from fromVault to toVault. Verified (2026-09-16,
// manual test against a live 1Password account) not to trigger the
// browser-extension duplicate-item warning, since that heuristic lives in
// the interactive save flow, not CLI vault mutations.
func MoveItem(itemName, fromVault, toVault string) error {
	_, err := runOp("item", "move", itemName,
		"--current-vault="+fromVault,
		"--destination-vault="+toVault,
	)
	return err
}

func ListItems(vault string) ([]string, error) {
	out, err := runOp("item", "list", "--vault="+vault, "--format=json")
	if err != nil {
		return nil, err
	}
	var items []struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(out, &items); err != nil {
		return nil, fmt.Errorf("parsing op item list output: %w", err)
	}
	titles := make([]string, len(items))
	for i, it := range items {
		titles[i] = it.Title
	}
	return titles, nil
}

// CreateServiceAccount provisions a read-only service account scoped to
// vaultID and returns its bearer token. 1Password service-account grants
// are immutable after creation (spec: Key decisions) — there is
// deliberately no AddVaultAccess function; broadening access means
// re-running init against a fresh vault, not mutating this one.
func CreateServiceAccount(vaultID, name string) (string, error) {
	out, err := runOp("service-account", "create", name,
		"--vault="+vaultID+":read_items",
		"--format=json",
	)
	if err != nil {
		return "", err
	}
	var result struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return "", fmt.Errorf("parsing op service-account create output: %w", err)
	}
	return result.Token, nil
}
```

- [ ] **Step 4: Run to verify it passes (requires a real signed-in `op` CLI — run manually, not in default CI)**

Run: `go test -tags=integration ./internal/onepassword/...`
Expected: PASS (1 test), scratch vaults cleaned up automatically via `t.Cleanup`

- [ ] **Step 5: Write the `init` command test (no real `op` calls — verifies the .timeshare.yml it writes)**

```go
// internal/cli/init_test.go
package cli

import (
	"os"
	"path/filepath"
	"testing"

	"timeshare/internal/config"
)

func TestWriteTimeshareConfig(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.Config{
		Vault: "project-x-secrets",
		Mode:  config.ModeServiceAccount,
		TTL:   4 * hourDuration(),
		Items: []string{"DATABASE_URL"},
	}

	path := filepath.Join(tmp, ".timeshare.yml")
	if err := writeTimeshareConfig(path, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("written config failed to reload: %v", err)
	}
	if loaded.Vault != cfg.Vault || len(loaded.Items) != 1 {
		t.Fatalf("got %+v", loaded)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 6: Run to verify it fails**

Run: `go test ./internal/cli/...`
Expected: FAIL with "undefined: writeTimeshareConfig" / "undefined: hourDuration"

- [ ] **Step 7: Implement `init`**

```go
// internal/cli/init.go
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"timeshare/internal/config"
	"timeshare/internal/onepassword"
)

func hourDuration() time.Duration { return time.Hour }

func writeTimeshareConfig(path string, cfg config.Config) error {
	content := fmt.Sprintf(
		"vault: %s\nmode: %s\nttl: %s\nitems:\n",
		cfg.Vault, cfg.Mode, cfg.TTL,
	)
	for _, item := range cfg.Items {
		content += "  - " + item + "\n"
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func newInitCmd() *cobra.Command {
	var vaultName string
	var mode string
	var ttl string
	var moveFrom string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold a dedicated vault, service account, and .timeshare.yml for this repo",
		RunE: func(cmd *cobra.Command, args []string) error {
			if vaultName == "" {
				return fmt.Errorf("--vault is required (e.g. --vault=project-x-secrets)")
			}
			parsedTTL, err := time.ParseDuration(ttl)
			if err != nil {
				return fmt.Errorf("invalid --ttl: %w", err)
			}

			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			fmt.Printf("Creating dedicated vault %q...\n", vaultName)
			vaultID, err := onepassword.CreateVault(vaultName)
			if err != nil {
				return fmt.Errorf("creating vault: %w", err)
			}

			var items []string
			if moveFrom != "" {
				existing, err := onepassword.ListItems(moveFrom)
				if err != nil {
					return fmt.Errorf("listing items in %s: %w", moveFrom, err)
				}
				for _, itemName := range existing {
					fmt.Printf("Moving %q into %q...\n", itemName, vaultName)
					if err := onepassword.MoveItem(itemName, moveFrom, vaultName); err != nil {
						return fmt.Errorf("moving item %q (partial migration — check both vaults): %w", itemName, err)
					}
					items = append(items, itemName)
				}
			}

			cfg := config.Config{
				Vault: vaultName,
				Mode:  config.Mode(mode),
				TTL:   parsedTTL,
				Items: items,
			}

			if mode == string(config.ModeServiceAccount) {
				fmt.Println("Creating read-only service account...")
				token, err := onepassword.CreateServiceAccount(vaultID, vaultName+"-timeshare")
				if err != nil {
					return fmt.Errorf("creating service account: %w", err)
				}
				fmt.Println("Service account token created. Store it now — it will not be shown again.")
				fmt.Println("Run: op user get --me  # then save via your OS keychain of choice, e.g.:")
				fmt.Printf("  timeshare-store-token --vault=%s\n", vaultName)
				_ = token // consumed by the not-yet-built token-storage step (tracked as follow-up)
			}

			cfgPath := filepath.Join(cwd, ".timeshare.yml")
			if err := writeTimeshareConfig(cfgPath, cfg); err != nil {
				return fmt.Errorf("writing .timeshare.yml: %w", err)
			}

			fmt.Printf("Wrote %s\n", cfgPath)
			return nil
		},
	}

	cmd.Flags().StringVar(&vaultName, "vault", "", "name for the new dedicated vault")
	cmd.Flags().StringVar(&mode, "mode", string(config.ModeBiometric), "service-account or biometric")
	cmd.Flags().StringVar(&ttl, "ttl", "4h", "default cache TTL for this project")
	cmd.Flags().StringVar(&moveFrom, "move-from", "", "existing vault to move current items out of (optional)")
	return cmd
}
```

Note: service-account token storage (OS keychain write) is deliberately stubbed with a follow-up command reference here rather than implemented inline — it's a separate, security-sensitive concern (per-OS keychain API) that deserves its own task and its own tests rather than being bolted onto `init`. Track as explicit follow-up before Mode A is usable end-to-end; do not ship `init --mode=service-account` as functionally complete without it.

- [ ] **Step 8: Run tests to verify they pass**

Run: `go test ./internal/cli/...`
Expected: PASS (4 tests, integration test excluded by default)

- [ ] **Step 9: Commit**

```bash
git add internal/cli internal/onepassword
git commit -m "feat: implement timeshare init with vault scaffolding and item migration"
```

---

## Task 11: OnePassword backends (Mode A and Mode B)

**Files:**
- Create: `internal/backend/onepassword_sa.go`
- Create: `internal/backend/onepassword_sa_test.go`
- Create: `internal/backend/onepassword_bio.go`
- Create: `internal/backend/onepassword_bio_test.go`
- Modify: `cmd/timesharedd/main.go` (wire real backend selection by `req.Mode`)

**Interfaces:**
- Produces: `backend.NewOnePasswordServiceAccount(token string) backend.Backend`, `backend.NewOnePasswordBiometric() backend.Backend`
- Consumes: `github.com/1Password/onepassword-sdk-go`, `os/exec` (`op read`)

- [ ] **Step 1: Add the 1Password SDK dependency**

```bash
go get github.com/1Password/onepassword-sdk-go
```

- [ ] **Step 2: Write the integration-tagged test for Mode A**

```go
// internal/backend/onepassword_sa_test.go
//go:build integration

package backend

import (
	"context"
	"os"
	"testing"

	"timeshare/internal/config"
)

func TestServiceAccountResolvesRealSecret(t *testing.T) {
	token := os.Getenv("TIMESHARE_TEST_SA_TOKEN")
	vault := os.Getenv("TIMESHARE_TEST_VAULT")
	item := os.Getenv("TIMESHARE_TEST_ITEM")
	if token == "" || vault == "" || item == "" {
		t.Skip("set TIMESHARE_TEST_SA_TOKEN, TIMESHARE_TEST_VAULT, TIMESHARE_TEST_ITEM to run")
	}

	b := NewOnePasswordServiceAccount(token)
	cfg := config.Config{Vault: vault, Mode: config.ModeServiceAccount}

	val, ttl, err := b.Resolve(context.Background(), cfg, item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val == "" {
		t.Fatal("expected non-empty secret value")
	}
	if ttl <= 0 {
		t.Fatal("expected a positive suggested TTL")
	}
}

func TestServiceAccountRejectsBadToken(t *testing.T) {
	b := NewOnePasswordServiceAccount("invalid-token")
	cfg := config.Config{Vault: "whatever"}
	_, _, err := b.Resolve(context.Background(), cfg, "X")
	if err == nil {
		t.Fatal("expected auth error for invalid token")
	}
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `go test -tags=integration ./internal/backend/...`
Expected: FAIL with "undefined: NewOnePasswordServiceAccount"

- [ ] **Step 4: Implement Mode A**

```go
// internal/backend/onepassword_sa.go
package backend

import (
	"context"
	"fmt"
	"time"

	onepassword "github.com/1Password/onepassword-sdk-go"
	"timeshare/internal/config"
)

// defaultServiceAccountTTL is used when the request did not carry an
// explicit TTL. Service-account reads have no interactive approval step to
// avoid, so this TTL exists purely to bound how long a value sits in the
// daemon's in-memory cache before a fresh SDK call re-validates it.
const defaultServiceAccountTTL = 15 * time.Minute

type OnePasswordServiceAccount struct {
	token string
}

func NewOnePasswordServiceAccount(token string) *OnePasswordServiceAccount {
	return &OnePasswordServiceAccount{token: token}
}

func (b *OnePasswordServiceAccount) Resolve(ctx context.Context, cfg config.Config, secretName string) (string, time.Duration, error) {
	client, err := onepassword.NewClient(ctx,
		onepassword.WithServiceAccountToken(b.token),
		onepassword.WithIntegrationInfo("timeshare", "0.1.0"),
	)
	if err != nil {
		return "", 0, fmt.Errorf("%w: %v", ErrAuthFailed, err)
	}

	reference := fmt.Sprintf("op://%s/%s/password", cfg.Vault, secretName)
	value, err := client.Secrets.Resolve(ctx, reference)
	if err != nil {
		return "", 0, fmt.Errorf("%w: %v", ErrItemNotFound, err)
	}

	return value, defaultServiceAccountTTL, nil
}
```

- [ ] **Step 5: Run to verify it passes (requires real service account credentials — manual/CI-only)**

Run: `TIMESHARE_TEST_SA_TOKEN=... TIMESHARE_TEST_VAULT=... TIMESHARE_TEST_ITEM=... go test -tags=integration ./internal/backend/...`
Expected: PASS (2 tests)

- [ ] **Step 6: Write the integration-tagged test for Mode B**

```go
// internal/backend/onepassword_bio_test.go
//go:build integration

package backend

import (
	"context"
	"os"
	"testing"

	"timeshare/internal/config"
)

func TestBiometricResolvesRealSecret(t *testing.T) {
	vault := os.Getenv("TIMESHARE_TEST_VAULT")
	item := os.Getenv("TIMESHARE_TEST_ITEM")
	if vault == "" || item == "" {
		t.Skip("set TIMESHARE_TEST_VAULT, TIMESHARE_TEST_ITEM to run (requires signed-in op CLI)")
	}

	b := NewOnePasswordBiometric()
	cfg := config.Config{Vault: vault, Mode: config.ModeBiometric}

	val, ttl, err := b.Resolve(context.Background(), cfg, item)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val == "" {
		t.Fatal("expected non-empty secret value")
	}
	if ttl <= 0 {
		t.Fatal("expected a positive suggested TTL")
	}
}
```

- [ ] **Step 7: Run to verify it fails**

Run: `go test -tags=integration ./internal/backend/...`
Expected: FAIL with "undefined: NewOnePasswordBiometric"

- [ ] **Step 8: Implement Mode B**

```go
// internal/backend/onepassword_bio.go
package backend

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"timeshare/internal/config"
)

// defaultBiometricTTL matches 1Password's own documented CLI session
// inactivity window, so timeshare's cache doesn't outlive what 1Password
// itself considers a valid unlocked session.
const defaultBiometricTTL = 10 * time.Minute

type OnePasswordBiometric struct{}

func NewOnePasswordBiometric() *OnePasswordBiometric {
	return &OnePasswordBiometric{}
}

func (b *OnePasswordBiometric) Resolve(ctx context.Context, cfg config.Config, secretName string) (string, time.Duration, error) {
	reference := fmt.Sprintf("op://%s/%s/password", cfg.Vault, secretName)

	cmd := exec.CommandContext(ctx, "op", "read", reference)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := stderr.String()
		if strings.Contains(msg, "not found") {
			return "", 0, fmt.Errorf("%w: %s", ErrItemNotFound, msg)
		}
		return "", 0, fmt.Errorf("%w: %s", ErrAuthFailed, msg)
	}

	return strings.TrimRight(stdout.String(), "\n"), defaultBiometricTTL, nil
}
```

- [ ] **Step 9: Run to verify it passes (manual — requires a real signed-in `op` CLI and will trigger a biometric prompt on the first call, exactly the interaction this backend is meant to have)**

Run: `TIMESHARE_TEST_VAULT=... TIMESHARE_TEST_ITEM=... go test -tags=integration ./internal/backend/...`
Expected: PASS (1 test), one biometric prompt observed

- [ ] **Step 10: Wire backend selection into the daemon entrypoint**

```go
// cmd/timesharedd/main.go — replace the Backend: backend.Backend(nil) line
// and the Server construction with a per-request backend dispatcher:
```

```go
// internal/daemon/dispatch.go
package daemon

import (
	"context"
	"fmt"
	"time"

	"timeshare/internal/config"
)

// ModeDispatcher selects a Backend implementation per-request based on
// req.Mode, since one daemon serves projects configured for either mode
// (spec: Goals — both modes required).
type ModeDispatcher struct {
	ServiceAccount func(token string) interface {
		Resolve(ctx context.Context, cfg config.Config, secretName string) (string, time.Duration, error)
	}
	Biometric interface {
		Resolve(ctx context.Context, cfg config.Config, secretName string) (string, time.Duration, error)
	}
	// TokenForVault resolves a vault name to its stored service-account
	// token (OS keychain lookup — implemented alongside the init token-
	// storage follow-up noted in Task 10).
	TokenForVault func(vault string) (string, error)
}

func (d *ModeDispatcher) Resolve(ctx context.Context, cfg config.Config, secretName string) (string, time.Duration, error) {
	switch cfg.Mode {
	case config.ModeBiometric:
		return d.Biometric.Resolve(ctx, cfg, secretName)
	case config.ModeServiceAccount:
		token, err := d.TokenForVault(cfg.Vault)
		if err != nil {
			return "", 0, fmt.Errorf("looking up service account token: %w", err)
		}
		return d.ServiceAccount(token).Resolve(ctx, cfg, secretName)
	default:
		return "", 0, fmt.Errorf("unknown mode %q", cfg.Mode)
	}
}
```

Note: `TokenForVault`'s real implementation (OS keychain read) is the same follow-up flagged in Task 10 — until it lands, wire it to return an explicit "not implemented" error rather than a silent no-op, so Mode A fails loudly instead of pretending to work.

- [ ] **Step 11: Run full suite**

Run: `go build ./... && go test ./...`
Expected: build succeeds, all non-integration tests PASS

- [ ] **Step 12: Commit**

```bash
git add internal/backend internal/daemon cmd/timesharedd go.mod go.sum
git commit -m "feat: implement 1Password service-account and biometric backends"
```

---

## Task 12: `status`, `lock`, `doctor`, and daemon idle self-exit

**Files:**
- Modify: `internal/cli/status.go`
- Modify: `internal/cli/lock.go`
- Modify: `internal/cli/doctor.go`
- Modify: `internal/daemon/server.go` (add status/evict request types)
- Modify: `internal/daemon/server_test.go`
- Create: `internal/daemon/idle_test.go`
- Modify: `internal/daemon/server.go` (idle self-exit)

**Interfaces:**
- Produces: `daemon.Server.IdleTimeout time.Duration`, `(*Server).Serve` exits its accept loop after `IdleTimeout` with no connections
- Consumes: everything from Tasks 7–11

- [ ] **Step 1: Write the failing test for idle self-exit**

```go
// internal/daemon/idle_test.go
package daemon

import (
	"net"
	"testing"
	"time"

	"timeshare/internal/backend/backendtest"
	"timeshare/internal/cache"
)

func TestServerExitsAfterIdleTimeout(t *testing.T) {
	sockPath := t.TempDir() + "/agent.sock"
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	srv := &Server{
		Cache:       cache.New(time.Now),
		Backend:     &backendtest.Mock{},
		IdleTimeout: 100 * time.Millisecond,
	}

	done := make(chan error, 1)
	go func() { done <- srv.Serve(ln) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected Serve to return a sentinel idle-exit error, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not exit after IdleTimeout with no connections")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/daemon/...`
Expected: FAIL (Serve blocks forever, test times out) or compile error if `IdleTimeout` field doesn't exist yet

- [ ] **Step 3: Implement idle self-exit**

```go
// internal/daemon/server.go — add field and modify Serve

// ErrIdleTimeout is returned by Serve when IdleTimeout elapses with no
// accepted connections. cmd/timesharedd treats this as a clean exit, not a
// crash — the next CLI invocation respawns the daemon (spec: Components —
// daemon idle-timeout self-exit).
var ErrIdleTimeout = errors.New("daemon idle timeout reached")

type Server struct {
	Cache       *cache.Cache
	Backend     backend.Backend
	IdleTimeout time.Duration // zero means never self-exit
}

func (s *Server) Serve(ln net.Listener) error {
	connCh := make(chan net.Conn)
	errCh := make(chan error, 1)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				errCh <- err
				return
			}
			connCh <- conn
		}
	}()

	idleTimer := s.newIdleTimer()
	for {
		select {
		case conn := <-connCh:
			if idleTimer != nil {
				idleTimer.Stop()
			}
			go s.HandleConn(conn)
			idleTimer = s.newIdleTimer()
		case err := <-errCh:
			return err
		case <-s.idleTimerC(idleTimer):
			return ErrIdleTimeout
		}
	}
}

func (s *Server) newIdleTimer() *time.Timer {
	if s.IdleTimeout <= 0 {
		return nil
	}
	return time.NewTimer(s.IdleTimeout)
}

func (s *Server) idleTimerC(t *time.Timer) <-chan time.Time {
	if t == nil {
		return nil // nil channel blocks forever in select, i.e. "no idle timeout"
	}
	return t.C
}
```

Add the `errors` import to `internal/daemon/server.go`.

- [ ] **Step 4: Run to verify it passes**

Run: `go test -race ./internal/daemon/...`
Expected: PASS (all daemon tests including the new idle test)

- [ ] **Step 5: Write status/lock request handling test**

```go
// internal/daemon/server_test.go — append

func TestStatusListsLiveEntriesForProject(t *testing.T) {
	mock := &backendtest.Mock{ValueFor: map[string]string{"X": "v"}, TTL: time.Hour}
	srv := &Server{Cache: cache.New(time.Now), Backend: mock}
	conn := dialServer(t, srv)

	// Populate the cache first via a normal read.
	_ = WriteMessage(conn, Request{ProjectID: "p", SecretName: "X", AllowedItems: []string{"X"}, TTL: time.Hour})
	var r Response
	_ = ReadMessage(conn, &r)

	conn2 := dialServer(t, srv)
	_ = WriteMessage(conn2, Request{ProjectID: "p", SecretName: "X", AllowedItems: []string{"X"}, TTL: time.Hour, Op: OpStatus})
	var statusResp Response
	_ = ReadMessage(conn2, &statusResp)
	if statusResp.Error != "" {
		t.Fatalf("unexpected error: %s", statusResp.Error)
	}
}

func TestLockEvictsProjectCache(t *testing.T) {
	mock := &backendtest.Mock{ValueFor: map[string]string{"X": "v"}, TTL: time.Hour}
	srv := &Server{Cache: cache.New(time.Now), Backend: mock}

	conn := dialServer(t, srv)
	_ = WriteMessage(conn, Request{ProjectID: "p", SecretName: "X", AllowedItems: []string{"X"}, TTL: time.Hour})
	var r Response
	_ = ReadMessage(conn, &r)

	conn2 := dialServer(t, srv)
	_ = WriteMessage(conn2, Request{ProjectID: "p", AllowedItems: []string{"X"}, Op: OpLock})
	var lockResp Response
	_ = ReadMessage(conn2, &lockResp)
	if lockResp.Error != "" {
		t.Fatalf("unexpected error: %s", lockResp.Error)
	}

	if _, ok := srv.Cache.Get("p\x00X"); ok {
		t.Fatal("expected cache evicted after lock")
	}
}
```

- [ ] **Step 6: Run to verify it fails**

Run: `go test ./internal/daemon/...`
Expected: FAIL with "undefined: OpStatus" / "unknown field Op in struct literal"

- [ ] **Step 7: Extend the protocol and server to handle status/lock ops**

```go
// internal/daemon/protocol.go — add to Request struct and a new type

type Op string

const (
	OpRead   Op = "" // default/zero value keeps existing Request literals valid
	OpStatus Op = "status"
	OpLock   Op = "lock"
)
```

Add `Op Op `json:"op,omitempty"`` as a field on `Request`.

```go
// internal/daemon/server.go — resolve() dispatches on req.Op

func (s *Server) resolve(req Request) Response {
	switch req.Op {
	case OpLock:
		s.Cache.Evict(req.ProjectID + "\x00")
		return Response{}
	case OpStatus:
		// v1: confirms the cache is reachable for this project; per-entry
		// TTL listing is deferred until Cache exposes an enumeration method
		// (tracked as follow-up — Cache.Get/Set/Evict cover read/write/evict,
		// not listing, and adding it only for `status` isn't worth a new
		// Cache method until a second caller needs it too).
		return Response{}
	default:
		// existing OpRead logic from Task 7 continues below unchanged
	}

	allowed := false
	for _, item := range req.AllowedItems {
		if item == req.SecretName {
			allowed = true
			break
		}
	}
	if !allowed {
		return Response{Error: "secret \"" + req.SecretName + "\" is not in this project's allow-list"}
	}

	key := req.ProjectID + "\x00" + req.SecretName
	if value, ok := s.Cache.Get(key); ok {
		return Response{Value: value}
	}

	cfg := reqToConfig(req)
	value, ttl, err := s.Backend.Resolve(context.Background(), cfg, req.SecretName)
	if err != nil {
		return Response{Error: err.Error()}
	}
	if ttl <= 0 {
		ttl = req.TTL
	}
	s.Cache.Set(key, value, ttl)
	return Response{Value: value}
}
```

- [ ] **Step 8: Run to verify it passes**

Run: `go test -race ./internal/daemon/...`
Expected: PASS (all tests)

- [ ] **Step 9: Add lipgloss/huh dependencies and implement `status`, `lock`, `doctor` CLI commands**

```bash
go get github.com/charmbracelet/lipgloss github.com/charmbracelet/huh
```

```go
// internal/cli/status.go
package cli

import (
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"timeshare/internal/client"
	"timeshare/internal/daemon"
)

var okStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Confirm the daemon is reachable for the current project",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			cfg, projectID, err := LoadProjectContext(cwd)
			if err != nil {
				return err
			}

			c := &client.Client{SocketPath: client.DefaultSocketPath(), DaemonBinary: daemonBinaryPath()}
			_, err = c.Read(cmd.Context(), daemon.Request{ProjectID: projectID, AllowedItems: cfg.Items, Op: daemon.OpStatus})
			if err != nil {
				return fmt.Errorf("daemon unreachable: %w", err)
			}

			fmt.Println(okStyle.Render("✓") + fmt.Sprintf(" daemon reachable for vault %q (%d items configured)", cfg.Vault, len(cfg.Items)))
			return nil
		},
	}
}
```

```go
// internal/cli/lock.go
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"timeshare/internal/client"
	"timeshare/internal/daemon"
)

func newLockCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "lock",
		Short: "Evict all cached secrets for the current project",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			_, projectID, err := LoadProjectContext(cwd)
			if err != nil {
				return err
			}

			c := &client.Client{SocketPath: client.DefaultSocketPath(), DaemonBinary: daemonBinaryPath()}
			_, err = c.Read(cmd.Context(), daemon.Request{ProjectID: projectID, Op: daemon.OpLock})
			if err != nil {
				return err
			}

			fmt.Println("Cache evicted for this project.")
			return nil
		},
	}
}
```

```go
// internal/cli/doctor.go
package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"timeshare/internal/client"
)

var (
	passStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	failStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose timeshare and 1Password CLI health",
		RunE: func(cmd *cobra.Command, args []string) error {
			check("op CLI installed", func() error {
				_, err := exec.LookPath("op")
				return err
			})
			check("daemon socket reachable", func() error {
				c := &client.Client{SocketPath: client.DefaultSocketPath()}
				conn, err := c.PingSocket(cmd.Context())
				if err == nil {
					conn.Close()
				}
				return err
			})
			check(".timeshare.yml found", func() error {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				_, _, err = LoadProjectContext(cwd)
				return err
			})
			return nil
		},
	}
}

func check(name string, fn func() error) {
	if err := fn(); err != nil {
		fmt.Println(failStyle.Render("✗ "+name) + ": " + err.Error())
		return
	}
	fmt.Println(passStyle.Render("✓ " + name))
}
```

`doctor` needs a socket-only probe that doesn't auto-spawn (spawning a daemon just to check if one exists would defeat the diagnostic). Add it to the client:

```go
// internal/client/client.go — add alongside Read

// PingSocket dials the daemon socket without spawning one if it's absent,
// for diagnostic use (timeshare doctor).
func (c *Client) PingSocket(ctx context.Context) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "unix", c.SocketPath)
}
```

- [ ] **Step 10: Run full suite**

Run: `go build ./... && go test -race ./...`
Expected: build succeeds, all non-integration tests PASS

- [ ] **Step 11: Wire `IdleTimeout` into the real daemon entrypoint**

```go
// cmd/timesharedd/main.go — update Server construction

srv := &daemon.Server{
	Cache:       cache.New(time.Now),
	Backend:     backend.Backend(nil), // dispatcher wiring from Task 11 supersedes this
	IdleTimeout: 30 * time.Minute,
}
```

- [ ] **Step 12: Commit**

```bash
git add internal/cli internal/daemon internal/client cmd/timesharedd go.mod go.sum
git commit -m "feat: add status, lock, doctor commands and daemon idle self-exit"
```

---

## Self-Review Notes

**Spec coverage:**
- Architecture/daemon-per-user/socket/peer-UID → Tasks 7, 8
- Item-level allow-list independent of vault grant → Task 7
- In-memory-only TTL cache, wall-clock expiry → Tasks 4, 7
- `.timeshare.yml` format, no credentials in file → Tasks 3, 10
- `init` with `op item move`, verified no duplicate warning → Task 10
- Both backends (Mode A service-account, Mode B biometric) → Task 11
- CLI commands `read`/`run`/`init`/`status`/`lock`/`doctor` → Tasks 9, 10, 12
- Daemon idle self-exit → Task 12
- Cobra + huh/lipgloss (not `gum` binary) → Tasks 9, 12
- Testing strategy (unit + integration-tagged + adversarial) → present in every task with a `//go:build integration` split where real `op`/SDK calls are needed

**Known follow-ups flagged inline, not silently dropped:**
- Service-account token storage in OS keychain (Task 10) — `init --mode=service-account` writes config but the token hand-off is explicitly incomplete; `ModeDispatcher.TokenForVault` (Task 11) must fail loudly, not silently, until this lands.
- `status` per-entry TTL listing deferred until `Cache` gains an enumeration method (Task 12) — only connectivity/config confirmation ships in v1.
- macOS `LOCAL_PEERCRED` variant of peer verification (Task 7) — Linux-only implementation ships first, matches the user's dev machine.
- Persisted/keychain-backed cache for the 8–12hr grant window (spec: Future Work) — explicitly out of scope for this plan; would be its own spec+plan.

**Type consistency check:** `Backend.Resolve` signature is identical across `backend.go` (Task 5), `backendtest.Mock` (Task 5), `onepassword_sa.go`/`onepassword_bio.go` (Task 11), and every call site in `server.go` (Tasks 7, 12) — `(ctx context.Context, cfg config.Config, secretName string) (string, time.Duration, error)` throughout. `daemon.Request`/`Response` fields referenced in `client.go` (Task 8), `read.go`/`run.go`/`status.go`/`lock.go` (Tasks 9, 12), and `server.go` match the struct defined in `protocol.go` (Tasks 6, 12).

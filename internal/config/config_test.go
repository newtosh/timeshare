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
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
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
	if len(cfg.Items) != 2 || cfg.Items[0].Name != "DATABASE_URL" {
		t.Errorf("Items = %v", cfg.Items)
	}
}

func TestLoadItemFieldOverride(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, `
vault: project-x-secrets
mode: biometric
ttl: 4h
items:
  - DATABASE_URL
  - name: sbg-engtools.gen
    field: notesPlain
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Items) != 2 {
		t.Fatalf("Items = %v", cfg.Items)
	}
	if cfg.Items[0].Name != "DATABASE_URL" || cfg.Items[0].Field != "" {
		t.Errorf("bare item = %+v", cfg.Items[0])
	}
	if cfg.Items[1].Name != "sbg-engtools.gen" || cfg.Items[1].Field != "notesPlain" {
		t.Errorf("mapped item = %+v", cfg.Items[1])
	}
	if got := cfg.FieldFor("sbg-engtools.gen"); got != "notesPlain" {
		t.Errorf("FieldFor = %q", got)
	}
	if got := cfg.FieldFor("DATABASE_URL"); got != "" {
		t.Errorf("FieldFor bare = %q, want empty", got)
	}
}

func TestWriteLoadRoundTripItemField(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		Vault: "project-x-secrets",
		Mode:  ModeBiometric,
		TTL:   4 * time.Hour,
		Items: []Item{
			{Name: "DATABASE_URL"},
			{Name: "sbg-engtools.gen", Field: "notesPlain"},
		},
	}
	path := filepath.Join(dir, ".timeshare.yml")
	if err := Write(path, cfg); err != nil {
		t.Fatalf("Write: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Items) != 2 || loaded.Items[1].Field != "notesPlain" {
		t.Fatalf("got %+v", loaded.Items)
	}
}

func TestLoadRejectsItemMappingWithoutName(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, `
vault: v
mode: biometric
ttl: 1h
items:
  - field: notesPlain
`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for items mapping missing name")
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
		t.Fatal("expected error when items and ssh_keys are both empty")
	}
}

func TestLoadAcceptsSSHKeysOnly(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, `
vault: project-x-secrets
mode: biometric
ttl: 4h
ssh_keys:
  - Private/deploy-key-prod
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Items) != 0 {
		t.Errorf("Items = %v, want empty", cfg.Items)
	}
	if len(cfg.SSHKeys) != 1 || cfg.SSHKeys[0] != "Private/deploy-key-prod" {
		t.Errorf("SSHKeys = %v", cfg.SSHKeys)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load("/nonexistent/.timeshare.yml"); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestAllows(t *testing.T) {
	cfg := Config{Items: []Item{{Name: "DATABASE_URL"}, {Name: "STRIPE_KEY"}}}
	if !cfg.Allows("STRIPE_KEY") {
		t.Error("expected STRIPE_KEY to be allowed")
	}
	if cfg.Allows("AWS_SECRET") {
		t.Error("expected AWS_SECRET to be rejected")
	}
}

func TestWriteLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		Vault: "project-x-secrets",
		Mode:  ModeServiceAccount,
		TTL:   4 * time.Hour,
		Items: []Item{{Name: "DATABASE_URL"}},
	}

	path := filepath.Join(dir, ".timeshare.yml")
	if err := Write(path, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	loaded, err := Load(path)
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

// TestWriteLoadRoundTripYAMLSpecialContent proves vault/item names
// containing YAML-significant content (not just plain whitespace, which
// happens to survive a naive writer) round-trip correctly. Write uses the
// real YAML marshaler for exactly this reason.
func TestWriteLoadRoundTripYAMLSpecialContent(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		Vault: "Team: Ops #prod",
		Mode:  ModeBiometric,
		TTL:   4 * time.Hour,
		Items: []Item{{Name: "DATABASE_URL: primary"}},
	}

	path := filepath.Join(dir, ".timeshare.yml")
	if err := Write(path, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("written config failed to reload: %v", err)
	}
	if loaded.Vault != cfg.Vault {
		t.Fatalf("vault round-trip: got %q, want %q", loaded.Vault, cfg.Vault)
	}
	if len(loaded.Items) != 1 || loaded.Items[0] != cfg.Items[0] {
		t.Fatalf("items round-trip: got %v, want %v", loaded.Items, cfg.Items)
	}
}

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
		Items:   []Item{{Name: "DATABASE_URL"}},
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

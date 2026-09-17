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

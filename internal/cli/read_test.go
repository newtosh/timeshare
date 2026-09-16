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

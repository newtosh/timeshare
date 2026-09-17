package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadExistingConfigMissingFile(t *testing.T) {
	tmp := t.TempDir()
	cfg, exists, err := loadExistingConfig(filepath.Join(tmp, ".timeshare.yml"))
	if err != nil {
		t.Fatalf("expected no error for a missing file, got: %v", err)
	}
	if exists {
		t.Fatal("expected exists=false for a missing file")
	}
	if cfg.Vault != "" {
		t.Fatalf("expected zero-value config, got %+v", cfg)
	}
}

func TestLoadExistingConfigRealFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, ".timeshare.yml")
	content := "vault: project-x\nmode: biometric\nttl: 4h\nitems:\n  - DATABASE_URL\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, exists, err := loadExistingConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Fatal("expected exists=true")
	}
	if cfg.Vault != "project-x" || len(cfg.Items) != 1 {
		t.Fatalf("got %+v", cfg)
	}
}

func TestLoadExistingConfigMalformedFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, ".timeshare.yml")
	if err := os.WriteFile(path, []byte("items: []\n"), 0o644); err != nil { // missing vault, empty items
		t.Fatal(err)
	}

	_, exists, err := loadExistingConfig(path)
	if err == nil {
		t.Fatal("expected an error for a malformed existing config")
	}
	if !exists {
		t.Fatal("expected exists=true even on parse failure — the file is there, just broken")
	}
}

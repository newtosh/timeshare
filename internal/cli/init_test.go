package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/newtosh/timeshare/internal/config"
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

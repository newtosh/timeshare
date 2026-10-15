package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProjectContext(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	yml := `
vault: v
mode: biometric
ttl: 1h
items: [X]
`
	if err := os.WriteFile(filepath.Join(tmp, ".timeshare.yml"), []byte(yml), 0o600); err != nil {
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

func TestLoadProjectContextWorktreeSharesMainConfigAndID(t *testing.T) {
	tmp := t.TempDir()
	main := filepath.Join(tmp, "main")
	wtGit := filepath.Join(main, ".git", "worktrees", "feature")
	if err := os.MkdirAll(wtGit, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtGit, "commondir"), []byte("../..\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	yml := "vault: shared\nmode: biometric\nttl: 4h\nssh_keys: [Private/deploy-key]\n"
	if err := os.WriteFile(filepath.Join(main, ".timeshare.yml"), []byte(yml), 0o600); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(tmp, "feature")
	nested := filepath.Join(work, "pkg")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, ".git"), []byte("gitdir: "+wtGit+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	mainCfg, mainID, err := LoadProjectContext(main)
	if err != nil {
		t.Fatal(err)
	}
	wtCfg, wtID, err := LoadProjectContext(nested)
	if err != nil {
		t.Fatal(err)
	}
	if mainID != wtID {
		t.Fatalf("project ids differ: main %s worktree %s", mainID, wtID)
	}
	if wtCfg.Vault != "shared" || mainCfg.Vault != "shared" {
		t.Fatalf("config not shared: %+v %+v", mainCfg, wtCfg)
	}
	if len(wtCfg.SSHKeys) != 1 {
		t.Fatalf("ssh keys = %v", wtCfg.SSHKeys)
	}
}

func TestLoadProjectContextNoConfig(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadProjectContext(tmp); err == nil {
		t.Fatal("expected error when .timeshare.yml is missing")
	}
}

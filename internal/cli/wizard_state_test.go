package cli

import (
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

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

func TestNewWizardStateNoFlagsSet(t *testing.T) {
	cmd := newTestInitFlags()
	if err := cmd.ParseFlags([]string{}); err != nil {
		t.Fatal(err)
	}
	s := newWizardState(cmd, "", "biometric", "4h", "", nil, nil, nil, false, false)
	if len(s.set) != 0 {
		t.Fatalf("expected no flags recorded in set map, got %v", s.set)
	}
}

func TestNewWizardStateSomeFlagsSet(t *testing.T) {
	cmd := newTestInitFlags()
	if err := cmd.ParseFlags([]string{"--vault=project-x"}); err != nil {
		t.Fatal(err)
	}
	s := newWizardState(cmd, "project-x", "biometric", "4h", "", nil, nil, nil, false, false)
	if !s.set["vault"] {
		t.Fatal("expected set[\"vault\"] true")
	}
	if s.set["mode"] {
		t.Fatal("expected set[\"mode\"] false — mode was left at its default, not explicitly passed")
	}
}

func TestNewWizardStateItemFlagsCountAsSet(t *testing.T) {
	cmd := newTestInitFlags()
	if err := cmd.ParseFlags([]string{"--from-item=legacy-vault/DATABASE_URL"}); err != nil {
		t.Fatal(err)
	}
	s := newWizardState(cmd, "", "biometric", "4h", "", nil, []string{"legacy-vault/DATABASE_URL"}, nil, false, false)
	if !s.set["from-item"] {
		t.Fatal("expected set[\"from-item\"] true")
	}
}

func TestNewWizardStateMoveFlagNotCountedAsSet(t *testing.T) {
	cmd := newTestInitFlags()
	if err := cmd.ParseFlags([]string{"--move"}); err != nil {
		t.Fatal(err)
	}
	s := newWizardState(cmd, "", "biometric", "4h", "", nil, nil, nil, false, true)
	if s.set["move"] {
		t.Fatal("expected set[\"move\"] false — --move modifies behavior, not wizard input")
	}
	if !s.Move {
		t.Fatal("expected s.Move true")
	}
}

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

func TestValidateCompleteRejectsMissingVault(t *testing.T) {
	s := &wizardState{TTL: "4h", Items: []string{"X"}}
	if err := s.validateComplete(); err == nil {
		t.Fatal("expected error for missing vault")
	}
}

func TestValidateCompleteRejectsNoItemSource(t *testing.T) {
	s := &wizardState{Vault: "v", TTL: "4h"}
	if err := s.validateComplete(); err == nil {
		t.Fatal("expected error when no --item/--from/--from-item/--ssh-key given")
	}
}

func TestValidateCompleteAcceptsSSHKeysOnly(t *testing.T) {
	s := &wizardState{Vault: "v", TTL: "4h", SSHKeys: []string{"Private/deploy-key"}}
	if err := s.validateComplete(); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidateCompleteRejectsBadTTL(t *testing.T) {
	s := &wizardState{Vault: "v", TTL: "not-a-duration", Items: []string{"X"}}
	if err := s.validateComplete(); err == nil {
		t.Fatal("expected error for invalid TTL")
	}
}

func TestValidateCompleteAcceptsFromVaultOnly(t *testing.T) {
	s := &wizardState{Vault: "v", TTL: "4h", FromVault: "legacy"}
	if err := s.validateComplete(); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestDefaultVaultNameFallsBackToDirName(t *testing.T) {
	// A directory with no .git ancestor: defaultVaultName falls back to the
	// base name of cwd itself rather than erroring.
	tmp := t.TempDir()
	got := defaultVaultName(tmp)
	want := filepath.Base(tmp)
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

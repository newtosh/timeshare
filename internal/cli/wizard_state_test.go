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
	cmd.Flags().String("move-from", "", "")
	cmd.Flags().StringArrayP("item", "i", nil, "")
	cmd.Flags().StringArray("move-item", nil, "")
	cmd.Flags().BoolP("force", "f", false, "")
	cmd.Flags().BoolP("non-interactive", "n", false, "")
	return cmd
}

func TestNewWizardStateNoFlagsSet(t *testing.T) {
	cmd := newTestInitFlags()
	if err := cmd.ParseFlags([]string{}); err != nil {
		t.Fatal(err)
	}
	s := newWizardState(cmd, "", "biometric", "4h", "", nil, nil, false)
	if s.anyFlagsSet() {
		t.Fatal("expected anyFlagsSet false when no flags were passed")
	}
}

func TestNewWizardStateSomeFlagsSet(t *testing.T) {
	cmd := newTestInitFlags()
	if err := cmd.ParseFlags([]string{"--vault=project-x"}); err != nil {
		t.Fatal(err)
	}
	s := newWizardState(cmd, "project-x", "biometric", "4h", "", nil, nil, false)
	if !s.anyFlagsSet() {
		t.Fatal("expected anyFlagsSet true when --vault was passed")
	}
	if !s.set["vault"] {
		t.Fatal("expected set[\"vault\"] true")
	}
	if s.set["mode"] {
		t.Fatal("expected set[\"mode\"] false — mode was left at its default, not explicitly passed")
	}
}

func TestNewWizardStateItemFlagsCountAsSet(t *testing.T) {
	cmd := newTestInitFlags()
	if err := cmd.ParseFlags([]string{"--move-item=legacy-vault/DATABASE_URL"}); err != nil {
		t.Fatal(err)
	}
	s := newWizardState(cmd, "", "biometric", "4h", "", nil, []string{"legacy-vault/DATABASE_URL"}, false)
	if !s.anyFlagsSet() {
		t.Fatal("expected anyFlagsSet true when --move-item was passed")
	}
	if !s.set["move-item"] {
		t.Fatal("expected set[\"move-item\"] true")
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
		t.Fatal("expected error when no --item/--move-from/--move-item given")
	}
}

func TestValidateCompleteRejectsBadTTL(t *testing.T) {
	s := &wizardState{Vault: "v", TTL: "not-a-duration", Items: []string{"X"}}
	if err := s.validateComplete(); err == nil {
		t.Fatal("expected error for invalid TTL")
	}
}

func TestValidateCompleteAcceptsMoveFromOnly(t *testing.T) {
	s := &wizardState{Vault: "v", TTL: "4h", MoveFrom: "legacy"}
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

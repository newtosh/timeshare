package cli

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/newtosh/timeshare/internal/projectid"

	"github.com/spf13/cobra"
)

// wizardState captures timeshare init's inputs and which of them came from
// explicitly-passed flags (via Cobra's Changed() introspection, since
// several flags — mode, ttl — have non-empty defaults that a plain
// zero-value check can't distinguish from an explicit pass).
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

// validateComplete checks the same completeness rules the original
// flag-only init enforced: required for the --non-interactive path.
func (s *wizardState) validateComplete() error {
	if s.Vault == "" {
		return fmt.Errorf("--vault is required (e.g. --vault=project-x-secrets)")
	}
	if s.FromVault == "" && len(s.Items) == 0 && len(s.FromItems) == 0 && len(s.SSHKeys) == 0 {
		return fmt.Errorf("at least one of --from, --item, --from-item, or --ssh-key is required")
	}
	if _, err := time.ParseDuration(s.TTL); err != nil {
		return fmt.Errorf("invalid --ttl: %w", err)
	}
	return nil
}

// defaultVaultName suggests a vault name for the blank wizard's vault-name
// step: the repo's directory name if cwd is inside a git repo, otherwise
// cwd's own base name. Never errors — worst case it suggests something the
// user is free to overwrite.
func defaultVaultName(cwd string) string {
	root, err := projectid.FindGitRoot(cwd)
	if err != nil {
		root = cwd
	}
	return filepath.Base(root)
}

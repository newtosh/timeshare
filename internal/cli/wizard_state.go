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
	MoveFrom  string
	Items     []string
	MoveItems []string
	Force     bool

	set map[string]bool
}

// wizardFlagNames are the flags that count toward "the user passed
// something" for entry-mode dispatch. --force and --non-interactive are
// deliberately excluded — they modify behavior, not wizard input.
var wizardFlagNames = []string{"vault", "mode", "ttl", "item", "move-from", "move-item"}

func newWizardState(cmd *cobra.Command, vault, mode, ttl, moveFrom string, items, moveItems []string, force bool) *wizardState {
	s := &wizardState{
		Vault:     vault,
		Mode:      mode,
		TTL:       ttl,
		MoveFrom:  moveFrom,
		Items:     items,
		MoveItems: moveItems,
		Force:     force,
		set:       make(map[string]bool),
	}
	for _, name := range wizardFlagNames {
		if cmd.Flags().Changed(name) {
			s.set[name] = true
		}
	}
	return s
}

// anyFlagsSet reports whether any wizard-input flag was explicitly passed.
func (s *wizardState) anyFlagsSet() bool {
	for _, name := range wizardFlagNames {
		if s.set[name] {
			return true
		}
	}
	return false
}

// validateComplete checks the same completeness rules the original
// flag-only init enforced: required for the --non-interactive path.
func (s *wizardState) validateComplete() error {
	if s.Vault == "" {
		return fmt.Errorf("--vault is required (e.g. --vault=project-x-secrets)")
	}
	if s.MoveFrom == "" && len(s.Items) == 0 && len(s.MoveItems) == 0 {
		return fmt.Errorf("at least one of --move-from, --item, or --move-item is required (a config with an empty items list will never load)")
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

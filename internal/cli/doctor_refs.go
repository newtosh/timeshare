package cli

import (
	"fmt"
	"strings"

	"github.com/newtosh/timeshare/internal/config"
	"github.com/newtosh/timeshare/internal/onepassword"
)

// Seams so doctor existence checks are unit-testable without shelling to op.
var doctorGetItem = onepassword.GetItem
var doctorGetSSHFingerprint = onepassword.GetItemFingerprint

// verifyConfiguredItems returns one error per missing/unresolvable item
// in cfg.Vault. Metadata only — does not read secret values.
func verifyConfiguredItems(cfg config.Config) []error {
	var errs []error
	for _, item := range cfg.Items {
		if _, err := doctorGetItem(cfg.Vault, item); err != nil {
			errs = append(errs, fmt.Errorf("item %q in vault %q: %w", item, cfg.Vault, err))
		}
	}
	return errs
}

// verifyConfiguredSSHKeys returns one error per bad ssh_keys ref
// (<vault>/<item>). Uses the fingerprint field so non-SSH-Key items fail
// here instead of mid-run.
func verifyConfiguredSSHKeys(refs []string) []error {
	var errs []error
	for _, ref := range refs {
		idx := strings.LastIndex(ref, "/")
		if idx < 0 {
			errs = append(errs, fmt.Errorf("ssh_key %q must be in <vault>/<item> form", ref))
			continue
		}
		vault, item := ref[:idx], ref[idx+1:]
		if _, err := doctorGetSSHFingerprint(vault, item); err != nil {
			errs = append(errs, fmt.Errorf("ssh_key %q: %w", ref, err))
		}
	}
	return errs
}

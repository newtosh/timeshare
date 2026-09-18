package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/newtosh/timeshare/internal/config"
	"github.com/newtosh/timeshare/internal/onepassword"

	"github.com/spf13/cobra"
)

// moveItem and copyItem are seams over onepassword.MoveItem/CopyItem so
// tests can exercise transferInto's partial-failure/return behavior
// without shelling out to `op`.
var moveItem = onepassword.MoveItem
var copyItem = onepassword.CopyItem

// transferInto copies (or, if move is true, moves) each of picked from
// sourceVault into destVault, printing progress as it goes. It returns
// alreadyMoved plus every item transferred successfully in THIS call,
// alongside any error — including on error, so a caller can persist a
// partial result instead of losing track of what really happened in
// 1Password before the failure.
func transferInto(destVault, sourceVault string, picked []onepassword.Item, alreadyMoved []string, move bool) ([]string, error) {
	verb, do := "Copying", copyItem
	if move {
		verb, do = "Moving", moveItem
	}
	items := append([]string{}, alreadyMoved...)
	for _, item := range picked {
		fmt.Printf("%s %q into %q...\n", verb, item.Title, destVault)
		if err := do(item.ID, sourceVault, destVault); err != nil {
			return items, fmt.Errorf("transferring item %q failed (already done: %v): %w", item.Title, items, err)
		}
		items = append(items, item.Title)
	}
	return items, nil
}

// printSuggestions prints up to 3 "did you mean" candidates for ref within
// sourceVault. Item titles aren't guaranteed unique, so suggestions
// include the ID for a stable follow-up reference. Silently does nothing
// if the vault itself can't be listed (the caller already has a real
// error to report).
func printSuggestions(sourceVault, ref string) {
	items, err := onepassword.ListItems(sourceVault)
	if err != nil {
		return
	}
	matches := onepassword.SuggestMatches(ref, items, 3)
	if len(matches) == 0 {
		return
	}
	fmt.Printf("%q not found in %q. Did you mean:\n", ref, sourceVault)
	for _, m := range matches {
		fmt.Printf("  - %s (id: %s)\n", m.Title, m.ID)
	}
}

// runInit is the shared execution core for a fully-specified wizardState:
// create the vault, move/collect items, write .timeshare.yml. Both the
// --non-interactive path and the completed interactive wizard funnel
// through this — it doesn't know or care which one produced s.
func runInit(cwd string, s *wizardState) error {
	if err := s.validateComplete(); err != nil {
		return err
	}
	parsedTTL, err := time.ParseDuration(s.TTL)
	if err != nil {
		return fmt.Errorf("invalid --ttl: %w", err)
	}

	cfgPath := filepath.Join(cwd, ".timeshare.yml")
	if _, statErr := os.Stat(cfgPath); statErr == nil && !s.Force {
		return fmt.Errorf("%s already exists (pass --force to overwrite)", cfgPath)
	}

	fmt.Printf("Creating dedicated vault %q...\n", s.Vault)
	vaultID, err := onepassword.CreateVault(s.Vault)
	if err != nil {
		return fmt.Errorf("creating vault: %w", err)
	}

	items := append([]string{}, s.Items...)
	if s.FromVault != "" {
		existing, err := onepassword.ListItems(s.FromVault)
		if err != nil {
			return fmt.Errorf("listing items in %s: %w", s.FromVault, err)
		}
		items, err = transferInto(s.Vault, s.FromVault, existing, items, s.Move)
		if err != nil {
			return err
		}
	}

	for _, spec := range s.FromItems {
		// Split on the rightmost "/" rather than the first, so a source
		// vault name that itself contains "/" still parses its item ref
		// correctly in the vault/item form. A bare vault name for the
		// interactive picker must not contain "/" at all — see the
		// flag's help text.
		var sourceVault, ref string
		var hasRef bool
		if idx := strings.LastIndex(spec, "/"); idx >= 0 {
			sourceVault, ref, hasRef = spec[:idx], spec[idx+1:], true
		} else {
			sourceVault = spec
		}

		var toMove []onepassword.Item
		if hasRef {
			item, err := onepassword.GetItem(sourceVault, ref)
			if err != nil {
				printSuggestions(sourceVault, ref)
				return fmt.Errorf("resolving %q in vault %q: %w", ref, sourceVault, err)
			}
			toMove = []onepassword.Item{item}
		} else {
			sourceItems, err := onepassword.ListItems(sourceVault)
			if err != nil {
				return fmt.Errorf("listing items in %s: %w", sourceVault, err)
			}
			picked, err := pickItems(sourceVault, sourceItems, false)
			if err != nil {
				return fmt.Errorf("picking items from %s: %w", sourceVault, err)
			}
			toMove = picked
		}

		var transferErr error
		items, transferErr = transferInto(s.Vault, sourceVault, toMove, items, s.Move)
		if transferErr != nil {
			return transferErr
		}
	}

	cfg := config.Config{
		Vault:   s.Vault,
		Mode:    config.Mode(s.Mode),
		TTL:     parsedTTL,
		Items:   items,
		SSHKeys: s.SSHKeys,
	}

	if s.Mode == string(config.ModeServiceAccount) {
		fmt.Println("Create a service account:")
		fmt.Printf("  op service-account create %s --vault=%s:read_items\n", s.Vault+"-timeshare", vaultID)
		fmt.Println("Then store the printed token in your OS keychain:")
		fmt.Printf("  <paste token> | timeshare token store %s\n", s.Vault)
	}

	if err := config.Write(cfgPath, cfg); err != nil {
		return fmt.Errorf("writing .timeshare.yml: %w", err)
	}

	fmt.Printf("Wrote %s\n", cfgPath)
	return nil
}

func newInitCmd() *cobra.Command {
	var vaultName string
	var mode string
	var ttl string
	var fromVault string
	var explicitItems []string
	var fromItems []string
	var sshKeys []string
	var force bool
	var move bool
	var nonInteractive bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold a dedicated vault and .timeshare.yml for this repo — guided wizard if no flags are given",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			s := newWizardState(cmd, vaultName, mode, ttl, fromVault, explicitItems, fromItems, sshKeys, force, move)

			if nonInteractive {
				return runInit(cwd, s)
			}

			cfgPath := filepath.Join(cwd, ".timeshare.yml")
			existingCfg, exists, loadErr := loadExistingConfig(cfgPath)
			if loadErr != nil {
				return fmt.Errorf("reading existing %s: %w", cfgPath, loadErr)
			}
			if exists {
				return runExistingConfigMenu(cwd, cfgPath, existingCfg)
			}

			completed, err := runWizard(cwd, s)
			if err != nil {
				return err
			}
			return runInit(cwd, completed)
		},
	}

	cmd.Flags().StringVarP(&vaultName, "vault", "v", "", "name for the new dedicated vault")
	cmd.Flags().StringVarP(&mode, "mode", "m", string(config.ModeBiometric), "service-account or biometric")
	cmd.Flags().StringVarP(&ttl, "ttl", "t", "4h", "default cache TTL for this project")
	cmd.Flags().StringVar(&fromVault, "from", "", "existing vault to copy current items out of (optional)")
	cmd.Flags().StringArrayVarP(&explicitItems, "item", "i", nil, "item name to include in .timeshare.yml (repeatable); must already exist in --vault")
	cmd.Flags().StringArrayVar(&fromItems, "from-item", nil, "copy one item from an existing vault: <source-vault>/<item-name-or-id>, or just <source-vault> (no slash) for an interactive picker (repeatable)")
	cmd.Flags().StringArrayVar(&sshKeys, "ssh-key", nil, "grant this repo's `run` access to an SSH key already stored in 1Password: <vault>/<item-name-or-id> (repeatable)")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "overwrite an existing .timeshare.yml")
	cmd.Flags().BoolVar(&move, "move", false, "move items out of the source vault instead of copying them (--from/--from-item default to copy, leaving the original in place)")
	cmd.Flags().BoolVarP(&nonInteractive, "non-interactive", "n", false, "never prompt; validate flags and fail fast on anything incomplete (for scripts/CI)")
	return cmd
}

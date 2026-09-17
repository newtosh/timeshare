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

func hourDuration() time.Duration { return time.Hour }

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

func writeTimeshareConfig(path string, cfg config.Config) error {
	content := fmt.Sprintf(
		"vault: %s\nmode: %s\nttl: %s\nitems:\n",
		cfg.Vault, cfg.Mode, cfg.TTL,
	)
	for _, item := range cfg.Items {
		content += "  - " + item + "\n"
	}
	return os.WriteFile(path, []byte(content), 0o644) //nolint:gosec // .timeshare.yml is meant to be committed to git and world-readable; it never contains a credential
}

func newInitCmd() *cobra.Command {
	var vaultName string
	var mode string
	var ttl string
	var moveFrom string
	var explicitItems []string
	var moveItems []string
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold a dedicated vault and .timeshare.yml for this repo",
		RunE: func(cmd *cobra.Command, args []string) error {
			if vaultName == "" {
				return fmt.Errorf("--vault is required (e.g. --vault=project-x-secrets)")
			}
			if moveFrom == "" && len(explicitItems) == 0 && len(moveItems) == 0 {
				return fmt.Errorf("at least one of --move-from, --item, or --move-item is required (a config with an empty items list will never load)")
			}
			parsedTTL, err := time.ParseDuration(ttl)
			if err != nil {
				return fmt.Errorf("invalid --ttl: %w", err)
			}

			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			cfgPath := filepath.Join(cwd, ".timeshare.yml")
			if _, err := os.Stat(cfgPath); err == nil && !force {
				return fmt.Errorf("%s already exists (pass --force to overwrite)", cfgPath)
			}

			fmt.Printf("Creating dedicated vault %q...\n", vaultName)
			vaultID, err := onepassword.CreateVault(vaultName)
			if err != nil {
				return fmt.Errorf("creating vault: %w", err)
			}

			items := append([]string{}, explicitItems...)
			if moveFrom != "" {
				existing, err := onepassword.ListItems(moveFrom)
				if err != nil {
					return fmt.Errorf("listing items in %s: %w", moveFrom, err)
				}
				for _, it := range existing {
					fmt.Printf("Moving %q into %q...\n", it.Title, vaultName)
					if err := onepassword.MoveItem(it.ID, moveFrom, vaultName); err != nil {
						return fmt.Errorf("moving item %q failed (already moved: %v): %w", it.Title, items, err)
					}
					items = append(items, it.Title)
				}
			}

			for _, spec := range moveItems {
				// Split on the rightmost "/" rather than the first, so a
				// source vault name that itself contains "/" still parses
				// its item ref correctly in the vault/item form. A bare
				// vault name for the interactive picker must not contain
				// "/" at all — see the flag's help text.
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
					picked, err := pickItems(sourceVault, sourceItems)
					if err != nil {
						return fmt.Errorf("picking items from %s: %w", sourceVault, err)
					}
					toMove = picked
				}

				for _, item := range toMove {
					fmt.Printf("Moving %q into %q...\n", item.Title, vaultName)
					if err := onepassword.MoveItem(item.ID, sourceVault, vaultName); err != nil {
						return fmt.Errorf("moving item %q failed (already moved: %v): %w", item.Title, items, err)
					}
					items = append(items, item.Title)
				}
			}

			cfg := config.Config{
				Vault: vaultName,
				Mode:  config.Mode(mode),
				TTL:   parsedTTL,
				Items: items,
			}

			if mode == string(config.ModeServiceAccount) {
				fmt.Println("Create a service account:")
				fmt.Printf("  op service-account create %s --vault=%s:read_items\n", vaultName+"-timeshare", vaultID)
				fmt.Println("Then store the printed token in your OS keychain:")
				fmt.Printf("  <paste token> | timeshare token store %s\n", vaultName)
			}

			if err := writeTimeshareConfig(cfgPath, cfg); err != nil {
				return fmt.Errorf("writing .timeshare.yml: %w", err)
			}

			fmt.Printf("Wrote %s\n", cfgPath)
			return nil
		},
	}

	cmd.Flags().StringVar(&vaultName, "vault", "", "name for the new dedicated vault")
	cmd.Flags().StringVar(&mode, "mode", string(config.ModeBiometric), "service-account or biometric")
	cmd.Flags().StringVar(&ttl, "ttl", "4h", "default cache TTL for this project")
	cmd.Flags().StringVar(&moveFrom, "move-from", "", "existing vault to move current items out of (optional)")
	cmd.Flags().StringArrayVar(&explicitItems, "item", nil, "item name to include in .timeshare.yml (repeatable); must already exist in --vault")
	cmd.Flags().StringArrayVar(&moveItems, "move-item", nil, "move one item from an existing vault: <source-vault>/<item-name-or-id>, or just <source-vault> (no slash) for an interactive picker (repeatable)")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing .timeshare.yml")
	return cmd
}

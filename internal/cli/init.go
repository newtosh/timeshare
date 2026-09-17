package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"timeshare/internal/config"
	"timeshare/internal/onepassword"
)

func hourDuration() time.Duration { return time.Hour }

func writeTimeshareConfig(path string, cfg config.Config) error {
	content := fmt.Sprintf(
		"vault: %s\nmode: %s\nttl: %s\nitems:\n",
		cfg.Vault, cfg.Mode, cfg.TTL,
	)
	for _, item := range cfg.Items {
		content += "  - " + item + "\n"
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func newInitCmd() *cobra.Command {
	var vaultName string
	var mode string
	var ttl string
	var moveFrom string
	var explicitItems []string
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold a dedicated vault and .timeshare.yml for this repo",
		RunE: func(cmd *cobra.Command, args []string) error {
			if vaultName == "" {
				return fmt.Errorf("--vault is required (e.g. --vault=project-x-secrets)")
			}
			if moveFrom == "" && len(explicitItems) == 0 {
				return fmt.Errorf("at least one of --move-from or --item is required (a config with an empty items list will never load)")
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
				for _, itemName := range existing {
					fmt.Printf("Moving %q into %q...\n", itemName, vaultName)
					if err := onepassword.MoveItem(itemName, moveFrom, vaultName); err != nil {
						return fmt.Errorf("moving item %q failed (already moved: %v): %w", itemName, items, err)
					}
					items = append(items, itemName)
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
	cmd.Flags().StringArrayVar(&explicitItems, "item", nil, "item name to include in .timeshare.yml (repeatable)")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing .timeshare.yml")
	return cmd
}

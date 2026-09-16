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

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold a dedicated vault, service account, and .timeshare.yml for this repo",
		RunE: func(cmd *cobra.Command, args []string) error {
			if vaultName == "" {
				return fmt.Errorf("--vault is required (e.g. --vault=project-x-secrets)")
			}
			parsedTTL, err := time.ParseDuration(ttl)
			if err != nil {
				return fmt.Errorf("invalid --ttl: %w", err)
			}

			cwd, err := os.Getwd()
			if err != nil {
				return err
			}

			fmt.Printf("Creating dedicated vault %q...\n", vaultName)
			vaultID, err := onepassword.CreateVault(vaultName)
			if err != nil {
				return fmt.Errorf("creating vault: %w", err)
			}

			var items []string
			if moveFrom != "" {
				existing, err := onepassword.ListItems(moveFrom)
				if err != nil {
					return fmt.Errorf("listing items in %s: %w", moveFrom, err)
				}
				for _, itemName := range existing {
					fmt.Printf("Moving %q into %q...\n", itemName, vaultName)
					if err := onepassword.MoveItem(itemName, moveFrom, vaultName); err != nil {
						return fmt.Errorf("moving item %q (partial migration — check both vaults): %w", itemName, err)
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
				fmt.Println("Creating read-only service account...")
				token, err := onepassword.CreateServiceAccount(vaultID, vaultName+"-timeshare")
				if err != nil {
					return fmt.Errorf("creating service account: %w", err)
				}
				fmt.Println("Service account token created. Store it now — it will not be shown again.")
				fmt.Println("Run: op user get --me  # then save via your OS keychain of choice, e.g.:")
				fmt.Printf("  timeshare-store-token --vault=%s\n", vaultName)
				_ = token // consumed by the not-yet-built token-storage step (tracked as follow-up)
			}

			cfgPath := filepath.Join(cwd, ".timeshare.yml")
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
	return cmd
}

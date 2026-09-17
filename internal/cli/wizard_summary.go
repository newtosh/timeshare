package cli

import (
	"fmt"
	"time"

	"github.com/newtosh/timeshare/internal/config"
	"github.com/newtosh/timeshare/internal/onepassword"

	"github.com/charmbracelet/huh"
)

const (
	menuAddItems  = "add-items"
	menuChangeTTL = "change-ttl"
	menuStartOver = "start-over"
)

// runExistingConfigMenu handles `timeshare init` (wizard mode) when
// .timeshare.yml already exists: show a summary, then act on one targeted
// choice. Never mutates the file until a choice is confirmed.
func runExistingConfigMenu(cwd, cfgPath string, cfg config.Config) error {
	fmt.Printf("Existing config at %s:\n", cfgPath)
	fmt.Printf("  vault: %s\n  mode:  %s\n  ttl:   %s\n  items: %d\n\n", cfg.Vault, cfg.Mode, cfg.TTL, len(cfg.Items))

	var choice string
	if err := huh.NewSelect[string]().
		Title("What would you like to do?").
		Options(
			huh.NewOption("Add items", menuAddItems),
			huh.NewOption("Change TTL", menuChangeTTL),
			huh.NewOption("Start over", menuStartOver),
		).
		Value(&choice).
		Run(); err != nil {
		return err
	}

	switch choice {
	case menuAddItems:
		var sourceVault string
		if err := huh.NewInput().Title("Existing vault to pick items from").Value(&sourceVault).Run(); err != nil {
			return err
		}
		sourceItems, err := onepassword.ListItems(sourceVault)
		if err != nil {
			return fmt.Errorf("listing items in %s: %w", sourceVault, err)
		}
		picked, err := pickItems(sourceVault, sourceItems)
		if err != nil {
			return fmt.Errorf("picking items from %s: %w", sourceVault, err)
		}
		items := append([]string{}, cfg.Items...)
		for _, item := range picked {
			fmt.Printf("Moving %q into %q...\n", item.Title, cfg.Vault)
			if err := onepassword.MoveItem(item.ID, sourceVault, cfg.Vault); err != nil {
				return fmt.Errorf("moving item %q failed (already moved: %v): %w", item.Title, items, err)
			}
			items = append(items, item.Title)
		}
		cfg.Items = items
		return writeTimeshareConfig(cfgPath, cfg)

	case menuChangeTTL:
		ttl := cfg.TTL.String()
		if err := huh.NewInput().Title("New default cache TTL").Value(&ttl).Run(); err != nil {
			return err
		}
		parsed, err := time.ParseDuration(ttl)
		if err != nil {
			return fmt.Errorf("invalid TTL: %w", err)
		}
		// Rewriting the file directly, not going through runInit: the
		// vault already exists, and runInit unconditionally calls
		// onepassword.CreateVault, which would be wrong here — this is a
		// pure config edit, no 1Password mutation at all.
		cfg.TTL = parsed
		return writeTimeshareConfig(cfgPath, cfg)

	case menuStartOver:
		seeded := &wizardState{set: make(map[string]bool)}
		completed, err := runWizard(cwd, seeded)
		if err != nil {
			return err
		}
		completed.Force = true
		return runInit(cwd, completed)
	}

	return nil
}

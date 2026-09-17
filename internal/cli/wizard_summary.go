package cli

import (
	"fmt"
	"regexp"
	"time"

	"github.com/newtosh/timeshare/internal/config"
	"github.com/newtosh/timeshare/internal/onepassword"

	"github.com/charmbracelet/huh"
)

var (
	ttlTrimHourZero = regexp.MustCompile(`h0m0s$`)
	ttlTrimMinZero  = regexp.MustCompile(`m0s$`)
)

// formatTTL renders a duration the way a user is likely to have typed it
// (e.g. "4h" rather than Go's default Duration.String() of "4h0m0s"), by
// trimming trailing zero-valued minute/second components.
func formatTTL(d time.Duration) string {
	s := d.String()
	if trimmed := ttlTrimHourZero.ReplaceAllString(s, "h"); trimmed != s {
		return trimmed
	}
	return ttlTrimMinZero.ReplaceAllString(s, "m")
}

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
	fmt.Printf("  vault: %s\n  mode:  %s\n  ttl:   %s\n  items: %d\n\n", cfg.Vault, cfg.Mode, formatTTL(cfg.TTL), len(cfg.Items))

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
		vaults, err := onepassword.ListVaults()
		if err != nil {
			return fmt.Errorf("listing vaults: %w", err)
		}
		sourceVault, err := pickVault(vaults)
		if err != nil {
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
		items, moveErr := moveInto(cfg.Vault, sourceVault, picked, cfg.Items)
		cfg.Items = items
		// Persist even on a partial failure: some items may really have
		// moved in 1Password before the error, and without this write
		// .timeshare.yml would silently drift out of sync with reality.
		if writeErr := config.Write(cfgPath, cfg); writeErr != nil {
			if moveErr != nil {
				return fmt.Errorf("%w (also failed writing partial config: %w)", moveErr, writeErr)
			}
			return writeErr
		}
		return moveErr

	case menuChangeTTL:
		ttl := formatTTL(cfg.TTL)
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
		return config.Write(cfgPath, cfg)

	case menuStartOver:
		var confirmed bool
		if err := huh.NewConfirm().
			Title(fmt.Sprintf("Start over will overwrite %s. Continue?", cfgPath)).
			Value(&confirmed).
			Run(); err != nil {
			return err
		}
		if !confirmed {
			return nil
		}

		// Seed with the CURRENT vault name rather than a blank state, so
		// the vault-name step pre-fills it instead of re-deriving one
		// from the repo dir name — a name collision (op allows duplicate
		// vault names) becomes visible/intentional rather than silent.
		seeded := &wizardState{Vault: cfg.Vault, set: make(map[string]bool)}
		completed, err := runWizard(cwd, seeded)
		if err != nil {
			return err
		}
		completed.Force = true
		return runInit(cwd, completed)
	}

	return nil
}

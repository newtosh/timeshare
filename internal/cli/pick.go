package cli

import (
	"fmt"

	"github.com/newtosh/timeshare/internal/onepassword"

	"github.com/charmbracelet/huh"
)

// pickItems shows an interactive, type-to-filter multi-select over
// vaultItems (from sourceVault, used only for the prompt title) and
// returns the items the user selected. Falls back to huh's own
// accessible/non-TTY mode automatically if stdin/stdout aren't a real
// terminal — nothing extra needed here for that case.
func pickItems(sourceVault string, vaultItems []onepassword.Item) ([]onepassword.Item, error) {
	if len(vaultItems) == 0 {
		return nil, fmt.Errorf("vault %q has no items to pick from", sourceVault)
	}

	byID := make(map[string]onepassword.Item, len(vaultItems))
	options := make([]huh.Option[string], len(vaultItems))
	for i, it := range vaultItems {
		byID[it.ID] = it
		options[i] = huh.NewOption(fmt.Sprintf("%s (%s)", it.Title, it.ID), it.ID)
	}

	var selectedIDs []string
	field := huh.NewMultiSelect[string]().
		Title(fmt.Sprintf("Select items to move from %q", sourceVault)).
		Options(options...).
		Filtering(true).
		Value(&selectedIDs)

	if err := huh.NewForm(huh.NewGroup(field)).Run(); err != nil {
		return nil, fmt.Errorf("item picker: %w", err)
	}

	if len(selectedIDs) == 0 {
		fmt.Printf("No items selected from %q, skipping.\n", sourceVault)
	}

	selected := make([]onepassword.Item, len(selectedIDs))
	for i, id := range selectedIDs {
		selected[i] = byID[id]
	}
	return selected, nil
}

// pickVault shows an interactive, type-to-filter select over vaults and
// returns the chosen vault's ID — stable even if names aren't unique,
// and op accepts an ID anywhere it accepts a name.
func pickVault(vaults []onepassword.Vault) (string, error) {
	if len(vaults) == 0 {
		return "", fmt.Errorf("no vaults found")
	}

	options := make([]huh.Option[string], len(vaults))
	for i, v := range vaults {
		options[i] = huh.NewOption(fmt.Sprintf("%s (%s)", v.Name, v.ID), v.ID)
	}

	var selected string
	field := huh.NewSelect[string]().
		Title("Existing vault to pick items from").
		Options(options...).
		Filtering(true).
		Value(&selected)

	if err := huh.NewForm(huh.NewGroup(field)).Run(); err != nil {
		return "", fmt.Errorf("vault picker: %w", err)
	}
	return selected, nil
}

package cli

import (
	"fmt"

	"github.com/newtosh/timeshare/internal/onepassword"
)

// pickItems shows an fzf-style, always-filtering multi-select over
// vaultItems (from sourceVault, used only for the picker title) and
// returns the items the user selected (space/tab to toggle, enter to
// confirm). An empty selection is a valid return — the caller decides
// whether that's acceptable.
func pickItems(sourceVault string, vaultItems []onepassword.Item) ([]onepassword.Item, error) {
	if len(vaultItems) == 0 {
		return nil, fmt.Errorf("vault %q has no items to pick from", sourceVault)
	}

	byID := make(map[string]onepassword.Item, len(vaultItems))
	items := make([]fzfItem, len(vaultItems))
	for i, it := range vaultItems {
		byID[it.ID] = it
		items[i] = fzfItem{Label: fmt.Sprintf("%s (%s)", it.Title, it.ID), Value: it.ID}
	}

	chosen, err := runFzfList(fmt.Sprintf("Select items to move from %q", sourceVault), items, true)
	if err != nil {
		return nil, fmt.Errorf("item picker: %w", err)
	}

	if len(chosen) == 0 {
		fmt.Printf("No items selected from %q, skipping.\n", sourceVault)
	}

	selected := make([]onepassword.Item, len(chosen))
	for i, it := range chosen {
		selected[i] = byID[it.Value]
	}
	return selected, nil
}

// pickVault shows an fzf-style, always-filtering select over vaults and
// returns the chosen vault's ID — stable even if names aren't unique, and
// op accepts an ID anywhere it accepts a name.
func pickVault(vaults []onepassword.Vault) (string, error) {
	if len(vaults) == 0 {
		return "", fmt.Errorf("no vaults found")
	}

	items := make([]fzfItem, len(vaults))
	for i, v := range vaults {
		items[i] = fzfItem{Label: fmt.Sprintf("%s (%s)", v.Name, v.ID), Value: v.ID}
	}

	chosen, err := runFzfList("Existing vault to pick items from", items, false)
	if err != nil {
		return "", fmt.Errorf("vault picker: %w", err)
	}
	if len(chosen) == 0 {
		return "", fmt.Errorf("no vault selected")
	}
	return chosen[0].Value, nil
}

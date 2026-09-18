package cli

import (
	"fmt"

	"github.com/newtosh/timeshare/internal/onepassword"
)

// pickItems shows an fzf-style, always-filtering multi-select over
// vaultItems (from sourceVault, used only for the picker title) and
// returns the items the user selected (space to toggle, enter to confirm).
// When requireOne is false, an empty selection is a valid return — the
// caller treats that as "picked nothing, skip" and prints its own message;
// when true, the picker itself blocks enter until at least one item is
// checked, so this never returns an empty slice on success.
func pickItems(sourceVault string, vaultItems []onepassword.Item, requireOne bool) ([]onepassword.Item, error) {
	return pickMulti(sourceVault, vaultItems, requireOne,
		fmt.Sprintf("Select items to move from %q", sourceVault),
		"item picker",
		true,  // print "skipping" on empty selection
		false, // empty vaultItems is an error
	)
}

// pickSSHKeys is pickItems for optional SSH Key selections: empty vault
// list and empty selection are both valid (no keys to grant / skip).
func pickSSHKeys(vault string, sshItems []onepassword.Item) ([]onepassword.Item, error) {
	return pickMulti(vault, sshItems, false,
		fmt.Sprintf("Select SSH keys to grant access to from %q", vault),
		"SSH key picker",
		false, // silent empty selection
		true,  // empty list → nil, nil
	)
}

func pickMulti(vault string, vaultItems []onepassword.Item, requireOne bool, title, errLabel string, announceSkip, emptyOK bool) ([]onepassword.Item, error) {
	if len(vaultItems) == 0 {
		if emptyOK {
			return nil, nil
		}
		return nil, fmt.Errorf("vault %q has no items to pick from", vault)
	}

	byID := make(map[string]onepassword.Item, len(vaultItems))
	items := make([]fzfItem, len(vaultItems))
	for i, it := range vaultItems {
		byID[it.ID] = it
		items[i] = fzfItem{Label: fmt.Sprintf("%s (%s)", it.Title, it.ID), Value: it.ID}
	}

	chosen, err := runFzfList(title, items, true, requireOne)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errLabel, err)
	}

	if announceSkip && len(chosen) == 0 {
		fmt.Printf("No items selected from %q, skipping.\n", vault)
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

	chosen, err := runFzfList("Existing vault to pick items from", items, false, false)
	if err != nil {
		return "", fmt.Errorf("vault picker: %w", err)
	}
	if len(chosen) == 0 {
		return "", fmt.Errorf("no vault selected")
	}
	return chosen[0].Value, nil
}

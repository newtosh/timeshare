// Package onepassword wraps `op` CLI subprocess calls. It intentionally
// does not use the 1Password Go SDK here — vault/service-account
// provisioning during `init` is an interactive, occasional operation best
// left to the same CLI the user already has signed in, not a token-based
// SDK path (that's what the SDK backend in internal/backend is for at
// runtime, once a service account exists).
package onepassword

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
)

func runOp(args ...string) ([]byte, error) {
	return runOpStdin(nil, args...)
}

func runOpStdin(stdin []byte, args ...string) ([]byte, error) {
	cmd := exec.Command("op", args...) //nolint:gosec // fixed binary name "op"; args are constructed by this package, not attacker-controlled
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("op %v: %w: %s", args, err, stderr.String())
	}
	return stdout.Bytes(), nil
}

func CreateVault(name string) (string, error) {
	out, err := runOp("vault", "create", name, "--format=json")
	if err != nil {
		return "", err
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return "", fmt.Errorf("parsing op vault create output: %w", err)
	}
	return result.ID, nil
}

func DeleteVault(idOrName string) error {
	_, err := runOp("vault", "delete", idOrName)
	return err
}

// Vault is a 1Password vault's identity: enough to reference it (ID) and
// show it to a user (Name). Names aren't guaranteed unique across an
// account, so callers should reference vaults by ID once one is chosen.
type Vault struct {
	ID   string
	Name string
}

// ListVaults lists every vault visible to the signed-in account.
func ListVaults() ([]Vault, error) {
	out, err := runOp("vault", "list", "--format=json")
	if err != nil {
		return nil, err
	}
	var raw []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parsing op vault list output: %w", err)
	}
	vaults := make([]Vault, len(raw))
	for i, v := range raw {
		vaults[i] = Vault{ID: v.ID, Name: v.Name}
	}
	return vaults, nil
}

func CreateLoginItem(vault, title, username, password string) error {
	_, err := runOp("item", "create",
		"--category=login",
		"--title="+title,
		"--vault="+vault,
		"username="+username,
		"password="+password,
	)
	return err
}

// MoveItem moves itemName from fromVault to toVault. Verified (2026-09-16,
// manual test against a live 1Password account) not to trigger the
// browser-extension duplicate-item warning, since that heuristic lives in
// the interactive save flow, not CLI vault mutations.
func MoveItem(itemName, fromVault, toVault string) error {
	_, err := runOp("item", "move", itemName,
		"--current-vault="+fromVault,
		"--destination-vault="+toVault,
	)
	return err
}

// CopyItem duplicates itemName from fromVault into toVault, leaving the
// original in place. `op` has no dedicated copy subcommand; this pipes
// `op item get --format=json` into `op item create -`, the pattern op's
// own --help documents for duplicating an item across vaults.
func CopyItem(itemName, fromVault, toVault string) error {
	raw, err := runOp("item", "get", itemName, "--vault="+fromVault, "--format=json")
	if err != nil {
		return err
	}
	_, err = runOpStdin(raw, "item", "create", "--vault="+toVault, "-")
	return err
}

// Item is a 1Password item's identity: enough to reference it (ID) and
// show it to a user (Title). Titles aren't guaranteed unique within a
// vault — 1Password's own docs recommend IDs for stable references.
type Item struct {
	ID    string
	Title string
}

func ListItems(vault string) ([]Item, error) {
	return listItems(vault, "")
}

// ListSSHKeyItems lists only SSH Key category items in vault — the
// source list for the init wizard's SSH-key picker.
func ListSSHKeyItems(vault string) ([]Item, error) {
	return listItems(vault, "SSH Key")
}

func listItems(vault, category string) ([]Item, error) {
	args := []string{"item", "list", "--vault=" + vault}
	if category != "" {
		args = append(args, "--categories="+category)
	}
	args = append(args, "--format=json")
	out, err := runOp(args...)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parsing op item list output: %w", err)
	}
	items := make([]Item, len(raw))
	for i, it := range raw {
		items[i] = Item{ID: it.ID, Title: it.Title}
	}
	return items, nil
}

// GetItem resolves ref (a title or an ID — op accepts either
// interchangeably) against vault. Returns an error if ref doesn't match
// exactly one item.
func GetItem(vault, ref string) (Item, error) {
	out, err := runOp("item", "get", ref, "--vault="+vault, "--format=json")
	if err != nil {
		return Item{}, err
	}
	var raw struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return Item{}, fmt.Errorf("parsing op item get output: %w", err)
	}
	return Item{ID: raw.ID, Title: raw.Title}, nil
}

// GetItemFingerprint resolves ref (a title or ID) within vault to that
// item's SSH key fingerprint, e.g. "SHA256:...". Only valid for SSH Key
// category items — any other category has no "fingerprint" field, so op
// returns an error.
func GetItemFingerprint(vault, ref string) (string, error) {
	out, err := runOp("item", "get", ref, "--vault="+vault, "--fields", "label=fingerprint", "--format=json")
	if err != nil {
		return "", err
	}
	var raw struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return "", fmt.Errorf("parsing op item get fingerprint output: %w", err)
	}
	if raw.Value == "" {
		return "", fmt.Errorf("item %q in vault %q has no fingerprint field (not an SSH Key item?)", ref, vault)
	}
	return raw.Value, nil
}

// DefaultSecretField is the Login-item field timeshare reads when an
// items: entry does not set field: (Secure Notes typically need
// notesPlain — see .timeshare.yml per-item field override).
const DefaultSecretField = "password"

// ReadField reads a single field from an item via `op item get` with
// vault/item as separate argv — not an op:// URI — so titles containing
// "/" or spaces resolve correctly (same approach as GetItemFingerprint).
func ReadField(vault, item, field string) (string, error) {
	out, err := runOp("item", "get", item, "--vault="+vault, "--fields", "label="+field, "--reveal", "--format=json")
	if err != nil {
		return "", err
	}
	var raw struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return "", fmt.Errorf("parsing op item get %s output: %w", field, err)
	}
	if raw.Value == "" {
		return "", fmt.Errorf("item %q in vault %q has no %q field", item, vault, field)
	}
	return raw.Value, nil
}

// SecretReference builds an op://vault/item/field URI for the SDK path,
// path-escaping the item segment so titles with "/" or spaces don't
// break the reference grammar.
func SecretReference(vault, item, field string) string {
	return "op://" + vault + "/" + url.PathEscape(item) + "/" + url.PathEscape(field)
}

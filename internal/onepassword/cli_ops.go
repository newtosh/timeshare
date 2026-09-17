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
	"os/exec"
)

func runOp(args ...string) ([]byte, error) {
	cmd := exec.Command("op", args...) //nolint:gosec // fixed binary name "op"; args are constructed by this package, not attacker-controlled
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
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

func ListItems(vault string) ([]string, error) {
	out, err := runOp("item", "list", "--vault="+vault, "--format=json")
	if err != nil {
		return nil, err
	}
	var items []struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(out, &items); err != nil {
		return nil, fmt.Errorf("parsing op item list output: %w", err)
	}
	titles := make([]string, len(items))
	for i, it := range items {
		titles[i] = it.Title
	}
	return titles, nil
}

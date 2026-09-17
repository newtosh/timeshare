package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"

	"github.com/newtosh/timeshare/internal/config"
	"github.com/newtosh/timeshare/internal/onepassword"
)

// Step indices are unused by anything reachable yet: runWizard (below) is
// only wired into `timeshare init` by Task 8, and its items step is filled
// in by Task 6.
//
//nolint:unused // wired in by Task 6/8
const (
	stepVault = iota
	stepMode
	stepItems
	stepTTL
	stepCount
)

//nolint:unused // wired in by Task 6/8
var wizardStepLabels = [stepCount]string{
	stepVault: "Vault name",
	stepMode:  "Auth mode",
	stepItems: "Items",
	stepTTL:   "TTL",
}

//nolint:unused // wired in by Task 6/8
var wizardStepHelp = [stepCount]string{
	stepVault: "The name of a new, dedicated 1Password vault timeshare will create for this project. Pick something specific to this repo — it shouldn't be shared with unrelated projects.",
	stepMode:  "Biometric: shells out to `op read`, same Touch ID/Windows Hello prompt you already get, cached for the TTL. Service account: headless, token-based, no prompts at all, but requires a manual token-store step after init (see the printed instructions).",
	stepItems: "Which 1Password items should this project's allow-list include. You can move items from an existing vault, or pick from a list interactively.",
	stepTTL:   "How long a resolved secret stays cached before the next read re-checks 1Password. Longer means fewer prompts but a longer window before a rotated/revoked secret takes effect.",
}

// isHelpRequest reports whether a raw prompt input was a bare "?" (possibly
// surrounded by whitespace), the wizard's help-toggle signal.
func isHelpRequest(input string) bool {
	return strings.TrimSpace(input) == "?"
}

// promptWithHelp runs prompt in a loop: if the result is a bare "?", it
// prints that step's help text and re-prompts instead of returning.
//
//nolint:unused // wired in by Task 6/8 (called from runWizard)
func promptWithHelp(step int, prompt func() (string, error)) (string, error) {
	for {
		val, err := prompt()
		if err != nil {
			return "", err
		}
		if !isHelpRequest(val) {
			return val, nil
		}
		fmt.Println(wizardStepHelp[step])
	}
}

// runWizard walks the vault/mode/items/ttl steps, skipping any step whose
// value was already seeded from a flag (seeded.set[...] true), and returns
// the completed state. It does not call runInit — the caller (newInitCmd's
// RunE, Task 8) does that once the wizard returns.
//
//nolint:unused // wired in by Task 8
func runWizard(cwd string, seeded *wizardState) (*wizardState, error) {
	s := &wizardState{
		Vault: seeded.Vault, Mode: seeded.Mode, TTL: seeded.TTL,
		MoveFrom: seeded.MoveFrom, Items: seeded.Items, MoveItems: seeded.MoveItems,
		Force: seeded.Force, set: seeded.set,
	}

	steps := func() []wizardStep {
		return []wizardStep{
			{Label: wizardStepLabels[stepVault], Value: s.Vault, Done: s.Vault != ""},
			{Label: wizardStepLabels[stepMode], Value: s.Mode, Done: s.set["mode"] || s.Mode != ""},
			{Label: wizardStepLabels[stepItems], Value: itemsSummary(s), Done: len(s.Items) > 0 || len(s.MoveItems) > 0 || s.MoveFrom != ""},
			{Label: wizardStepLabels[stepTTL], Value: s.TTL, Done: s.set["ttl"]},
		}
	}

	if !s.set["vault"] {
		fmt.Print(renderStepBlock(steps(), stepVault, "Vault name:") + "\n")
		val, err := promptWithHelp(stepVault, func() (string, error) {
			v := s.Vault
			if v == "" {
				v = defaultVaultName(cwd)
			}
			err := huh.NewInput().Title("Vault name").Value(&v).Run()
			return v, err
		})
		if err != nil {
			return nil, err
		}
		s.Vault = val
	}

	if !s.set["mode"] {
		fmt.Print(renderStepBlock(steps(), stepMode, "Auth mode:") + "\n")
		mode := s.Mode
		if mode == "" {
			mode = string(config.ModeBiometric)
		}
		if err := huh.NewSelect[string]().
			Title("Auth mode").
			Options(
				huh.NewOption("Biometric (recommended)", string(config.ModeBiometric)),
				huh.NewOption("Service account", string(config.ModeServiceAccount)),
			).
			Value(&mode).
			Run(); err != nil {
			return nil, err
		}
		s.Mode = mode
	}

	if !s.set["item"] && !s.set["move-from"] && !s.set["move-item"] {
		fmt.Print(renderStepBlock(steps(), stepItems, "Items:\n\nMove items from an existing vault, or add them\nyourself later and edit .timeshare.yml by hand.") + "\n")

		var sourceVault string
		if err := huh.NewInput().
			Title("Existing vault to pick items from (leave blank to skip and add items later)").
			Value(&sourceVault).
			Run(); err != nil {
			return nil, err
		}

		if sourceVault != "" {
			sourceItems, err := onepassword.ListItems(sourceVault)
			if err != nil {
				return nil, fmt.Errorf("listing items in %s: %w", sourceVault, err)
			}
			picked, err := pickItems(sourceVault, sourceItems)
			if err != nil {
				return nil, fmt.Errorf("picking items from %s: %w", sourceVault, err)
			}
			s.MoveItems = make([]string, len(picked))
			for i, item := range picked {
				s.MoveItems[i] = sourceVault + "/" + item.ID
			}
		}
	}

	if !s.set["ttl"] {
		fmt.Print(renderStepBlock(steps(), stepTTL, "Default cache TTL:") + "\n")
		val, err := promptWithHelp(stepTTL, func() (string, error) {
			ttl := s.TTL
			if ttl == "" {
				ttl = "4h"
			}
			err := huh.NewInput().Title("Default cache TTL").Value(&ttl).Run()
			return ttl, err
		})
		if err != nil {
			return nil, err
		}
		s.TTL = val
	}

	return s, nil
}

//nolint:unused // wired in by Task 8 (called from runWizard)
func itemsSummary(s *wizardState) string {
	n := len(s.Items) + len(s.MoveItems)
	if s.MoveFrom != "" {
		return "from " + s.MoveFrom
	}
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("%d selected", n)
}

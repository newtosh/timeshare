package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"

	"github.com/newtosh/timeshare/internal/config"
	"github.com/newtosh/timeshare/internal/onepassword"
)

const (
	stepVault = iota
	stepMode
	stepItems
	stepTTL
	stepCount
)

var wizardStepLabels = [stepCount]string{
	stepVault: "Repo vault name",
	stepMode:  "Auth mode",
	stepItems: "Items",
	stepTTL:   "TTL",
}

var wizardStepHelp = [stepCount]string{
	stepVault: "The name of a new, dedicated 1Password vault timeshare will create for this repo. Pick something specific to this repo — it shouldn't be shared with unrelated projects.",
	stepMode:  "Biometric: shells out to `op read`, same Touch ID/Windows Hello prompt you already get, cached for the TTL. Service account: headless, token-based, no prompts at all, but requires a manual token-store step after init (see the printed instructions).",
	stepItems: "Which 1Password items should this project's allow-list include. You can move items from an existing vault, or pick from a list interactively.",
	stepTTL:   "How long a resolved secret stays cached before the next read re-checks 1Password. Longer means fewer prompts but a longer window before a rotated/revoked secret takes effect.",
}

// stepBlockRenderer redraws the wizard's step block in place: each call
// clears the previous render (cursor up + clear-to-end) before printing the
// next one, instead of letting every step's block scroll past on its own.
type stepBlockRenderer struct{ lines int }

func (r *stepBlockRenderer) render(block string) {
	if r.lines > 0 {
		// Reset SGR (colors) before erasing: \033[J fills the erased region
		// using whatever background color is currently active, so without
		// an explicit reset first, our column backgrounds can leak forward
		// into terminal state that huh's own widgets (filter text, prompts)
		// render on top of next — making that text unreadable against a
		// leftover background instead of the terminal's real default.
		fmt.Printf("\033[0m\033[%dA\033[J", r.lines)
	}
	fmt.Print(block)
	fmt.Print("\033[0m")
	r.lines = strings.Count(block, "\n")
}

// isHelpRequest reports whether a raw prompt input was a bare "?" (possibly
// surrounded by whitespace), the wizard's help-toggle signal.
func isHelpRequest(input string) bool {
	return strings.TrimSpace(input) == "?"
}

// promptWithHelp runs prompt in a loop: if the result is a bare "?", it
// prints that step's help text and re-prompts instead of returning.
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
func runWizard(cwd string, seeded *wizardState) (*wizardState, error) {
	s := &wizardState{
		Vault: seeded.Vault, Mode: seeded.Mode, TTL: seeded.TTL,
		MoveFrom: seeded.MoveFrom, Items: seeded.Items, MoveItems: seeded.MoveItems,
		Force: seeded.Force, set: seeded.set,
	}

	// answered tracks which steps this wizard RUN has actively prompted
	// and gotten a response for, independent of s's field values (which
	// can be non-empty just from cobra flag defaults, e.g. Mode defaults
	// to "biometric" even with zero flags passed). Every step's Done
	// below uses the same rule: seeded-via-flag, or answered just now.
	var answered [stepCount]bool
	render := &stepBlockRenderer{}

	steps := func() []wizardStep {
		return []wizardStep{
			{Label: wizardStepLabels[stepVault], Value: s.Vault, Done: s.set["vault"] || answered[stepVault]},
			{Label: wizardStepLabels[stepMode], Value: s.Mode, Done: s.set["mode"] || answered[stepMode]},
			{Label: wizardStepLabels[stepItems], Value: itemsSummary(s), Done: s.set["item"] || s.set["move-from"] || s.set["move-item"] || answered[stepItems]},
			{Label: wizardStepLabels[stepTTL], Value: s.TTL, Done: s.set["ttl"] || answered[stepTTL]},
		}
	}

	if !s.set["vault"] {
		render.render(renderStepBlock(steps(), stepVault, "Repo vault name:") + "\n")
		val, err := promptWithHelp(stepVault, func() (string, error) {
			v := s.Vault
			if v == "" {
				v = defaultVaultName(cwd)
			}
			err := huh.NewInput().Title("Repo vault name").Value(&v).Run()
			return v, err
		})
		if err != nil {
			return nil, err
		}
		s.Vault = val
		answered[stepVault] = true
	}

	if !s.set["mode"] {
		render.render(renderStepBlock(steps(), stepMode, "Auth mode:") + "\n")
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
		answered[stepMode] = true
	}

	if !s.set["item"] && !s.set["move-from"] && !s.set["move-item"] {
		render.render(renderStepBlock(steps(), stepItems, "Existing vault to pick items from:\n\nMove items from an existing vault. At least one\nitem is required — a config with an empty\nitems list will never load.") + "\n")

		vaults, err := onepassword.ListVaults()
		if err != nil {
			return nil, fmt.Errorf("listing vaults: %w", err)
		}
		sourceVault, err := pickVault(vaults)
		if err != nil {
			return nil, err
		}

		sourceItems, err := onepassword.ListItems(sourceVault)
		if err != nil {
			return nil, fmt.Errorf("listing items in %s: %w", sourceVault, err)
		}

		// An empty items list is never a valid end state for this tool
		// (see validateComplete), so the wizard must not be able to
		// produce one: re-open the picker against the SAME vault on an
		// empty pick, rather than re-asking for the vault too. pickItems
		// already reports "No items selected..." — no need to repeat it.
		var picked []onepassword.Item
		for len(picked) == 0 {
			picked, err = pickItems(sourceVault, sourceItems)
			if err != nil {
				return nil, fmt.Errorf("picking items from %s: %w", sourceVault, err)
			}
		}
		s.MoveItems = make([]string, len(picked))
		for i, item := range picked {
			s.MoveItems[i] = sourceVault + "/" + item.ID
		}
		answered[stepItems] = true
	}

	if !s.set["ttl"] {
		render.render(renderStepBlock(steps(), stepTTL, "Default cache TTL:") + "\n")
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
		answered[stepTTL] = true
	}

	return s, nil
}

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

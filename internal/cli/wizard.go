package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	"github.com/newtosh/timeshare/internal/config"
	"github.com/newtosh/timeshare/internal/onepassword"
)

const (
	stepVault = iota
	stepMode
	stepItems
	stepSSHKeys
	stepTTL
	stepCount
)

var wizardStepLabels = [stepCount]string{
	stepVault:   "Repo vault name",
	stepMode:    "Auth mode",
	stepItems:   "Items to move",
	stepSSHKeys: "SSH keys",
	stepTTL:     "TTL",
}

var wizardStepHelp = [stepCount]string{
	stepVault:   "The name of a new, dedicated 1Password vault timeshare will create for this repo. Pick something specific to this repo — it shouldn't be shared with unrelated projects.",
	stepMode:    "Biometric: shells out to `op read`, same Touch ID/Windows Hello prompt you already get, cached for the TTL. Service account: headless, token-based, no prompts at all, but requires a manual token-store step after init (see the printed instructions).",
	stepItems:   "Which 1Password items should this project's allow-list include. You can move items from an existing vault, or pick from a list interactively.",
	stepSSHKeys: "Optional: which SSH keys (already stored in 1Password) this repo's `timeshare run` may use, time-boxed by the same TTL as everything else. Keys are never moved or copied — this only records a reference to wherever they already live.",
	stepTTL:     "How long a resolved secret stays cached before the next read re-checks 1Password. Longer means fewer prompts but a longer window before a rotated/revoked secret takes effect.",
}

// breadcrumbRenderer redraws the wizard's breadcrumb line in place: each
// call clears the previous render before printing the next one, instead of
// letting every step's breadcrumb scroll past on its own.
type breadcrumbRenderer struct{ lines int }

func (r *breadcrumbRenderer) render(line string) {
	if r.lines > 0 {
		fmt.Printf("\033[0m\033[%dA\033[J", r.lines)
	}
	fmt.Print(line + "\n")
	fmt.Print("\033[0m")
	r.lines = strings.Count(line, "\n") + 1
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
// RunE) does that once the wizard returns.
//
// Layout: a horizontal breadcrumb (renderBreadcrumb) stays pinned above
// whatever the active step needs — a plain prompt for a text/select step,
// or the full-width fzf-style picker (pickVault/pickItems) for a list step.
// There's no permanent split pane: the picker gets the whole terminal width
// since that's what it needs, matching fzf's own layout.
func runWizard(cwd string, seeded *wizardState) (*wizardState, error) {
	s := &wizardState{
		Vault: seeded.Vault, Mode: seeded.Mode, TTL: seeded.TTL,
		FromVault: seeded.FromVault, Items: seeded.Items, FromItems: seeded.FromItems,
		SSHKeys: seeded.SSHKeys, Force: seeded.Force, set: seeded.set,
	}

	// answered tracks which steps this wizard RUN has actively prompted
	// and gotten a response for, independent of s's field values (which
	// can be non-empty just from cobra flag defaults, e.g. Mode defaults
	// to "biometric" even with zero flags passed). Every step's Done
	// below uses the same rule: seeded-via-flag, or answered just now.
	var answered [stepCount]bool
	render := &breadcrumbRenderer{}

	steps := func() []wizardStep {
		return []wizardStep{
			{Label: wizardStepLabels[stepVault], Value: s.Vault, Done: s.set["vault"] || answered[stepVault]},
			{Label: wizardStepLabels[stepMode], Value: s.Mode, Done: s.set["mode"] || answered[stepMode]},
			{Label: wizardStepLabels[stepItems], Value: itemsSummary(s), Done: s.set["item"] || s.set["from"] || s.set["from-item"] || answered[stepItems]},
			{Label: wizardStepLabels[stepSSHKeys], Value: sshKeysSummary(s), Done: s.set["ssh-key"] || answered[stepSSHKeys]},
			{Label: wizardStepLabels[stepTTL], Value: s.TTL, Done: s.set["ttl"] || answered[stepTTL]},
		}
	}

	if !s.set["vault"] {
		render.render(renderBreadcrumb(steps(), stepVault))
		val, err := promptWithHelp(stepVault, func() (string, error) {
			v := s.Vault
			if v == "" {
				v = defaultVaultName(cwd)
			}
			err := huh.NewInput().Title("Repo vault name").Value(&v).WithTheme(wizardTheme()).Run()
			return v, err
		})
		if err != nil {
			return nil, err
		}
		s.Vault = val
		answered[stepVault] = true
	}

	if !s.set["mode"] {
		render.render(renderBreadcrumb(steps(), stepMode))
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
			WithTheme(wizardTheme()).
			Run(); err != nil {
			return nil, err
		}
		s.Mode = mode
		answered[stepMode] = true
	}

	if !s.set["item"] && !s.set["from"] && !s.set["from-item"] {
		render.render(renderBreadcrumb(steps(), stepItems))
		fmt.Println(lipgloss.NewStyle().Foreground(colorDim).Render("Copy items from an existing vault. At least one item is required."))

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
		// (see validateComplete): requireOne=true makes pickItems itself
		// block enter until at least one item is checked, instead of us
		// looping and re-running the picker (which used to spin up a
		// brand new tea.Program on an empty submit — since it's inline,
		// not alt-screen, that visibly reprinted the whole list below
		// what was already on screen).
		picked, err := pickItems(sourceVault, sourceItems, true)
		if err != nil {
			return nil, fmt.Errorf("picking items from %s: %w", sourceVault, err)
		}
		s.FromItems = make([]string, len(picked))
		for i, item := range picked {
			s.FromItems[i] = sourceVault + "/" + item.ID
		}
		answered[stepItems] = true
	}

	if !s.set["ssh-key"] {
		render.render(renderBreadcrumb(steps(), stepSSHKeys))
		fmt.Println(lipgloss.NewStyle().Foreground(colorDim).Render("Optionally grant this repo access to SSH keys already stored in 1Password. Select none and press enter to skip."))

		vaults, err := onepassword.ListVaults()
		if err != nil {
			return nil, fmt.Errorf("listing vaults: %w", err)
		}
		sourceVault, err := pickVault(vaults)
		if err != nil {
			return nil, err
		}

		sshItems, err := onepassword.ListSSHKeyItems(sourceVault)
		if err != nil {
			return nil, fmt.Errorf("listing SSH keys in %s: %w", sourceVault, err)
		}

		picked, err := pickSSHKeys(sourceVault, sshItems)
		if err != nil {
			return nil, err
		}
		s.SSHKeys = make([]string, len(picked))
		for i, item := range picked {
			s.SSHKeys[i] = sourceVault + "/" + item.ID
		}
		answered[stepSSHKeys] = true
	}

	if !s.set["ttl"] {
		render.render(renderBreadcrumb(steps(), stepTTL))
		val, err := promptWithHelp(stepTTL, func() (string, error) {
			ttl := s.TTL
			if ttl == "" {
				ttl = "4h"
			}
			err := huh.NewInput().Title("Default cache TTL").Value(&ttl).WithTheme(wizardTheme()).Run()
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
	if s.FromVault != "" {
		return "from " + s.FromVault
	}
	return selectedCount(len(s.Items)+len(s.FromItems), "")
}

func sshKeysSummary(s *wizardState) string {
	return selectedCount(len(s.SSHKeys), "none")
}

func selectedCount(n int, empty string) string {
	if n == 0 {
		return empty
	}
	return fmt.Sprintf("%d selected", n)
}

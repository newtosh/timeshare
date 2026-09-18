package cli

import (
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// Wizard color palette. Chosen to match the fzf-style redesign mockup
// approved 2026-09-18 (GitHub Dark Dimmed-derived). No background colors —
// the wizard paints text only and lets the terminal's own background show
// through, matching how fzf itself renders and avoiding the erase/SGR-reset
// class of bugs an explicitly-painted background invites.
var (
	colorDone   = lipgloss.Color("#3fb950") // done step markers, checked items
	colorValue  = lipgloss.Color("#e3b341") // captured step values
	colorActive = lipgloss.Color("#e6edf3") // active breadcrumb label
	colorDim    = lipgloss.Color("#6e7681") // pending steps, stats line, help
	colorFg     = lipgloss.Color("#c9d1d9") // plain foreground text
	colorAccent = lipgloss.Color("#f778ba") // prompt arrow, cursor row marker
	colorSelect = lipgloss.Color("#79c0ff") // selected/cursor row text
	colorSep    = lipgloss.Color("#30363d") // breadcrumb separators
)

// wizardTheme returns a huh.Theme matching the wizard's palette: no left
// border (Option C is a single borderless pane, not a boxed one), pink
// selector/cursor, blue selected option, amber title, green checkmarks.
func wizardTheme() *huh.Theme {
	t := huh.ThemeBase()

	t.Focused.Base = lipgloss.NewStyle()
	t.Blurred.Base = lipgloss.NewStyle()

	t.Focused.Title = t.Focused.Title.Foreground(colorActive).Bold(true)
	t.Focused.SelectSelector = lipgloss.NewStyle().Foreground(colorAccent).SetString("> ")
	t.Focused.MultiSelectSelector = lipgloss.NewStyle().Foreground(colorAccent).SetString("> ")
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(colorSelect)
	t.Focused.SelectedPrefix = lipgloss.NewStyle().Foreground(colorDone).SetString("[x] ")
	t.Focused.UnselectedPrefix = lipgloss.NewStyle().Foreground(colorDim).SetString("[ ] ")
	t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(colorSelect)
	t.Focused.TextInput.Prompt = t.Focused.TextInput.Prompt.Foreground(colorAccent)
	t.Focused.Description = t.Focused.Description.Foreground(colorDim)

	return t
}

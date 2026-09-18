package cli

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// wizardStep is one entry in the breadcrumb.
type wizardStep struct {
	Label string
	Value string
	Done  bool
}

// renderBreadcrumb renders the wizard's step progress as a single
// horizontal line: done steps in green with their captured value in amber,
// the active step bold near-white, the rest dim — separated by "›". No
// background painting: the wizard paints text only and lets the terminal's
// own background show through (matches fzf's own look, and avoids the
// erase/SGR-reset class of bugs an explicitly-painted background invites).
func renderBreadcrumb(steps []wizardStep, activeIdx int) string {
	var parts []string
	for i, st := range steps {
		switch {
		case st.Done:
			label := lipgloss.NewStyle().Foreground(colorDone).Render(st.Label)
			if st.Value != "" {
				label += " " + lipgloss.NewStyle().Foreground(colorValue).Render(st.Value)
			}
			parts = append(parts, label)
		case i == activeIdx:
			parts = append(parts, lipgloss.NewStyle().Foreground(colorActive).Bold(true).Render(st.Label))
		default:
			parts = append(parts, lipgloss.NewStyle().Foreground(colorDim).Render(st.Label))
		}
	}
	sep := lipgloss.NewStyle().Foreground(colorSep).Render(" > ")
	return strings.Join(parts, sep)
}

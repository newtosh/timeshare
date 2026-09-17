package cli

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// wizardStep is one row in the step tracker's left column.
type wizardStep struct {
	Label string
	Value string
	Done  bool
}

var (
	wizardCheckStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	wizardValueStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	wizardActiveStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Bold(true)
	wizardDimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	wizardLeftCol     = lipgloss.NewStyle().Width(40).Padding(0, 2, 1, 1).Background(lipgloss.Color("235"))
	wizardRightCol    = lipgloss.NewStyle().Padding(0, 2, 1, 2).Background(lipgloss.Color("238")).BorderStyle(lipgloss.NormalBorder()).BorderLeft(true).BorderForeground(lipgloss.Color("240"))
)

// renderStepBlock renders one snapshot of the wizard: a left column listing
// every step (checkmark + captured value for done steps, an active marker
// for the current step, dimmed for the rest) beside a right column holding
// activeContent verbatim (the active step's prompt/help text — the caller
// owns what that string contains, this function only lays it out).
func renderStepBlock(steps []wizardStep, activeIdx int, activeContent string) string {
	var left strings.Builder
	for i, st := range steps {
		switch {
		case st.Done:
			left.WriteString(wizardCheckStyle.Render("✓") + " " + st.Label)
			if st.Value != "" {
				left.WriteString("  " + wizardValueStyle.Render(st.Value))
			}
			left.WriteString("\n")
		case i == activeIdx:
			left.WriteString(wizardActiveStyle.Render("▸ "+st.Label) + "\n")
		default:
			left.WriteString(wizardDimStyle.Render("  "+st.Label) + "\n")
		}
	}

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		wizardLeftCol.Render(left.String()),
		wizardRightCol.Render(activeContent),
	)
}

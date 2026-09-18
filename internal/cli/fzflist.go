package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// fzfItem is one row in an fzfList: Label is both what's displayed and what
// the live fuzzy filter matches against. Value is a stable identifier
// (survives filtering/reordering), used as the multi-select checkbox key.
type fzfItem struct {
	Label string
	Value string
}

func (i fzfItem) FilterValue() string { return i.Label }

// fzfDelegate renders one row: a pink "> " marker on the cursor row (plain
// two-space indent otherwise), blue text on the cursor row, a green "[x] "/
// dim "[ ] " checkbox prefix in multi-select mode. No description line, no
// per-item styling beyond that — this is deliberately closer to fzf's
// single-line-per-match look than bubbles/list's default two-line delegate.
// checked is the SAME map instance fzfListModel mutates on toggle (maps
// share underlying storage across copies), so Render always sees current
// state without any extra plumbing.
type fzfDelegate struct {
	multi   bool
	checked map[string]bool
}

func (d fzfDelegate) Height() int                         { return 1 }
func (d fzfDelegate) Spacing() int                        { return 0 }
func (d fzfDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (d fzfDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	it, ok := item.(fzfItem)
	if !ok {
		return
	}

	marker := "  "
	textStyle := lipgloss.NewStyle().Foreground(colorFg)
	if index == m.Index() {
		marker = lipgloss.NewStyle().Foreground(colorAccent).Render("> ")
		textStyle = lipgloss.NewStyle().Foreground(colorSelect)
	}

	checkbox := ""
	if d.multi {
		if d.checked[it.Value] {
			checkbox = lipgloss.NewStyle().Foreground(colorDone).Render("[x] ")
		} else {
			checkbox = lipgloss.NewStyle().Foreground(colorDim).Render("[ ] ")
		}
	}

	_, _ = fmt.Fprint(w, marker+checkbox+textStyle.Render(it.Label))
}

// maxFzfListHeight caps how many rows the picker's list can ever occupy.
// minFzfListHeight is the floor even with 0-1 matches, so the layout
// doesn't jump around too abruptly as the filtered count changes.
const (
	maxFzfListHeight = 14
	minFzfListHeight = 3
)

// resizeListToVisible sets l's height to match its own current visible-item
// count (clamped to [minFzfListHeight, maxFzfListHeight] and to whatever
// room termHeight leaves), instead of always rendering a fixed-size block
// padded with blank rows. Without this, a 5-item list and a 40-item list
// render the same tall block — the gap between the last real row and the
// stats/prompt line below is wasted blank space, and it makes every step
// in the wizard look like a different height depending on how many matches
// happen to be visible, which reads as inconsistent/broken rather than
// intentional.
func resizeListToVisible(l *list.Model, termHeight int) {
	h := len(l.VisibleItems())
	if h < minFzfListHeight {
		h = minFzfListHeight
	}
	if h > maxFzfListHeight {
		h = maxFzfListHeight
	}
	if termHeight > 0 {
		if cap := termHeight - 2; cap >= minFzfListHeight && h > cap {
			h = cap
		}
	}
	l.SetSize(l.Width(), h)
}

// fzfListModel is a minimal fzf-style picker: always-active fuzzy filter,
// no title/pagination/help/status chrome (we render our own stats line,
// prompt, and key hint), single- or multi-select.
//
// Keys: up/down (bubbles/list defaults) to move the cursor, tab to
// autocomplete the filter text to the currently highlighted item's full
// label (which re-filters down to just that item, so enter immediately
// confirms it), space to toggle the highlighted item in multi-select mode
// (a no-op in single-select mode — space is just typed into the filter),
// enter to confirm, esc/ctrl+c to abort.
type fzfListModel struct {
	list       list.Model
	multi      bool
	checked    map[string]bool
	termHeight int
	aborted    bool
	submitted  bool
}

func newFzfListModel(title string, items []fzfItem, multi bool) fzfListModel {
	listItems := make([]list.Item, len(items))
	for i, it := range items {
		listItems[i] = it
	}

	checked := make(map[string]bool)
	d := fzfDelegate{multi: multi, checked: checked}
	l := list.New(listItems, d, 80, maxFzfListHeight)
	l.Title = title
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetShowHelp(false)
	l.SetShowFilter(false) // we render our own prompt line from FilterInput
	l.SetFilteringEnabled(true)
	l.DisableQuitKeybindings()
	l.FilterInput.Prompt = ""
	// SetFilterState(Filtering) alone only flips the state flag — it never
	// computes filteredItems, so VisibleItems() would return empty until
	// the first keystroke. SetFilterText("") does the synchronous
	// computation too (filterItems() special-cases an empty term to mean
	// "everything", matching fzf's own empty-query behavior), but it also
	// leaves filterState at FilterApplied ("user is not editing filter"),
	// which would stop routing keystrokes into the filter box. Flip back
	// to Filtering after — it doesn't touch the already-computed items.
	l.SetFilterText("")
	l.SetFilterState(list.Filtering)
	resizeListToVisible(&l, 0)

	return fzfListModel{
		list:    l,
		multi:   multi,
		checked: checked,
	}
}

func (m fzfListModel) Init() tea.Cmd { return nil }

func (m fzfListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.termHeight = msg.Height
		m.list.SetSize(msg.Width, m.list.Height())
		resizeListToVisible(&m.list, m.termHeight)
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.aborted = true
			return m, tea.Quit
		case "enter":
			m.submitted = true
			return m, tea.Quit
		case "up":
			// bubbles/list only handles CursorUp/CursorDown while
			// filterState != Filtering — since this picker stays in
			// Filtering permanently (so the text box keeps receiving
			// keystrokes; see newFzfListModel), those built-in bindings
			// never fire and up/down would otherwise land in
			// handleFiltering(), which only recognizes accept/cancel
			// bindings and the text input itself. Move the cursor
			// ourselves instead of forwarding to list.Update().
			m.list.CursorUp()
			return m, nil
		case "down":
			m.list.CursorDown()
			return m, nil
		case "tab":
			// Autocomplete to the currently highlighted item's full label.
			// Re-filtering to that exact text narrows VisibleItems down to
			// just it, so a following enter confirms it directly.
			if it, ok := m.list.SelectedItem().(fzfItem); ok {
				m.list.SetFilterText(it.Label)
				m.list.SetFilterState(list.Filtering)
			}
			resizeListToVisible(&m.list, m.termHeight)
			return m, nil
		case " ":
			if m.multi {
				if it, ok := m.list.SelectedItem().(fzfItem); ok {
					m.checked[it.Value] = !m.checked[it.Value]
				}
				return m, nil
			}
			// Single-select: space is just typed filter text — fall
			// through to the normal list.Update() below.
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	resizeListToVisible(&m.list, m.termHeight)
	return m, cmd
}

func (m fzfListModel) View() string {
	var b strings.Builder
	b.WriteString(m.list.View())
	b.WriteString("\n")

	stats := fmt.Sprintf("%d/%d", len(m.list.VisibleItems()), len(m.list.Items()))
	b.WriteString(lipgloss.NewStyle().Foreground(colorDim).Render(stats))
	b.WriteString("\n")

	prompt := lipgloss.NewStyle().Foreground(colorAccent).Render("> ")
	b.WriteString(prompt + m.list.FilterInput.View())
	b.WriteString("\n")

	hint := "up/down navigate  tab complete  enter select"
	if m.multi {
		hint = "up/down navigate  tab complete  space toggle  enter confirm"
	}
	b.WriteString(lipgloss.NewStyle().Foreground(colorDim).Render(hint))
	return b.String()
}

// runFzfList runs the picker to completion and returns the chosen items:
// in single mode, the highlighted item at Enter; in multi mode, everything
// toggled with space (Enter with nothing toggled returns empty — the
// caller decides whether that's acceptable, matching pickItems's existing
// contract).
func runFzfList(title string, items []fzfItem, multi bool) ([]fzfItem, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("nothing to pick from")
	}

	m := newFzfListModel(title, items, multi)
	result, err := tea.NewProgram(m).Run()
	if err != nil {
		return nil, fmt.Errorf("picker: %w", err)
	}
	final := result.(fzfListModel)
	if final.aborted {
		return nil, fmt.Errorf("user aborted")
	}

	if !multi {
		if it, ok := final.list.SelectedItem().(fzfItem); ok {
			return []fzfItem{it}, nil
		}
		return nil, nil
	}

	var chosen []fzfItem
	for _, it := range items {
		if final.checked[it.Value] {
			chosen = append(chosen, it)
		}
	}
	return chosen, nil
}

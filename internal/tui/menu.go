package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// MenuModel is the landing screen: a simple cursor-based list of actions.
type MenuModel struct {
	cursor int
}

func NewMenuModel() MenuModel { return MenuModel{} }

// items returns the menu's current entries; "Resume live debate" only
// appears while a debate is running.
func (m MenuModel) items(live *LiveDebate) []string {
	items := []string{"Start a new debate", "Browse archive"}
	if live != nil && !live.IsDone() {
		items = append(items, "Resume live debate")
	}
	return items
}

// Update handles a keypress. Screen transitions are requested via
// SwitchScreenMsg commands; the root model owns the actual switch.
func (m MenuModel) Update(msg tea.KeyMsg, live *LiveDebate) (MenuModel, tea.Cmd) {
	items := m.items(live)
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(items)-1 {
			m.cursor++
		}
	case "enter":
		switch items[m.cursor] {
		case "Start a new debate":
			return m, func() tea.Msg { return SwitchScreenMsg{Screen: ScreenForm} }
		case "Browse archive":
			return m, func() tea.Msg { return SwitchScreenMsg{Screen: ScreenArchive} }
		case "Resume live debate":
			return m, func() tea.Msg { return SwitchScreenMsg{Screen: ScreenSession} }
		}
	}
	return m, nil
}

func (m MenuModel) View(live *LiveDebate) string {
	var b strings.Builder
	b.WriteString("agon — two-agent debates\n\n")
	for i, item := range m.items(live) {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		fmt.Fprintf(&b, "%s%s\n", cursor, item)
	}
	b.WriteString("\n↑/↓ select · enter confirm · ctrl+c quit\n")
	return b.String()
}

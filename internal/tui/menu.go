package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// menuModel is the app's landing screen: a simple cursor-based list of
// actions.
type menuModel struct {
	cursor int
}

func newMenuModel() menuModel { return menuModel{} }

// items returns the menu's current entries; "Resume live debate" only
// appears while a debate is running.
func (m menuModel) items(live *liveDebate) []string {
	items := []string{"Start a new debate", "Browse archive"}
	if live != nil && !live.isDone() {
		items = append(items, "Resume live debate")
	}
	return items
}

func (m menuModel) update(msg tea.KeyMsg, a *App) (tea.Model, tea.Cmd) {
	items := m.items(a.live)
	switch msg.String() {
	case "up", "k":
		if a.menu.cursor > 0 {
			a.menu.cursor--
		}
	case "down", "j":
		if a.menu.cursor < len(items)-1 {
			a.menu.cursor++
		}
	case "enter":
		switch items[a.menu.cursor] {
		case "Start a new debate":
			a.form = newFormModel(a.defaultModel)
			return a, func() tea.Msg { return switchScreenMsg{screen: screenForm} }
		case "Browse archive":
			return a, func() tea.Msg { return switchScreenMsg{screen: screenArchive} }
		case "Resume live debate":
			a.session.showLive(a.live)
			a.screen = screenSession
			return a, waitForLiveUpdate(a.live)
		}
	}
	return a, nil
}

func (m menuModel) View(live *liveDebate) string {
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

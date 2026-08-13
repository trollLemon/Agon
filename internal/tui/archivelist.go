package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/trollLemon/agon/internal/archive"
)

// archiveListModel is a cursor-based browser over archived sessions
// (docs/SPEC.md D8).
type archiveListModel struct {
	dir    string
	items  []archive.Session
	cursor int
	err    error
}

func newArchiveListModel(dir string) archiveListModel {
	return archiveListModel{dir: dir}
}

// reload returns a tea.Cmd that re-reads the archive directory.
func (m archiveListModel) reload() tea.Cmd {
	dir := m.dir
	return func() tea.Msg {
		items, err := archive.List(dir)
		if err != nil {
			return archiveListLoadedMsg{}
		}
		return archiveListLoadedMsg{items: items}
	}
}

func (m *archiveListModel) setItems(items []archive.Session) {
	m.items = items
	if m.cursor >= len(items) {
		m.cursor = max(0, len(items)-1)
	}
}

func (m *archiveListModel) update(msg tea.KeyMsg, a *App) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return a, func() tea.Msg { return switchScreenMsg{screen: screenMenu} }
	case "up", "k":
		if a.archiveList.cursor > 0 {
			a.archiveList.cursor--
		}
	case "down", "j":
		if a.archiveList.cursor < len(a.archiveList.items)-1 {
			a.archiveList.cursor++
		}
	case "enter":
		if len(a.archiveList.items) == 0 {
			return a, nil
		}
		sessionID := a.archiveList.items[a.archiveList.cursor].SessionID
		return a, func() tea.Msg { return openArchivedMsg{sessionID: sessionID} }
	}
	return a, nil
}

func (m archiveListModel) View() string {
	var b strings.Builder
	b.WriteString("Archived debates\n\n")
	if len(m.items) == 0 {
		b.WriteString("(none yet)\n")
	}
	for i, s := range m.items {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		status := " "
		if len(s.Aborted) > 0 {
			status = "✗"
		} else if s.Verdict != "" {
			status = "✓"
		}
		fmt.Fprintf(&b, "%s%s %-40s  %s  %s\n", cursor, status, truncate(s.Title, 40),
			s.CreatedAt.Local().Format("2006-01-02 15:04"), s.Mode)
	}
	b.WriteString("\n↑/↓ select · enter open · esc back\n")
	return b.String()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

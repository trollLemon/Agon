package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/trollLemon/agon/internal/archive"
)

// ArchiveListModel is a cursor-based browser over archived sessions.
type ArchiveListModel struct {
	dir    string
	items  []archive.Session
	cursor int
	err    error
}

func NewArchiveListModel(dir string) ArchiveListModel {
	return ArchiveListModel{dir: dir}
}

// Reload returns a tea.Cmd that re-reads the archive directory.
func (m ArchiveListModel) Reload() tea.Cmd {
	dir := m.dir
	return func() tea.Msg {
		items, err := archive.List(dir)
		if err != nil {
			return ArchiveListLoadedMsg{}
		}
		return ArchiveListLoadedMsg{Items: items}
	}
}

func (m *ArchiveListModel) SetItems(items []archive.Session) {
	m.items = items
	if m.cursor >= len(items) {
		m.cursor = max(0, len(items)-1)
	}
}

// Update handles a keypress. Opening a session or leaving the screen is
// requested via OpenArchivedMsg / SwitchScreenMsg commands.
func (m ArchiveListModel) Update(msg tea.KeyMsg) (ArchiveListModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return m, func() tea.Msg { return SwitchScreenMsg{Screen: ScreenMenu} }
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case "enter":
		if len(m.items) == 0 {
			return m, nil
		}
		sessionID := m.items[m.cursor].SessionID
		return m, func() tea.Msg { return OpenArchivedMsg{SessionID: sessionID} }
	}
	return m, nil
}

func (m ArchiveListModel) View() string {
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

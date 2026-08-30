// Copyright (C) 2026  trolllemon
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/trollLemon/agon/internal/orchestrator"
)

// QueueListModel is a simple read-only list of queued debates.
type QueueListModel struct {
	items  []*orchestrator.Debate
	cursor int
}

func NewQueueListModel() QueueListModel { return QueueListModel{} }

func (m *QueueListModel) SetItems(items []*orchestrator.Debate) {
	m.items = items
	if m.cursor >= len(items) {
		m.cursor = max(0, len(items)-1)
	}
}

func (m QueueListModel) Update(msg tea.KeyMsg) (QueueListModel, tea.Cmd) {
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
		// go to live session view for the selected queued debate
		return m, func() tea.Msg { return SwitchScreenMsg{Screen: ScreenSession} }
	}
	return m, nil
}

func (m QueueListModel) View() string {
	var b strings.Builder
	b.WriteString("Queued debates\n\n")
	if len(m.items) == 0 {
		b.WriteString("(none)\n")
	}
	for i, d := range m.items {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		cfg := d.Config()
		status := "[Queued]"
		if i == 0 {
			status = "[Live]"
		}
		fmt.Fprintf(&b, "%s%s %-40s  %s\n", cursor, status, truncate(cfg.Title, 40), cfg.Topic)
	}
	b.WriteString("\n↑/↓ select · enter open · esc back\n")
	return b.String()
}

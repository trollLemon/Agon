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

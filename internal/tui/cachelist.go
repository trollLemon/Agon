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
	"github.com/trollLemon/agon/internal/types"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/trollLemon/agon/internal/cache"
)

// CacheListModel is a cursor-based browser over interrupted cached sessions.
// The UI title is "Resume" but the code name reflects the technical store.
type CacheListModel struct {
	dir    string
	items  []*types.Session
	cursor int
	err    error
}

func NewCacheListModel(dir string) CacheListModel {
	return CacheListModel{dir: dir}
}

// Reload returns a tea.Cmd that re-reads the cache directory for interrupted sessions.
func (m CacheListModel) Reload() tea.Cmd {
	dir := m.dir
	return func() tea.Msg {
		items, err := cache.ListInterrupted(dir)
		if err != nil {
			return CacheListLoadedMsg{}
		}
		return CacheListLoadedMsg{Items: items}
	}
}

func (m *CacheListModel) SetItems(items []*types.Session) {
	m.items = items
	if m.cursor >= len(items) {
		m.cursor = max(0, len(items)-1)
	}
}

func (m *CacheListModel) Items() []*types.Session { return m.items }

// Update handles a keypress. Opening a cached session or leaving the screen is
// requested via OpenCachedMsg / SwitchScreenMsg commands.
func (m CacheListModel) Update(msg tea.KeyMsg) (CacheListModel, tea.Cmd) {
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
		return m, func() tea.Msg { return OpenCachedMsg{SessionID: sessionID} }
	}
	return m, nil
}

func (m CacheListModel) View() string {
	var b strings.Builder
	b.WriteString("Resume interrupted debates\n\n")
	if len(m.items) == 0 {
		b.WriteString("(none interrupted — cache is empty)\n")
	}
	for i, s := range m.items {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		roundInfo := ""
		if s.Rounds > 0 {
			doneRounds := len(s.Messages) / 2
			roundInfo = fmt.Sprintf(" (%d/%d rounds done)", doneRounds, s.Rounds)
		}
		fmt.Fprintf(&b, "%s%-40s  %s  %s%s\n", cursor, truncate(s.Title, 40),
			s.CreatedAt.Local().Format("2006-01-02 15:04"), s.Mode, roundInfo)
	}
	b.WriteString("\n↑/↓ select · enter resume · esc back\n")
	return b.String()
}

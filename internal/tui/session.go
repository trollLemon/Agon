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
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"

	"github.com/trollLemon/agon/internal/archive"
	"github.com/trollLemon/agon/internal/orchestrator"
)

// SessionView is a rendering-neutral snapshot of a debate: it comes either
// from a live debate or a loaded archive.Session, so the screen only
// needs one render path.
type SessionView struct {
	SessionID string
	Title     string
	Topic     string
	Mode      string
	Tone      string
	Rounds    int
	Sides     []archive.Side

	Messages []archive.Message

	CurrentRole    string
	CurrentRound   int
	CurrentContent string
	CurrentTools   []archive.ToolCall

	Verdict string
	Live    bool
	Done    bool
	Err     error
}

func fromArchivedSession(sess archive.Session) SessionView {
	return SessionView{
		SessionID: sess.SessionID, Title: sess.Title, Topic: sess.Topic, Mode: sess.Mode, Tone: sess.Tone,
		Rounds: sess.Rounds, Sides: sess.Sides, Messages: sess.Messages,
		Verdict: sess.Verdict, Live: false, Done: true,
	}
}

// SessionModel renders the exclusive full-screen session view: a header,
// a scrolling transcript, and an abort confirmation prompt.
type SessionModel struct {
	width, height int
	viewport      viewport.Model
	view          SessionView
	confirmAbort  bool

	mdRenderer      *glamour.TermRenderer // cached; glamour.NewTermRenderer is expensive to create
	mdRendererWidth int
}

func NewSessionModel() SessionModel {
	vp := viewport.New(80, 20)
	return SessionModel{viewport: vp}
}

func (m *SessionModel) SetSize(w, h int) {
	m.width, m.height = w, h
	headerHeight := 2
	m.viewport.Width = w
	if h > headerHeight {
		m.viewport.Height = h - headerHeight
	}
	m.refreshViewportContent()
}

func (m *SessionModel) SessionID() string { return m.view.SessionID }

func (m *SessionModel) SetView(v SessionView) {
	m.view = v
	m.confirmAbort = false
	m.refreshViewportContent()
	if v.Live && !v.Done {
		m.viewport.GotoBottom()
	} else {
		m.viewport.GotoTop()
	}
}

func (m *SessionModel) ShowArchived(sess archive.Session) {
	m.view = fromArchivedSession(sess)
	m.confirmAbort = false
	m.refreshViewportContent()
	m.viewport.GotoTop()
}

func (m *SessionModel) refreshViewportContent() {
	m.viewport.SetContent(m.renderTranscript())
}

// renderTranscript builds the debate transcript as markdown and, if
// possible, renders it through a cached glamour renderer. The renderer is
// recreated only when the width changes, since constructing one is
// expensive (it parses a style sheet) and this runs on every live-update
// poll during a debate.
func (m *SessionModel) renderTranscript() string {
	width := m.viewport.Width
	if width <= 0 {
		width = 80
	}
	text := transcriptMarkdown(m.view)

	if m.mdRenderer == nil || m.mdRendererWidth != width {
		r, err := glamour.NewTermRenderer(glamourStyleOption(), glamour.WithWordWrap(width))
		if err != nil {
			return text
		}
		m.mdRenderer = r
		m.mdRendererWidth = width
	}
	out, err := m.mdRenderer.Render(text)
	if err != nil {
		return text
	}
	return out
}

// glamourStyleOption picks a glamour style without ever auto-detecting the
// terminal's background color. glamour.WithAutoStyle queries the terminal
// with an OSC escape sequence and blocks for its reply; doing that while
// Bubble Tea's own input reader is already consuming raw stdin races with
// it and can stall the whole app for seconds, or leave it looking frozen
// until unrelated input (a keypress, a resize) perturbs the race. GLAMOUR_
// STYLE still lets a user pick "light"/"notty"/a custom style path.
func glamourStyleOption() glamour.TermRendererOption {
	if s := os.Getenv("GLAMOUR_STYLE"); s != "" && s != "auto" {
		return glamour.WithStylePath(s)
	}
	return glamour.WithStandardStyle("dark")
}

// Update handles a keypress. Leaving the screen is
// requested via SwitchScreenMsg.
func (m SessionModel) Update(msg tea.KeyMsg, curDebate *orchestrator.Debate) (SessionModel, tea.Cmd) {
	if m.confirmAbort {
		switch msg.String() {
		case "y":
			m.confirmAbort = false
			if curDebate != nil {
				curDebate.Abort("aborted by user")
			}
			return m, nil
		default:
			m.confirmAbort = false
			return m, nil
		}
	}

	switch msg.String() {
	case "esc":
		return m, func() tea.Msg { return SwitchScreenMsg{Screen: ScreenMenu} }
	case "a":
		if m.view.Live && !m.view.Done {
			m.confirmAbort = true
		}
		return m, nil
	case "j", "down":
		m.viewport.ScrollDown(1)
		return m, nil
	case "k", "up":
		m.viewport.ScrollUp(1)
		return m, nil
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m *SessionModel) View() string {
	var b strings.Builder
	b.WriteString(headerLine(m.view, m.width))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", max(0, m.width)))
	b.WriteString("\n")
	b.WriteString(m.viewport.View())
	if m.confirmAbort {
		b.WriteString("\n")
		b.WriteString("Abort this debate? Its transcript will be discarded. (y/n)")
	}
	return b.String()
}

func headerLine(v SessionView, width int) string {
	indicator := "◾ archived"
	if v.Live && !v.Done {
		indicator = "● live"
	} else if v.Live && v.Done {
		indicator = "✓ finished"
	}
	round := v.CurrentRound
	if round == 0 && len(v.Messages) > 0 {
		round = v.Messages[len(v.Messages)-1].Round
	}
	if v.Verdict != "" {
		round = v.Rounds
	}
	title := v.Title
	if title == "" {
		title = "Debate"
	}
	line := fmt.Sprintf("%s  %s  round %d/%d  tone:%s  mode:%s", indicator, title, round, v.Rounds, v.Tone, v.Mode)
	if width > 0 && len(line) > width {
		line = line[:width]
	}
	return line
}

// transcriptMarkdown builds the debate transcript as plain markdown (no
// terminal rendering) from a SessionView.
func transcriptMarkdown(v SessionView) string {
	labels := make(map[string]string, len(v.Sides))
	for _, s := range v.Sides {
		labels[s.ID] = s.Label
	}
	labelFor := func(role string) string {
		if l, ok := labels[role]; ok {
			return l
		}
		if role == "judge" {
			return "Judge"
		}
		return role
	}

	var b strings.Builder
	for _, m := range v.Messages {
		fmt.Fprintf(&b, "**%s** _(round %d)_\n\n%s\n\n", labelFor(m.Role), m.Round, m.Content)
		for _, tc := range m.ToolCalls {
			fmt.Fprintf(&b, "> ⚙ %s(%s) → %s\n\n", tc.Name, tc.Args, tc.ResultSummary)
		}
	}
	if v.CurrentRole != "" {
		fmt.Fprintf(&b, "**%s** _(round %d, typing…)_\n\n%s\n\n", labelFor(v.CurrentRole), v.CurrentRound, v.CurrentContent)
		for _, tc := range v.CurrentTools {
			fmt.Fprintf(&b, "> ⚙ %s(%s) → %s\n\n", tc.Name, tc.Args, tc.ResultSummary)
		}
	}
	if v.Verdict != "" {
		fmt.Fprintf(&b, "---\n\n### Verdict\n\n%s\n\n", v.Verdict)
	}
	if v.Err != nil {
		fmt.Fprintf(&b, "---\n\n**Error:** %s\n", v.Err.Error())
	}
	return b.String()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

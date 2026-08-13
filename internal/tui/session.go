package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"

	"github.com/agentdisc/goagentdisc/internal/archive"
)

// sessionView is a rendering-neutral snapshot of a debate: it comes either
// from a live liveDebate or a loaded archive.Session, so the screen only
// needs one render path.
type sessionView struct {
	sessionID string
	title     string
	topic     string
	mode      string
	tone      string
	rounds    int
	sides     []archive.Side

	messages []archive.Message

	currentRole    string
	currentRound   int
	currentContent string
	currentTools   []archive.ToolCall

	verdict string
	live    bool
	done    bool
	err     error
}

func fromLiveSnapshot(s liveSnapshot) sessionView {
	return sessionView{
		sessionID: s.sessionID, title: s.title, topic: s.topic, mode: s.mode, tone: s.tone,
		rounds: s.rounds, sides: s.sides, messages: s.messages,
		currentRole: s.currentRole, currentRound: s.currentRound, currentContent: s.currentContent,
		currentTools: s.currentTools, verdict: s.verdict, live: true, done: s.done, err: s.err,
	}
}

func fromArchivedSession(sess archive.Session) sessionView {
	return sessionView{
		sessionID: sess.SessionID, title: sess.Title, topic: sess.Topic, mode: sess.Mode, tone: sess.Tone,
		rounds: sess.Rounds, sides: sess.Sides, messages: sess.Messages,
		verdict: sess.Verdict, live: false, done: true,
	}
}

// sessionModel renders the exclusive full-screen session view (docs/SPEC.md
// D8): a header, a scrolling transcript, and an abort confirmation prompt.
type sessionModel struct {
	width, height int
	viewport      viewport.Model
	view          sessionView
	confirmAbort  bool

	mdRenderer      *glamour.TermRenderer // cached; glamour.NewTermRenderer is expensive to create
	mdRendererWidth int
}

func newSessionModel() sessionModel {
	vp := viewport.New(80, 20)
	return sessionModel{viewport: vp}
}

func (m *sessionModel) setSize(w, h int) {
	m.width, m.height = w, h
	headerHeight := 2
	m.viewport.Width = w
	if h > headerHeight {
		m.viewport.Height = h - headerHeight
	}
	m.refreshViewportContent()
}

func (m *sessionModel) sessionID() string { return m.view.sessionID }

func (m *sessionModel) showLive(ld *liveDebate) {
	m.view = fromLiveSnapshot(ld.snapshot())
	m.confirmAbort = false
	m.refreshViewportContent()
	m.viewport.GotoBottom()
}

func (m *sessionModel) refreshLive(ld *liveDebate) {
	m.view = fromLiveSnapshot(ld.snapshot())
	m.refreshViewportContent()
	m.viewport.GotoBottom()
}

func (m *sessionModel) showArchived(sess archive.Session) {
	m.view = fromArchivedSession(sess)
	m.confirmAbort = false
	m.refreshViewportContent()
	m.viewport.GotoTop()
}

func (m *sessionModel) refreshViewportContent() {
	m.viewport.SetContent(m.renderTranscript())
}

// renderTranscript builds the debate transcript as markdown and, if
// possible, renders it through a cached glamour renderer. The renderer is
// recreated only when the width changes, since constructing one is
// expensive (it parses a style sheet) and this runs on every live-update
// poll during a debate.
func (m *sessionModel) renderTranscript() string {
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

func (m *sessionModel) update(msg tea.KeyMsg, a *App) (tea.Model, tea.Cmd) {
	if m.confirmAbort {
		switch msg.String() {
		case "y":
			m.confirmAbort = false
			if a.live != nil {
				a.live.debate.Abort("aborted by user")
			}
			return a, nil
		default:
			m.confirmAbort = false
			return a, nil
		}
	}

	switch msg.String() {
	case "esc":
		return a, func() tea.Msg { return switchScreenMsg{screen: screenMenu} }
	case "a":
		if m.view.live && !m.view.done {
			m.confirmAbort = true
		}
		return a, nil
	case "j", "down":
		m.viewport.LineDown(1)
		return a, nil
	case "k", "up":
		m.viewport.LineUp(1)
		return a, nil
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return a, cmd
}

func (m *sessionModel) View() string {
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

func headerLine(v sessionView, width int) string {
	indicator := "◾ archived"
	if v.live && !v.done {
		indicator = "● live"
	} else if v.live && v.done {
		indicator = "✓ finished"
	}
	round := v.currentRound
	if round == 0 && len(v.messages) > 0 {
		round = v.messages[len(v.messages)-1].Round
	}
	if v.verdict != "" {
		round = v.rounds
	}
	title := v.title
	if title == "" {
		title = "Debate"
	}
	line := fmt.Sprintf("%s  %s  round %d/%d  tone:%s  mode:%s", indicator, title, round, v.rounds, v.tone, v.mode)
	if width > 0 && len(line) > width {
		line = line[:width]
	}
	return line
}

// transcriptMarkdown builds the debate transcript as plain markdown (no
// terminal rendering) from a sessionView.
func transcriptMarkdown(v sessionView) string {
	labels := make(map[string]string, len(v.sides))
	for _, s := range v.sides {
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
	for _, m := range v.messages {
		fmt.Fprintf(&b, "**%s** _(round %d)_\n\n%s\n\n", labelFor(m.Role), m.Round, m.Content)
		for _, tc := range m.ToolCalls {
			fmt.Fprintf(&b, "> ⚙ %s(%s) → %s\n\n", tc.Name, tc.Args, tc.ResultSummary)
		}
	}
	if v.currentRole != "" {
		fmt.Fprintf(&b, "**%s** _(round %d, typing…)_\n\n%s\n\n", labelFor(v.currentRole), v.currentRound, v.currentContent)
		for _, tc := range v.currentTools {
			fmt.Fprintf(&b, "> ⚙ %s(%s) → %s\n\n", tc.Name, tc.Args, tc.ResultSummary)
		}
	}
	if v.verdict != "" {
		fmt.Fprintf(&b, "---\n\n### Verdict\n\n%s\n\n", v.verdict)
	}
	if v.err != nil {
		fmt.Fprintf(&b, "---\n\n**Error:** %s\n", v.err.Error())
	}
	return b.String()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

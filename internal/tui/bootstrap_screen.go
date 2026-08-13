package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

const bootstrapHeader = "Loading model… this can take a while on first run " +
	"(downloading native libraries and model weights)."

// bootstrapModel renders the model-bootstrap boot log as its own full-window
// screen: a header, a horizontal rule, and a scrolling viewport that follows
// the tail of the log while it streams. It replaces the earlier design where
// only the last few boot-log lines were appended below the new-debate form,
// which left most of the terminal blank and clipped the history.
//
// The boot log is written from the bootstrap goroutine, outside the Bubble
// Tea event loop, so new lines are pulled in by the steady bootLogTickMsg
// poll (App.Update -> refresh), NOT by user input. handleKey only reacts to
// scroll/esc keypresses.
type bootstrapModel struct {
	width, height int
	viewport      viewport.Model
	log           *bootLog
	err           error
}

func newBootstrapModel() bootstrapModel {
	return bootstrapModel{viewport: viewport.New(80, 20)}
}

func (m *bootstrapModel) setSize(w, h int) {
	m.width, m.height = w, h
	headerHeight := 2 // header line + rule line
	m.viewport.Width = w
	if h > headerHeight {
		m.viewport.Height = h - headerHeight
	}
	m.refreshContent()
}

// start binds the model to a fresh boot log and jumps to the bottom so the
// newest lines are in view as they arrive.
func (m *bootstrapModel) start(bl *bootLog) {
	m.log = bl
	m.err = nil
	m.refreshContent()
	m.viewport.GotoBottom()
}

// setError records a bootstrap failure so it renders beneath the log.
func (m *bootstrapModel) setError(err error) {
	m.err = err
	m.refreshContent()
	m.viewport.GotoBottom()
}

// refresh re-reads the boot log; it is called from the bootLogTickMsg poll
// while bootstrap is in flight. It keeps the tail pinned to the bottom unless
// the user has scrolled up to read earlier output.
func (m *bootstrapModel) refresh() {
	atBottom := m.viewport.AtBottom()
	m.refreshContent()
	if atBottom {
		m.viewport.GotoBottom()
	}
}

func (m *bootstrapModel) refreshContent() {
	var b strings.Builder
	if m.log != nil {
		b.WriteString(strings.Join(m.log.tail(maxBootLogLines), "\n"))
	}
	if m.err != nil {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "Error loading model: %v\n\nPress esc to go back.", m.err)
	}
	m.viewport.SetContent(b.String())
}

// handleKey processes only user keypresses (scrolling, and esc-to-return
// after a failure). Live log growth is handled by refresh(), driven by the
// tick poll — never by keys.
func (m *bootstrapModel) handleKey(msg tea.KeyMsg, a *App) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.err != nil {
			a.screen = screenForm
			return a, nil
		}
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

func (m *bootstrapModel) View() string {
	header := bootstrapHeader
	if m.width > 0 && len(header) > m.width {
		header = header[:m.width]
	}
	var b strings.Builder
	b.WriteString(header)
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", max(0, m.width)))
	b.WriteString("\n")
	b.WriteString(m.viewport.View())
	return b.String()
}

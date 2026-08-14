package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

const bootstrapHeader = "Loading model… this can take a while on first run " +
	"(downloading native libraries and model weights)."

// BootstrapModel renders the model-bootstrap boot log as its own full-window
// screen: a header, a horizontal rule, and a scrolling viewport that follows
// the tail of the log while it streams. It replaces the earlier design where
// only the last few boot-log lines were appended below the new-debate form,
// which left most of the terminal blank and clipped the history.
//
// The boot log is written from the bootstrap goroutine, outside the Bubble
// Tea event loop, so new lines are pulled in by the steady BootLogTickMsg
// poll (root model -> Refresh), NOT by user input. HandleKey only reacts to
// scroll/esc keypresses.
type BootstrapModel struct {
	width, height int
	viewport      viewport.Model
	log           *BootLog
	err           error
}

func NewBootstrapModel() BootstrapModel {
	return BootstrapModel{viewport: viewport.New(80, 20)}
}

func (m *BootstrapModel) SetSize(w, h int) {
	m.width, m.height = w, h
	headerHeight := 2 // header line + rule line
	m.viewport.Width = w
	if h > headerHeight {
		m.viewport.Height = h - headerHeight
	}
	m.refreshContent()
}

// Start binds the model to a fresh boot log and jumps to the bottom so the
// newest lines are in view as they arrive.
func (m *BootstrapModel) Start(bl *BootLog) {
	m.log = bl
	m.err = nil
	m.refreshContent()
	m.viewport.GotoBottom()
}

// SetError records a bootstrap failure so it renders beneath the log.
func (m *BootstrapModel) SetError(err error) {
	m.err = err
	m.refreshContent()
	m.viewport.GotoBottom()
}

// Refresh re-reads the boot log; it is called from the BootLogTickMsg poll
// while bootstrap is in flight. It keeps the tail pinned to the bottom unless
// the user has scrolled up to read earlier output.
func (m *BootstrapModel) Refresh() {
	atBottom := m.viewport.AtBottom()
	m.refreshContent()
	if atBottom {
		m.viewport.GotoBottom()
	}
}

func (m *BootstrapModel) refreshContent() {
	var b strings.Builder
	if m.log != nil {
		b.WriteString(strings.Join(m.log.Tail(maxBootLogLines), "\n"))
	}
	if m.err != nil {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "Error loading model: %v\n\nPress esc to go back.", m.err)
	}
	m.viewport.SetContent(b.String())
}

// HandleKey processes only user keypresses (scrolling, and esc-to-return
// after a failure). Live log growth is handled by Refresh(), driven by the
// tick poll — never by keys.
func (m BootstrapModel) HandleKey(msg tea.KeyMsg) (BootstrapModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.err != nil {
			return m, func() tea.Msg { return SwitchScreenMsg{Screen: ScreenForm} }
		}
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

func (m *BootstrapModel) View() string {
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

package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/trollLemon/agon/internal/prompts"
)

// formField identifies one focusable element of the new-debate form
// (topic, starting context, mode, tone, rounds, model).
type formField int

const (
	fieldTopic formField = iota
	fieldContext
	fieldMode
	fieldTone
	fieldRounds
	fieldModel
	fieldCount
)

// FormModel is the new-debate form.
type FormModel struct {
	focus   formField
	topic   textinput.Model
	context textarea.Model
	mode    prompts.Mode
	tone    prompts.Tone
	rounds  textinput.Model
	model   textinput.Model
	errMsg  string
}

func NewFormModel(defaultModel string) FormModel {
	topic := textinput.New()
	topic.Placeholder = "The proposition or X vs Y under debate"
	topic.Focus()

	ctx := textarea.New()
	ctx.Placeholder = "Starting context: constraints, links, a repo path to ground the debate in real code…"
	ctx.SetHeight(4)

	rounds := textinput.New()
	rounds.Placeholder = strconv.Itoa(prompts.DefaultRounds)
	rounds.SetValue(strconv.Itoa(prompts.DefaultRounds))
	rounds.CharLimit = 2

	model := textinput.New()
	model.SetValue(defaultModel)

	return FormModel{
		focus:   fieldTopic,
		topic:   topic,
		context: ctx,
		mode:    prompts.DefaultMode,
		tone:    prompts.DefaultTone,
		rounds:  rounds,
		model:   model,
	}
}

func (m FormModel) Update(msg tea.KeyMsg) (FormModel, tea.Cmd) {
	switch msg.String() {
	case "tab":
		m.setFocus((m.focus + 1) % fieldCount)
		return m, nil
	case "shift+tab":
		m.setFocus((m.focus - 1 + fieldCount) % fieldCount)
		return m, nil
	case "ctrl+s":
		return m.submit()
	case "esc":
		return m, func() tea.Msg { return SwitchScreenMsg{Screen: ScreenMenu} }
	}

	switch m.focus {
	case fieldMode:
		switch msg.String() {
		case "left", "right", "enter":
			m.mode = otherMode(m.mode)
		}
		return m, nil
	case fieldTone:
		switch msg.String() {
		case "left", "right", "enter":
			m.tone = nextTone(m.tone)
		}
		return m, nil
	}

	var cmd tea.Cmd
	switch m.focus {
	case fieldTopic:
		if msg.String() == "enter" {
			m.setFocus(fieldContext)
			return m, nil
		}
		m.topic, cmd = m.topic.Update(msg)
	case fieldContext:
		m.context, cmd = m.context.Update(msg)
	case fieldRounds:
		if msg.String() == "enter" {
			m.setFocus(fieldModel)
			return m, nil
		}
		m.rounds, cmd = m.rounds.Update(msg)
	case fieldModel:
		if msg.String() == "enter" {
			return m.submit()
		}
		m.model, cmd = m.model.Update(msg)
	}
	return m, cmd
}

// SetError records a validation- or app-level error to render in the form.
func (m *FormModel) SetError(msg string) { m.errMsg = msg }

func (m *FormModel) setFocus(f formField) {
	m.focus = f
	m.topic.Blur()
	m.context.Blur()
	m.rounds.Blur()
	m.model.Blur()
	switch f {
	case fieldTopic:
		m.topic.Focus()
	case fieldContext:
		m.context.Focus()
	case fieldRounds:
		m.rounds.Focus()
	case fieldModel:
		m.model.Focus()
	}
}

func (m FormModel) submit() (FormModel, tea.Cmd) {
	topic := strings.TrimSpace(m.topic.Value())
	if topic == "" {
		m.errMsg = "topic is required"
		m.setFocus(fieldTopic)
		return m, nil
	}
	rounds, err := strconv.Atoi(strings.TrimSpace(m.rounds.Value()))
	if err != nil || rounds < 1 {
		m.errMsg = "rounds must be a positive integer"
		m.setFocus(fieldRounds)
		return m, nil
	}
	modelSource := strings.TrimSpace(m.model.Value())

	msg := StartDebateMsg{
		Topic: topic, Context: m.context.Value(), Mode: m.mode, Tone: m.tone,
		Rounds: rounds, Model: modelSource,
	}
	m.errMsg = ""
	return m, func() tea.Msg { return msg }
}

func otherMode(m prompts.Mode) prompts.Mode {
	if m == prompts.ModeProposition {
		return prompts.ModeVersus
	}
	return prompts.ModeProposition
}

func nextTone(t prompts.Tone) prompts.Tone {
	for i, v := range prompts.ValidTones {
		if v == t {
			return prompts.ValidTones[(i+1)%len(prompts.ValidTones)]
		}
	}
	return prompts.DefaultTone
}

func (m FormModel) View() string {
	var b strings.Builder
	b.WriteString("Start a new debate\n\n")

	fmt.Fprintf(&b, "%s Topic:\n%s\n\n", focusMark(m.focus == fieldTopic), m.topic.View())
	fmt.Fprintf(&b, "%s Starting context:\n%s\n\n", focusMark(m.focus == fieldContext), m.context.View())
	fmt.Fprintf(&b, "%s Mode: %s  (←/→ to change)\n\n", focusMark(m.focus == fieldMode), m.mode)
	fmt.Fprintf(&b, "%s Tone: %s  (←/→ to change)\n\n", focusMark(m.focus == fieldTone), m.tone)
	fmt.Fprintf(&b, "%s Rounds:\n%s\n\n", focusMark(m.focus == fieldRounds), m.rounds.View())
	fmt.Fprintf(&b, "%s Model:\n%s\n\n", focusMark(m.focus == fieldModel), m.model.View())

	if m.errMsg != "" {
		fmt.Fprintf(&b, "! %s\n\n", m.errMsg)
	}
	b.WriteString("tab/shift+tab move · enter next/submit · ctrl+s submit · esc cancel\n")
	return b.String()
}

func focusMark(focused bool) string {
	if focused {
		return ">"
	}
	return " "
}

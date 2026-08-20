// Package features drives the agon TUI through godog BDD scenarios,
// using teatest so the app runs exactly as it would in a
// real terminal and a fake Engine so no real model is ever loaded.
package features

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/cucumber/godog"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/trollLemon/agon/internal/app"
	"github.com/trollLemon/agon/internal/archive"
	"github.com/trollLemon/agon/internal/orchestrator"
	"github.com/trollLemon/agon/internal/tools"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		Name:                "agon",
		ScenarioInitializer: initializeScenario(t),
		Options: &godog.Options{
			Format: "pretty",
			Paths:  []string{"."},
		},
	}
	if suite.Run() != 0 {
		t.Fatal("non-zero status returned from godog test suite")
	}
}

// world holds one scenario's state: the running app, the archive dir it
// reads/writes, and any archived session id created as a fixture. Steps are
// registered once against the suite; Before/After reset it per scenario.
type world struct {
	t          *testing.T
	archiveDir string
	tm         *teatest.TestModel
	seen       []byte // cumulative output for this scenario; waitFor never drains it
}

func initializeScenario(t *testing.T) func(*godog.ScenarioContext) {
	return func(ctx *godog.ScenarioContext) {
		w := &world{t: t}

		ctx.Before(func(sc context.Context, s *godog.Scenario) (context.Context, error) {
			w.archiveDir = ""
			w.tm = nil
			w.seen = nil
			return sc, nil
		})
		ctx.After(func(sc context.Context, s *godog.Scenario, err error) (context.Context, error) {
			if w.tm != nil {
				w.tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
				w.tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
			}
			return sc, err
		})

		ctx.Given(`^the app is open$`, w.theAppIsOpen)
		ctx.Given(`^the app is open with a model that never responds$`, w.theAppIsOpenBlocking)
		ctx.Given(`^an archived debate titled "([^"]*)" exists$`, w.anArchivedDebateTitledExists)

		ctx.When(`^I open the new debate form$`, w.iOpenTheNewDebateForm)
		ctx.When(`^I fill in the topic "([^"]*)"$`, w.iFillInTheTopic)
		ctx.When(`^I submit the form$`, w.iSubmitTheForm)
		ctx.When(`^I open the archive list$`, w.iOpenTheArchiveList)
		ctx.When(`^I open the first archived debate$`, w.iOpenTheFirstArchivedDebate)
		ctx.When(`^I abort the debate and confirm$`, w.iAbortTheDebateAndConfirm)

		ctx.Then(`^I should see "([^"]*)"$`, w.iShouldSee)
		ctx.Then(`^I should eventually see "([^"]*)"$`, w.iShouldSee)
		ctx.Then(`^the debate is archived$`, w.theDebateIsArchived)
		ctx.Then(`^no debate is archived$`, w.noDebateIsArchived)
		ctx.Then(`^the session is read-only$`, w.theSessionIsReadOnly)
	}
}

// fakeEngine is a deterministic, instant Engine so BDD scenarios never
// touch a real model. Initialize is a no-op.
type fakeEngine struct{}

func (fakeEngine) Initialize(ctx context.Context, modelSource string, log orchestrator.LogFunc) error {
	return nil
}

func (fakeEngine) ChatStreaming(ctx context.Context, role orchestrator.Role, messages []orchestrator.ChatMessage, toolSpecs []tools.Spec) (<-chan orchestrator.StreamEvent, error) {
	ch := make(chan orchestrator.StreamEvent, 2)
	go func() {
		defer close(ch)
		ch <- orchestrator.StreamEvent{ContentDelta: fmt.Sprintf("Reply from %s.", role)}
	}()
	return ch, nil
}

// blockingEngine never produces a response until its context is canceled, so
// the abort scenario can reliably catch a debate mid-turn. Initialize
// succeeds immediately.
type blockingEngine struct{}

func (blockingEngine) Initialize(ctx context.Context, modelSource string, log orchestrator.LogFunc) error {
	return nil
}

func (blockingEngine) ChatStreaming(ctx context.Context, role orchestrator.Role, messages []orchestrator.ChatMessage, toolSpecs []tools.Spec) (<-chan orchestrator.StreamEvent, error) {
	ch := make(chan orchestrator.StreamEvent)
	go func() {
		defer close(ch)
		<-ctx.Done()
	}()
	return ch, nil
}

func (w *world) newApp(engine orchestrator.Engine) {
	if w.archiveDir == "" {
		w.archiveDir = w.t.TempDir()
	}
	app := app.New(app.Options{ArchiveDir: w.archiveDir}, engine)
	w.tm = teatest.NewTestModel(w.t, app, teatest.WithInitialTermSize(120, 40))
	w.waitFor("Start a new debate")
}

func (w *world) theAppIsOpen() {
	w.newApp(fakeEngine{})
}

func (w *world) theAppIsOpenBlocking() {
	w.newApp(blockingEngine{})
}

func (w *world) anArchivedDebateTitledExists(title string) error {
	if w.archiveDir == "" {
		w.archiveDir = w.t.TempDir()
	}
	sess := archive.Session{
		SessionID: "demo-20260101-000000",
		Title:     title,
		Topic:     title,
		Mode:      "proposition",
		Tone:      "formal",
		Rounds:    1,
		Sides: []archive.Side{
			{ID: "advocate", Label: "Advocate", Stance: "for"},
			{ID: "critic", Label: "Critic", Stance: "against"},
		},
		Messages: []archive.Message{{Role: "advocate", Round: 1, Content: "Ship it."}},
		Verdict:  "Adopt.",
	}
	return archive.Write(w.archiveDir, sess)
}

func (w *world) iOpenTheNewDebateForm() {
	w.tm.Send(key("enter")) // menu cursor starts on "Start a new debate"
	w.waitFor("Topic:")
}

func (w *world) iFillInTheTopic(topic string) {
	w.tm.Type(topic)
}

func (w *world) iSubmitTheForm() {
	for range 6 {
		w.tm.Send(key("tab")) // topic -> context -> sandbox -> mode -> tone -> rounds -> model
	}
	w.tm.Send(key("enter")) // submit from the model field
}

func (w *world) iOpenTheArchiveList() {
	w.tm.Send(key("down")) // menu cursor -> "Browse archive"
	w.tm.Send(key("enter"))
}

func (w *world) iOpenTheFirstArchivedDebate() {
	w.tm.Send(key("enter"))
}

func (w *world) iAbortTheDebateAndConfirm() {
	w.tm.Send(key("a"))
	w.waitFor("Abort this debate")
	w.tm.Send(key("y"))
}

func (w *world) iShouldSee(text string) {
	w.waitFor(text)
}

func (w *world) theDebateIsArchived() error {
	entries, err := archive.List(w.archiveDir)
	if err != nil {
		return err
	}
	if len(entries) != 1 {
		return fmt.Errorf("expected exactly one archived session, got %d", len(entries))
	}
	return nil
}

func (w *world) noDebateIsArchived() error {
	entries, err := archive.List(w.archiveDir)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return fmt.Errorf("expected no archived session, got %d", len(entries))
	}
	return nil
}

func (w *world) theSessionIsReadOnly() {
	w.waitFor("archived")
}

func (w *world) waitFor(text string) {
	w.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		b, _ := io.ReadAll(w.tm.Output())
		w.seen = append(w.seen, b...)
		if bytes.Contains(w.seen, []byte(text)) {
			return
		}
		if time.Now().After(deadline) {
			w.t.Fatalf("timed out waiting for %q; last output:\n%s", text, w.seen)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

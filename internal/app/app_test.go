package app

import (
	"bytes"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/trollLemon/agon/internal/archive"
)

func waitForText(t *testing.T, tm *teatest.TestModel, text string) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte(text))
	}, teatest.WithDuration(20*time.Second), teatest.WithCheckInterval(10*time.Millisecond))
}

// waitForAllText is like waitForText but requires every substring to appear
// in the same accumulated output; use it instead of sequential waitForText
// calls when several expected strings can land in a single render frame,
// since WaitFor drains the bytes it has already seen.
func waitForAllText(t *testing.T, tm *teatest.TestModel, texts ...string) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		for _, text := range texts {
			if !bytes.Contains(b, []byte(text)) {
				return false
			}
		}
		return true
	}, teatest.WithDuration(20*time.Second), teatest.WithCheckInterval(10*time.Millisecond))
}

func TestNewDebateEndToEnd(t *testing.T) {
	dir := t.TempDir()
	app := New(Options{ArchiveDir: dir}, fakeEngine{})
	tm := teatest.NewTestModel(t, app, teatest.WithInitialTermSize(120, 40))

	waitForText(t, tm, "Start a new debate")

	tm.Send(key("enter")) // menu -> form
	waitForText(t, tm, "Topic:")

	tm.Type("Should we ship it")
	for range 6 {
		tm.Send(key("tab")) // topic -> context -> sandbox -> mode -> tone -> rounds -> model
	}
	tm.Send(key("enter")) // submit from the model field

	waitForText(t, tm, "Verdict")

	tm.Send(key("esc")) // back to menu; live debate is done, archive already written
	waitForText(t, tm, "Start a new debate")

	entries, err := archive.List(dir)
	if err != nil {
		t.Fatalf("archive.List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly one archived session, got %d", len(entries))
	}
	if entries[0].Mode != "proposition" {
		t.Errorf("expected proposition mode, got %q", entries[0].Mode)
	}

	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(20*time.Second))
}

func TestBrowseArchivedSessionReadOnly(t *testing.T) {
	dir := t.TempDir()
	sess := archive.Session{
		SessionID: "demo-20260101-000000",
		Title:     "Demo debate",
		Topic:     "Should we ship it?",
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
	if err := archive.Write(dir, sess); err != nil {
		t.Fatalf("archive.Write: %v", err)
	}

	app := New(Options{ArchiveDir: dir}, fakeEngine{})
	tm := teatest.NewTestModel(t, app, teatest.WithInitialTermSize(120, 40))

	waitForText(t, tm, "Start a new debate")
	tm.Send(key("down")) // cursor -> "Browse archive"
	tm.Send(key("enter"))
	waitForText(t, tm, "Archived debates")

	tm.Send(key("enter")) // open the only archived session
	waitForAllText(t, tm, "Demo debate", "Adopt")

	tm.Send(key("esc"))
	waitForText(t, tm, "Start a new debate")

	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(20*time.Second))
}

func TestAbortDiscardsLiveDebate(t *testing.T) {
	dir := t.TempDir()
	engine := blockingEngine{}
	app := New(Options{ArchiveDir: dir}, engine)
	tm := teatest.NewTestModel(t, app, teatest.WithInitialTermSize(120, 40))

	waitForText(t, tm, "Start a new debate")
	tm.Send(key("enter")) // menu -> form
	waitForText(t, tm, "Topic:")

	tm.Type("Should we ship it")
	for range 6 {
		tm.Send(key("tab"))
	}
	tm.Send(key("enter")) // submit

	waitForText(t, tm, "live")

	tm.Send(key("a"))
	waitForText(t, tm, "Abort this debate")

	tm.Send(key("y"))
	waitForText(t, tm, "finished")

	entries, err := archive.List(dir)
	if err != nil {
		t.Fatalf("archive.List: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no archived session after abort, got %d", len(entries))
	}

	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(20*time.Second))
}

// TestBootstrapLogVisibleWhileLoading exercises the exact bug reported by a
// user: the form screen showed a bare "Loading model…" for as long as
// bootstrap ran, with zero visibility into whether it was progressing or
// stuck. The engine's Initialize log callback should surface on screen while
// model client is still loading.
func TestBootstrapLogVisibleWhileLoading(t *testing.T) {
	dir := t.TempDir()
	app := New(Options{ArchiveDir: dir}, newSlowLoggingEngine(300*time.Millisecond))
	tm := teatest.NewTestModel(t, app, teatest.WithInitialTermSize(120, 40))

	waitForText(t, tm, "Start a new debate")
	tm.Send(key("enter")) // menu -> form
	waitForText(t, tm, "Topic:")

	tm.Type("Should we ship it")
	for range 6 {
		tm.Send(key("tab"))
	}
	tm.Send(key("enter")) // submit; kicks off the slow, logging bootstrap

	// Only assert the log became visible while still loading, not that the
	// very last line before completion renders: that final write races the
	// screen switching away to the live debate (the same class of "last
	// update can be lost to a screen transition" race as live.go's final
	// poke, see waitForLiveUpdate) and isn't reliable or important here —
	// what matters is that progress is visible at all during a slow load.
	waitForAllText(t, tm, "Loading model", "download-libraries: waiting to start download")
	waitForText(t, tm, "Verdict")

	tm.Send(key("esc"))
	waitForText(t, tm, "Start a new debate")

	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(20*time.Second))
}

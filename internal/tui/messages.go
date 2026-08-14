package tui

import (
	"github.com/trollLemon/agon/internal/archive"
	"github.com/trollLemon/agon/internal/prompts"
)

// Screen identifies which of the app's screens is rendered. Session view is
// exclusive: while it is active, nothing else is drawn (D8).
type Screen int

const (
	ScreenMenu Screen = iota
	ScreenForm
	ScreenBootstrap
	ScreenSession
	ScreenArchive
)

// SwitchScreenMsg requests a screen change.
type SwitchScreenMsg struct{ Screen Screen }

// StartDebateMsg carries a validated new-debate form submission.
type StartDebateMsg struct {
	Topic   string
	Context string
	Mode    prompts.Mode
	Tone    prompts.Tone
	Rounds  int
	Model   string
}

// BootstrapDoneMsg reports the result of lazily initializing the model
// engine (loading the chosen model).
type BootstrapDoneMsg struct {
	Err error
}

// OpenArchivedMsg requests opening a session (live or archived) by id.
type OpenArchivedMsg struct{ SessionID string }

// LiveUpdateMsg signals that a live debate has new events or finished; the
// root model re-renders whichever screen is displaying that session.
type LiveUpdateMsg struct{ SessionID string }

// ArchiveListLoadedMsg carries a freshly reloaded archive listing.
type ArchiveListLoadedMsg struct{ Items []archive.Session }
